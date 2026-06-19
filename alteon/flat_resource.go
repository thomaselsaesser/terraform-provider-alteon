package alteon

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	radwaregosdk "github.com/Radware/radware_go_sdk"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// flat_resource.go implementiert einen generischen, datengetriebenen Resource-Builder
// fuer den Umbau der Bestandsressourcen (real_server, virtual_server, virtual_service,
// ssl_policy, http2_policy, https_health_check) auf flaches, deklaratives Schema.
//
// Statt pro Ressource 200+ Zeilen fast identischen Code zu pflegen, beschreibt eine
// flatSpec die Felder, und der Builder generiert Schema + CRUD daraus.
//
// Designentscheidungen (aus dem Debugging an server_group abgeleitet):
//   - Create: GetOk (nur konfigurierte Felder senden)
//   - Update: HasChange (nur geaenderte Felder senden, verhindert Widersprueche
//     bei interdependenten Feldern wie HealthCheckLayer/HealthID)
//   - Read: CamelCase-JSON vom Geraet -> snake_case Schema
//   - Partial-PUTs sind ok (per curl am Geraet bestaetigt)
//   - Command-Felder (Add*/Rem*/Delete*/Copy*) werden nicht ins Schema uebernommen

// flatFieldType beschreibt den Terraform-Typ eines Feldes.
type flatFieldType int

const (
	ftString flatFieldType = iota
	ftInt
)

// flatField beschreibt ein einzelnes Feld.
type flatField struct {
	Schema string        // snake_case HCL-Feldname
	API    string        // CamelCase REST-Feldname
	Type   flatFieldType // String oder Int
}

// flatSpec beschreibt eine komplette Ressource.
type flatSpec struct {
	ResourceName string
	Tables       []string      // REST-Tabellen (in Lesereihenfolge)
	KeySchema    string        // Schema-Feldname des Keys (z.B. "index")
	KeyType      flatFieldType // String oder Int
	// KeyAPIs: API-Feldnamen der Tabellen-Keys (werden beim Read uebersprungen).
	// Fuer Einzelschluessel z.B. ["Index"]; fuer mehrteilige z.B. ["ServIndex","Index"].
	KeyAPIs []string
	// Fuer zweiteilige Keys: zweites Key-Feld im Schema + API (optional).
	Key2Schema string
	Key2API    string
	Key2Type   flatFieldType
	// PartKeyAPIs: zusaetzliche Key-Felder in Part-Tabellen (ServSecondPartIndex etc.)
	PartKeyAPIs []string
	Fields      []flatField
}

// flatResourceFromSpec generiert eine vollstaendige Terraform-Ressource.
func flatResourceFromSpec(spec flatSpec) *schema.Resource {
	sch := map[string]*schema.Schema{
		spec.KeySchema: {
			Type:     schemaType(spec.KeyType),
			Required: true,
			ForceNew: true,
		},
		"last_updated": {
			Type:     schema.TypeString,
			Optional: true,
			Computed: true,
		},
	}
	if spec.Key2Schema != "" {
		sch[spec.Key2Schema] = &schema.Schema{
			Type:     schemaType(spec.Key2Type),
			Required: true,
			ForceNew: true,
		}
	}
	for _, f := range spec.Fields {
		sch[f.Schema] = &schema.Schema{
			Type:     schemaType(f.Type),
			Optional: true,
			Computed: true,
		}
	}

	var importer *schema.ResourceImporter
	if spec.Key2Schema != "" {
		importer = &schema.ResourceImporter{
			StateContext: flatImportTwoPartKey(spec),
		}
	} else {
		importer = &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		}
	}

	return &schema.Resource{
		CreateContext: flatCreate(spec),
		ReadContext:   flatRead(spec),
		UpdateContext: flatUpdate(spec),
		DeleteContext: flatDelete(spec),
		Importer:      importer,
		Schema:        sch,
	}
}

func schemaType(t flatFieldType) schema.ValueType {
	if t == ftInt {
		return schema.TypeInt
	}
	return schema.TypeString
}

// --- Payload-Builder ---------------------------------------------------------

func flatCreatePayload(spec flatSpec, d *schema.ResourceData) map[string]interface{} {
	p := map[string]interface{}{}
	for _, f := range spec.Fields {
		switch f.Type {
		case ftString:
			if v, ok := d.GetOk(f.Schema); ok {
				p[f.API] = v.(string)
			}
		case ftInt:
			if v, ok := d.GetOk(f.Schema); ok {
				p[f.API] = v.(int)
			}
		}
	}
	return p
}

func flatUpdatePayload(spec flatSpec, d *schema.ResourceData) map[string]interface{} {
	p := map[string]interface{}{}
	for _, f := range spec.Fields {
		if !d.HasChange(f.Schema) {
			continue
		}
		switch f.Type {
		case ftString:
			p[f.API] = d.Get(f.Schema).(string)
		case ftInt:
			p[f.API] = d.Get(f.Schema).(int)
		}
	}
	return p
}

// --- CRUD --------------------------------------------------------------------

func flatKeyPath(spec flatSpec, d *schema.ResourceData) string {
	if spec.Key2Schema == "" {
		// Einfacher Key.
		switch spec.KeyType {
		case ftInt:
			return strconv.Itoa(d.Get(spec.KeySchema).(int))
		default:
			v := d.Get(spec.KeySchema).(string)
			if v == "" {
				return d.Id()
			}
			return v
		}
	}
	// Zweiteiliger Key.
	var part1, part2 string
	switch spec.KeyType {
	case ftInt:
		part1 = strconv.Itoa(d.Get(spec.KeySchema).(int))
	default:
		part1 = d.Get(spec.KeySchema).(string)
	}
	switch spec.Key2Type {
	case ftInt:
		part2 = strconv.Itoa(d.Get(spec.Key2Schema).(int))
	default:
		part2 = d.Get(spec.Key2Schema).(string)
	}
	if part1 == "" || part2 == "" || part2 == "0" {
		return d.Id()
	}
	return part1 + "/" + part2
}

func flatCreate(spec flatSpec) schema.CreateContextFunc {
	return func(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
		client := m.(*radwaregosdk.New_Client)
		keyPath := flatKeyPath(spec, d)
		payload := flatCreatePayload(spec, d)

		// Schreibe in die ERSTE Tabelle (Haupt-Tabelle); bei mehrteiligen Tabellen
		// werden die Part-Tabellen via Update nachgezogen.
		api := configPath(spec.Tables[0], keyPath)
		if diags := writeItem(client, api, payload, true); diags.HasError() {
			return diags
		}
		d.SetId(keyPath)

		// Bei Multi-Table: restliche Tabellen per PUT aktualisieren.
		if len(spec.Tables) > 1 {
			for i := 1; i < len(spec.Tables); i++ {
				partPayload := flatPartPayload(spec, d, i)
				if len(partPayload) > 0 {
					api := configPath(spec.Tables[i], keyPath)
					if diags := writeItem(client, api, partPayload, false); diags.HasError() {
						return diags
					}
				}
			}
		}
		return flatRead(spec)(ctx, d, m)
	}
}

// flatPartPayload baut den Payload fuer eine Part-Tabelle (Index > 0).
// Felder werden der Tabelle zugeordnet ueber die Reihenfolge in der Fields-Liste.
// Da wir die Felder nicht nach Tabelle taggen (um die Spec einfach zu halten),
// senden wir bei Create alle Felder an alle Tabellen — Alteon ignoriert unbekannte
// Felder stillschweigend. Bei Update (HasChange) werden ohnehin nur geaenderte
// Felder gesendet.
func flatPartPayload(spec flatSpec, d *schema.ResourceData, tableIdx int) map[string]interface{} {
	// Beim Create senden wir den vollen Payload an Part-Tabellen.
	return flatCreatePayload(spec, d)
}

func flatRead(spec flatSpec) schema.ReadContextFunc {
	return func(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
		client := m.(*radwaregosdk.New_Client)
		keyPath := flatKeyPath(spec, d)
		if keyPath == "" || keyPath == "0" {
			keyPath = d.Id()
		}

		// Alle Key-API-Namen sammeln (Haupt + Part-Keys).
		skipKeys := map[string]bool{}
		for _, k := range spec.KeyAPIs {
			skipKeys[k] = true
		}
		for _, k := range spec.PartKeyAPIs {
			skipKeys[k] = true
		}

		// Feld-Lookup: API-Name -> flatField
		apiToField := map[string]*flatField{}
		for i := range spec.Fields {
			apiToField[spec.Fields[i].API] = &spec.Fields[i]
		}

		anyFound := false
		for _, table := range spec.Tables {
			api := configPath(table, keyPath)
			item, found, diags := readItem(client, api, table)
			if diags.HasError() {
				return diags
			}
			if !found {
				continue
			}
			anyFound = true
			for apiKey, val := range item {
				if skipKeys[apiKey] {
					continue
				}
				f, ok := apiToField[apiKey]
				if !ok {
					continue // Feld nicht im Schema -> ignorieren
				}
				switch f.Type {
				case ftString:
					d.Set(f.Schema, asString(val))
				case ftInt:
					d.Set(f.Schema, asInt(val))
				}
			}
		}
		if !anyFound {
			d.SetId("")
		} else {
			// Key-Felder in den State schreiben (nach Import noetig, sonst
			// ForceNew weil index von "" auf den echten Wert wechselt).
			switch spec.KeyType {
			case ftInt:
				if n, err := strconv.Atoi(keyPath); err == nil {
					d.Set(spec.KeySchema, n)
				}
			default:
				if spec.Key2Schema != "" {
					// Zweiteiliger Key: aufsplitten.
					parts := splitSlash(keyPath)
					if len(parts) == 2 {
						d.Set(spec.KeySchema, parts[0])
						switch spec.Key2Type {
						case ftInt:
							if n, err := strconv.Atoi(parts[1]); err == nil {
								d.Set(spec.Key2Schema, n)
							}
						default:
							d.Set(spec.Key2Schema, parts[1])
						}
					}
				} else {
					d.Set(spec.KeySchema, keyPath)
				}
			}
		}
		return nil
	}
}

func flatUpdate(spec flatSpec) schema.UpdateContextFunc {
	return func(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
		client := m.(*radwaregosdk.New_Client)
		keyPath := flatKeyPath(spec, d)
		payload := flatUpdatePayload(spec, d)

		if len(payload) > 0 {
			// Bei Multi-Table-Ressourcen: Payload an alle Tabellen senden.
			// Tabellen, die keines der Felder kennen, antworten mit 406
			// "Nothing to set" -- das ist kein Fehler, sondern heisst nur
			// "dieses Feld gehoert nicht zu mir". Wir ignorieren es.
			for _, table := range spec.Tables {
				api := configPath(table, keyPath)
				diags := writeItemLenient(client, api, payload)
				if diags.HasError() {
					return diags
				}
			}
		}
		d.Set("last_updated", time.Now().Format(time.RFC3339))
		return flatRead(spec)(ctx, d, m)
	}
}

func flatDelete(spec flatSpec) schema.DeleteContextFunc {
	return func(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
		client := m.(*radwaregosdk.New_Client)
		keyPath := flatKeyPath(spec, d)
		api := configPath(spec.Tables[0], keyPath)
		if diags := deleteItem(client, api); diags.HasError() {
			return diags
		}
		d.SetId("")
		return nil
	}
}

func flatImportTwoPartKey(spec flatSpec) schema.StateContextFunc {
	return func(ctx context.Context, d *schema.ResourceData, m interface{}) ([]*schema.ResourceData, error) {
		id := d.Id()
		parts := splitSlash(id)
		if len(parts) == 2 {
			switch spec.KeyType {
			case ftInt:
				if n, err := strconv.Atoi(parts[0]); err == nil {
					d.Set(spec.KeySchema, n)
				}
			default:
				d.Set(spec.KeySchema, parts[0])
			}
			switch spec.Key2Type {
			case ftInt:
				if n, err := strconv.Atoi(parts[1]); err == nil {
					d.Set(spec.Key2Schema, n)
				}
			default:
				d.Set(spec.Key2Schema, parts[1])
			}
		}
		return []*schema.ResourceData{d}, nil
	}
}

func splitSlash(s string) []string {
	for i, c := range s {
		if c == '/' {
			return []string{s[:i], s[i+1:]}
		}
	}
	return []string{s}
}

// asJSON ist ein Debug-Helfer (nicht im normalen Pfad genutzt).
func asJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}
