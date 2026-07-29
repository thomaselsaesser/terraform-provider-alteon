package alteon

import (
	"context"
	"time"

	radwaregosdk "github.com/Radware/radware_go_sdk"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// alteon_ssl_cert_group verwaltet eine SSL-Zertifikatsgruppe
// (slbNewSslCfgGroupsTable) mit deklarativer Zertifikats-Mitgliederliste.
//
// Tabelle: slbNewSslCfgGroupsTable, Key: ID (String).
// Mitglieder: AddCert/RemCert (Kommandos) mit CERT-NAMEN (Strings).
// Type: 3=serverCertificate, 4=trustedCertificate, 5=intermediateCertificate.
//
// WICHTIG: Die CertBmap (Hex-Bitmap) kodiert INTERNE Positionen, die NICHT
// mit den Cert-Namen korrelieren. Daher wird die Bitmap NICHT zum Lesen der
// Mitglieder verwendet. Die Mitgliederliste bleibt wie konfiguriert im State
// (kein Drift-Detection auf Cert-Members). Schreiben ueber AddCert/RemCert
// mit dem korrekten Cert-Namen funktioniert einwandfrei.

const sslGroupTable = "slbNewSslCfgGroupsTable"

func resource_alteon_ssl_cert_group() *schema.Resource {
	return &schema.Resource{
		CreateContext: resource_alteon_ssl_cert_group_create,
		ReadContext:   resource_alteon_ssl_cert_group_read,
		UpdateContext: resource_alteon_ssl_cert_group_update,
		DeleteContext: resource_alteon_ssl_cert_group_delete,
		Importer:      &schema.ResourceImporter{StateContext: schema.ImportStatePassthroughContext},
		Schema: map[string]*schema.Schema{
			"group_id": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Certificate group ID (table key).",
			},
			"name": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"type": {
				Type:        schema.TypeInt,
				Optional:    true,
				Computed:    true,
				Description: "3=serverCertificate, 4=trustedCertificate, 5=intermediateCertificate.",
			},
			"default_cert": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"config_type": {
				Type:        schema.TypeInt,
				Optional:    true,
				Computed:    true,
				Description: "1=regular, 2=read-only.",
			},
			"chaining_mode": {
				Type:        schema.TypeInt,
				Optional:    true,
				Computed:    true,
				Description: "0=bySubjectIssuer, 1=bySkidAkid.",
			},
			"certificates": {
				Type:        schema.TypeSet,
				Optional:    true,
				Elem:        &schema.Schema{Type: schema.TypeString},
				Description: "Set of certificate names/IDs in this group (e.g. [\"3\", \"4\"] or [\"wahlenweb.intern.hessen.de\"]). Uses AddCert/RemCert with the cert name.",
			},
			"last_updated": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
		},
	}
}

func sslGroupHeadPayload(d *schema.ResourceData, create bool) map[string]interface{} {
	p := map[string]interface{}{}
	check := func(key string) bool {
		if create {
			_, ok := d.GetOk(key)
			return ok
		}
		return d.HasChange(key)
	}
	if check("name") {
		p["Name"] = d.Get("name").(string)
	}
	if check("type") {
		p["Type"] = d.Get("type").(int)
	}
	if check("default_cert") {
		p["DefaultCert"] = d.Get("default_cert").(string)
	}
	if check("config_type") {
		p["ConfigType"] = d.Get("config_type").(int)
	}
	if check("chaining_mode") {
		p["ChainingMode"] = d.Get("chaining_mode").(int)
	}
	return p
}

// sslGroupWantCerts gibt die konfigurierte Cert-Liste als String-Slice zurueck.
func sslGroupWantCerts(d *schema.ResourceData) []string {
	raw := d.Get("certificates").(*schema.Set).List()
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		out = append(out, v.(string))
	}
	return out
}

// sslGroupApplyCertDelta berechnet Delta und feuert AddCert/RemCert.
func sslGroupApplyCertDelta(client *radwaregosdk.New_Client, groupID string, oldCerts, newCerts []string) diag.Diagnostics {
	api := configPath(sslGroupTable, groupID)
	oldSet := map[string]bool{}
	newSet := map[string]bool{}
	for _, c := range oldCerts {
		oldSet[c] = true
	}
	for _, c := range newCerts {
		newSet[c] = true
	}
	// Hinzufuegen: in new aber nicht in old.
	for c := range newSet {
		if !oldSet[c] {
			if dd := writeItem(client, api, map[string]interface{}{"AddCert": c}, false); dd.HasError() {
				return dd
			}
		}
	}
	// Entfernen: in old aber nicht in new.
	for c := range oldSet {
		if !newSet[c] {
			if dd := writeItem(client, api, map[string]interface{}{"RemCert": c}, false); dd.HasError() {
				return dd
			}
		}
	}
	return nil
}

func resource_alteon_ssl_cert_group_create(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*radwaregosdk.New_Client)
	id := d.Get("group_id").(string)
	api := configPath(sslGroupTable, id)
	if diags := writeItem(client, api, sslGroupHeadPayload(d, true), true); diags.HasError() {
		return diags
	}
	d.SetId(id)
	// Certs hinzufuegen.
	if v, ok := d.GetOk("certificates"); ok {
		certs := make([]string, 0)
		for _, c := range v.(*schema.Set).List() {
			certs = append(certs, c.(string))
		}
		if diags := sslGroupApplyCertDelta(client, id, nil, certs); diags.HasError() {
			return diags
		}
	}
	return resource_alteon_ssl_cert_group_read(ctx, d, m)
}

func resource_alteon_ssl_cert_group_read(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*radwaregosdk.New_Client)
	id := d.Id()
	api := configPath(sslGroupTable, id)
	item, found, diags := readItem(client, api, sslGroupTable)
	if diags.HasError() {
		return diags
	}
	if !found {
		d.SetId("")
		return diags
	}
	d.Set("group_id", id)
	if v, ok := item["Name"]; ok {
		d.Set("name", asString(v))
	}
	if v, ok := item["Type"]; ok {
		d.Set("type", asInt(v))
	}
	if v, ok := item["DefaultCert"]; ok {
		d.Set("default_cert", asString(v))
	}
	if v, ok := item["ConfigType"]; ok {
		d.Set("config_type", asInt(v))
	}
	if v, ok := item["ChainingMode"]; ok {
		d.Set("chaining_mode", asInt(v))
	}
	// Cert-Members werden NICHT aus der Bitmap gelesen (interne Positionen,
	// nicht Cert-Namen). Die Liste bleibt wie konfiguriert im State.
	// Drift-Detection auf Cert-Members ist damit nicht moeglich.
	return diags
}

func resource_alteon_ssl_cert_group_update(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*radwaregosdk.New_Client)
	id := d.Id()
	api := configPath(sslGroupTable, id)
	payload := sslGroupHeadPayload(d, false)
	if len(payload) > 0 {
		if diags := writeItem(client, api, payload, false); diags.HasError() {
			return diags
		}
	}
	if d.HasChange("certificates") {
		old, nw := d.GetChange("certificates")
		oldCerts := make([]string, 0)
		newCerts := make([]string, 0)
		for _, c := range old.(*schema.Set).List() {
			oldCerts = append(oldCerts, c.(string))
		}
		for _, c := range nw.(*schema.Set).List() {
			newCerts = append(newCerts, c.(string))
		}
		if diags := sslGroupApplyCertDelta(client, id, oldCerts, newCerts); diags.HasError() {
			return diags
		}
	}
	d.Set("last_updated", time.Now().Format(time.RFC3339))
	return resource_alteon_ssl_cert_group_read(ctx, d, m)
}

func resource_alteon_ssl_cert_group_delete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*radwaregosdk.New_Client)
	api := configPath(sslGroupTable, d.Id())
	if diags := deleteItem(client, api); diags.HasError() {
		return diags
	}
	d.SetId("")
	return nil
}
