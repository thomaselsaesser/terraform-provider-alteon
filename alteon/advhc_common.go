package alteon

import (
	"context"
	"time"

	radwaregosdk "github.com/Radware/radware_go_sdk"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// Advanced Health Checks (/c/slb/advhc). Jeder HC-Typ hat eine eigene REST-Tabelle
// (slbNewAdvhc<Typ>Table) mit gemeinsamem Grundgeruest + typspezifischen Feldern.
// Statt 25 fast identische Dateien zu pflegen, beschreiben wir jeden Typ ueber eine
// kompakte Feldliste und generieren Schema + CRUD daraus.
//
// HTTP deckt HTTPS mit ab (Feld "https"), daher keine separate advhc_https-Ressource.
// Die bestehende Ressource alteon_https_health_check bleibt unangetastet bestehen.

// advhcFieldKind beschreibt, wie ein Feld zwischen HCL und JSON konvertiert wird.
type advhcFieldKind int

const (
	fInt    advhcFieldKind = iota // Integer
	fString                       // String
	fBool                         // 1/2-Enum -> bool
	fEnum                         // benannte Auswahl (z.B. "get"/"post") -> int
)

// advhcField beschreibt ein einzelnes Feld einer HC-Tabelle.
type advhcField struct {
	Schema string         // HCL-Feldname (snake_case)
	API    string         // JSON-Feldname (Alteon)
	Kind   advhcFieldKind // Typ-Konvertierung
	Desc   string         // Beschreibung
	Enum   map[string]int // fuer fEnum: name->wert
	EnumR  map[int]string // Rueckrichtung (gefuellt von buildEnumReverse)
}

// commonAdvhcFields ist das Grundgeruest, das in praktisch jedem HC-Typ vorkommt.
func commonAdvhcFields() []advhcField {
	return []advhcField{
		{Schema: "name", API: "Name", Kind: fString, Desc: "Descriptive name of the health check."},
		{Schema: "dport", API: "DPort", Kind: fInt, Desc: "Destination port to check (0 = inherit from service)."},
		{Schema: "ip_version", API: "IPVer", Kind: fInt, Desc: "IP version (4 or 6) used for the check."},
		{Schema: "host_name", API: "HostName", Kind: fString, Desc: "Host name used in the check."},
		{Schema: "transparent", API: "Transparent", Kind: fBool, Desc: "Transparent health check."},
		{Schema: "interval", API: "Interval", Kind: fInt, Desc: "Interval between checks (seconds)."},
		{Schema: "retries", API: "Retries", Kind: fInt, Desc: "Failures before the server is declared down."},
		{Schema: "restore_retries", API: "RestoreRetries", Kind: fInt, Desc: "Successes before a down server is restored."},
		{Schema: "timeout", API: "Timeout", Kind: fInt, Desc: "Timeout per check (seconds)."},
		{Schema: "overflow", API: "Overflow", Kind: fInt, Desc: "Overflow behaviour."},
		{Schema: "down_interval", API: "DownInterval", Kind: fInt, Desc: "Interval used while the server is down (seconds)."},
		{Schema: "invert", API: "Invert", Kind: fBool, Desc: "Invert the health check result."},
		{Schema: "snat", API: "Snat", Kind: fBool, Desc: "Use source NAT for the check."},
	}
}

// advhcTypeSpec beschreibt einen kompletten HC-Typ.
type advhcTypeSpec struct {
	ResourceName string // z.B. "alteon_advhc_tcp"
	Table        string // z.B. "slbNewAdvhcTcpTable"
	Fields       []advhcField
}

// buildEnumReverse fuellt EnumR fuer alle fEnum-Felder.
func (s *advhcTypeSpec) prepare() {
	for i := range s.Fields {
		f := &s.Fields[i]
		if f.Kind == fEnum && f.Enum != nil {
			f.EnumR = map[int]string{}
			for name, val := range f.Enum {
				f.EnumR[val] = name
			}
		}
	}
}

// resourceFromAdvhcSpec generiert eine vollstaendige Terraform-Ressource aus einer Spec.
func resourceFromAdvhcSpec(spec advhcTypeSpec) *schema.Resource {
	spec.prepare()
	sch := map[string]*schema.Schema{
		"id_name": {
			Type:        schema.TypeString,
			Required:    true,
			ForceNew:    true,
			Description: "Health check ID (the table key, a name string).",
		},
		"last_updated": {
			Type:     schema.TypeString,
			Optional: true,
			Computed: true,
		},
	}
	for _, f := range spec.Fields {
		s := &schema.Schema{Optional: true, Computed: true, Description: f.Desc}
		switch f.Kind {
		case fInt, fEnum:
			if f.Kind == fEnum {
				s.Type = schema.TypeString
			} else {
				s.Type = schema.TypeInt
			}
		case fString:
			s.Type = schema.TypeString
		case fBool:
			s.Type = schema.TypeBool
		}
		sch[f.Schema] = s
	}

	return &schema.Resource{
		CreateContext: advhcCreate(spec),
		ReadContext:   advhcRead(spec),
		UpdateContext: advhcUpdate(spec),
		DeleteContext: advhcDelete(spec),
		Importer:      &schema.ResourceImporter{StateContext: schema.ImportStatePassthroughContext},
		Schema:        sch,
	}
}

func advhcPayload(spec advhcTypeSpec, d *schema.ResourceData) map[string]interface{} {
	p := map[string]interface{}{}
	for _, f := range spec.Fields {
		switch f.Kind {
		case fInt:
			if v, ok := d.GetOk(f.Schema); ok {
				p[f.API] = v.(int)
			}
		case fString:
			if v, ok := d.GetOk(f.Schema); ok {
				p[f.API] = v.(string)
			}
		case fBool:
			if v, ok := d.GetOkExists(f.Schema); ok {
				p[f.API] = boolToEnable(v.(bool))
			}
		case fEnum:
			if v, ok := d.GetOk(f.Schema); ok {
				if num, found := f.Enum[v.(string)]; found {
					p[f.API] = num
				}
			}
		}
	}
	return p
}

func advhcApplyToState(spec advhcTypeSpec, d *schema.ResourceData, item map[string]interface{}) {
	for _, f := range spec.Fields {
		v, ok := item[f.API]
		if !ok {
			continue
		}
		switch f.Kind {
		case fInt:
			d.Set(f.Schema, asInt(v))
		case fString:
			d.Set(f.Schema, asString(v))
		case fBool:
			d.Set(f.Schema, enableToBool(v))
		case fEnum:
			if name, found := f.EnumR[asInt(v)]; found {
				d.Set(f.Schema, name)
			}
		}
	}
}

func advhcCreate(spec advhcTypeSpec) schema.CreateContextFunc {
	return func(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
		client := m.(*radwaregosdk.New_Client)
		key := d.Get("id_name").(string)
		api := configPath(spec.Table, key)
		if diags := writeItem(client, api, advhcPayload(spec, d), true); diags.HasError() {
			return diags
		}
		d.SetId(key)
		return advhcRead(spec)(ctx, d, m)
	}
}

func advhcRead(spec advhcTypeSpec) schema.ReadContextFunc {
	return func(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
		client := m.(*radwaregosdk.New_Client)
		api := configPath(spec.Table, d.Id())
		item, found, diags := readItem(client, api, spec.Table)
		if diags.HasError() {
			return diags
		}
		if !found {
			d.SetId("")
			return diags
		}
		d.Set("id_name", d.Id())
		advhcApplyToState(spec, d, item)
		return diags
	}
}

func advhcUpdate(spec advhcTypeSpec) schema.UpdateContextFunc {
	return func(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
		client := m.(*radwaregosdk.New_Client)
		api := configPath(spec.Table, d.Id())
		if diags := writeItem(client, api, advhcPayload(spec, d), false); diags.HasError() {
			return diags
		}
		d.Set("last_updated", time.Now().Format(time.RFC3339))
		return advhcRead(spec)(ctx, d, m)
	}
}

func advhcDelete(spec advhcTypeSpec) schema.DeleteContextFunc {
	return func(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
		client := m.(*radwaregosdk.New_Client)
		api := configPath(spec.Table, d.Id())
		if diags := deleteItem(client, api); diags.HasError() {
			return diags
		}
		d.SetId("")
		return nil
	}
}
