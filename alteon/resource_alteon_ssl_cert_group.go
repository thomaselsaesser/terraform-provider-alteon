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
// Mitglieder: CertBmap (Hex-Bitmap), AddCert/RemCert (Kommandos).
// Type: 3=serverCertificate, 4=trustedCertificate, 5=intermediateCertificate.

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
				Elem:        &schema.Schema{Type: schema.TypeInt},
				Description: "Complete set of certificate indices in this group (declarative). Uses CertBmap/AddCert/RemCert.",
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

func sslGroupReadCerts(client *radwaregosdk.New_Client, groupID string) ([]int, diag.Diagnostics) {
	api := configPath(sslGroupTable, groupID)
	item, found, diags := readItem(client, api, sslGroupTable)
	if diags.HasError() || !found {
		return nil, diags
	}
	if v, ok := item["CertBmap"]; ok {
		return decodeHexBitmap(asString(v), 1), nil
	}
	return nil, diags
}

func sslGroupApplyCertDelta(client *radwaregosdk.New_Client, groupID string, want []int) diag.Diagnostics {
	have, diags := sslGroupReadCerts(client, groupID)
	if diags.HasError() {
		return diags
	}
	add, rem := setDelta(want, have)
	api := configPath(sslGroupTable, groupID)
	for _, c := range add {
		if dd := writeItem(client, api, map[string]interface{}{"AddCert": c}, false); dd.HasError() {
			return dd
		}
	}
	for _, c := range rem {
		if dd := writeItem(client, api, map[string]interface{}{"RemCert": c}, false); dd.HasError() {
			return dd
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
	if v, ok := d.GetOk("certificates"); ok {
		want := interfaceListToInts(v.(*schema.Set).List())
		if diags := sslGroupApplyCertDelta(client, id, want); diags.HasError() {
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
	certs, cdiags := sslGroupReadCerts(client, id)
	if cdiags.HasError() {
		return cdiags
	}
	d.Set("certificates", certs)
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
		want := interfaceListToInts(d.Get("certificates").(*schema.Set).List())
		if diags := sslGroupApplyCertDelta(client, id, want); diags.HasError() {
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
