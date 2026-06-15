package alteon

import (
	"context"
	"strconv"
	"strings"

	radwaregosdk "github.com/Radware/radware_go_sdk"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// Diese Datei ruestet die bestehenden SLB-Ressourcen (real_server, server_group,
// virtual_server, virtual_service, ssl_policy, http2_policy, https_health_check)
// mit Import + echtem Read nach, OHNE ihr vorhandenes elements-Schema zu aendern.
//
// Mapping-Prinzip (gegen echte Geraete-Daten verifiziert):
//   - Die elements-Block-Felder heissen lowercase OHNE Trenner; das JSON vom Geraet
//     ist CamelCase. Regel: schemaKey = strings.ToLower(jsonKey) -- trifft die grosse
//     Mehrheit der Felder.
//   - Wenige Felder weichen ab (z.B. JSON ProxyIpAddr -> Schema proxyipaddress).
//     Dafuer gibt es pro Ressource eine kleine override-Map.
//   - JSON-Felder ohne passendes Schema-Feld werden ignoriert (kein Phantom-Diff).
//   - Der Tabellen-Key (Index/VirtServerIndex/...) wird NICHT in den elements-Block
//     geschrieben, sondern bleibt im separaten Key-Schemafeld.

// legacyImportSpec beschreibt, wie eine Bestandsressource gelesen wird.
type legacyImportSpec struct {
	// Tabellen, aus denen die elements zusammengesetzt werden (in Reihenfolge).
	Tables []string
	// JSON-Keyfelder, die NICHT in den elements-Block gehoeren (Tabellen-Keys).
	KeyFields map[string]bool
	// elementsAttr ist der Name des Listen-Attributs im Schema (meist "elements").
	ElementsAttr string
	// Overrides: jsonKey -> schemaKey, wo ToLower nicht reicht.
	Overrides map[string]string
	// Schema-Feldnamen des elements-Blocks (zum Filtern unbekannter JSON-Felder).
	ElementFields map[string]bool
}

// legacyReadElements liest die Tabellen und baut den elements-Block.
func legacyReadElements(client *radwaregosdk.New_Client, spec legacyImportSpec, keyPath string) (map[string]interface{}, bool, diag.Diagnostics) {
	merged := map[string]interface{}{}
	anyFound := false
	var diags diag.Diagnostics

	for _, table := range spec.Tables {
		api := "/config/" + table + "/" + keyPath + "/"
		item, found, d := readItem(client, api, table)
		if d.HasError() {
			return nil, false, d
		}
		if !found {
			continue
		}
		anyFound = true
		for jsonKey, val := range item {
			if spec.KeyFields[jsonKey] {
				continue // Tabellen-Key nicht in elements
			}
			schemaKey := strings.ToLower(jsonKey)
			if ov, ok := spec.Overrides[jsonKey]; ok {
				schemaKey = ov
			}
			// Nur Felder uebernehmen, die das Schema kennt.
			if spec.ElementFields != nil && !spec.ElementFields[schemaKey] {
				continue
			}
			merged[schemaKey] = val
		}
	}
	return merged, anyFound, diags
}

// legacyImportRead ist der gemeinsame Read fuer die nachgeruesteten Ressourcen.
// keyPath ist der vollstaendige Pfadschluessel (z.B. "5" oder "96/1").
func legacyImportRead(ctx context.Context, d *schema.ResourceData, m interface{}, spec legacyImportSpec, keyPath string) diag.Diagnostics {
	client := m.(*radwaregosdk.New_Client)
	merged, found, diags := legacyReadElements(client, spec, keyPath)
	if diags.HasError() {
		return diags
	}
	if !found {
		d.SetId("")
		return diags
	}
	// elements ist eine Liste mit genau einem Element.
	d.Set(spec.ElementsAttr, []interface{}{merged})
	return diags
}

// elementFieldsFromResource liest die Feldnamen des elements-Blocks aus dem
// Ressourcen-Schema, damit die override/Filter-Logik nichts Unbekanntes setzt.
func elementFieldsFromResource(r *schema.Resource, elementsAttr string) map[string]bool {
	out := map[string]bool{}
	if s, ok := r.Schema[elementsAttr]; ok {
		if res, ok := s.Elem.(*schema.Resource); ok {
			for k := range res.Schema {
				out[k] = true
			}
		}
	}
	return out
}

// importTwoPartKey ist ein StateContext-Importer fuer zweiteilige Keys ("a/b"),
// der die beiden Teile auf die angegebenen Schemafelder verteilt. field2 ist im
// virtual_service-Schema ein Integer, daher wird der zweite Teil als int gesetzt.
func importTwoPartKey(field1, field2 string) schema.StateContextFunc {
	return func(ctx context.Context, d *schema.ResourceData, m interface{}) ([]*schema.ResourceData, error) {
		parts := strings.SplitN(d.Id(), "/", 2)
		if len(parts) == 2 {
			d.Set(field1, parts[0])
			if n, err := strconv.Atoi(parts[1]); err == nil {
				d.Set(field2, n)
			} else {
				d.Set(field2, parts[1])
			}
		}
		return []*schema.ResourceData{d}, nil
	}
}
