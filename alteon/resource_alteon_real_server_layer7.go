package alteon

import (
	"context"
	"time"

	radwaregosdk "github.com/Radware/radware_go_sdk"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// alteon_real_server_layer7 verwaltet die Layer-7-URL-Zuordnung eines Real Servers.
//
// CLI-Aequivalent: /c/slb/real <id>/layer7/exclude e/addlb <url-index>
//
// Liest UrlBmap aus slbNewCfgEnhRealServerSecondPartTable (Ist-Zustand),
// schreibt AddUrl/RemUrl + ExcludeStr auf slbNewCfgEnhRealServerTable (Kommandos).
//
// Key: der Real-Server-Index. Pro Real Server gibt es maximal eine Layer7-Zuordnung.

const (
	rsMainTable   = "SlbNewCfgEnhRealServerTable"
	rsSecondTable = "SlbNewCfgEnhRealServerSecondPartTable"
)

func resource_alteon_real_server_layer7() *schema.Resource {
	return &schema.Resource{
		CreateContext: rsL7Create,
		ReadContext:   rsL7Read,
		UpdateContext: rsL7Update,
		DeleteContext: rsL7Delete,
		Importer:      &schema.ResourceImporter{StateContext: schema.ImportStatePassthroughContext},
		Schema: map[string]*schema.Schema{
			"real_server": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Real server index.",
			},
			"exclude_str": {
				Type:        schema.TypeInt,
				Optional:    true,
				Computed:    true,
				Description: "1=enabled, 2=disabled. Enables Layer7 exclude-string mode.",
			},
			"urls": {
				Type:        schema.TypeSet,
				Optional:    true,
				Elem:        &schema.Schema{Type: schema.TypeInt},
				Description: "Complete set of URL path indices (from alteon_url_lb_path) assigned to this real server.",
			},
			"last_updated": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
		},
	}
}

// rsL7ReadUrls liest die URL-Zuordnung aus UrlBmap.
func rsL7ReadUrls(client *radwaregosdk.New_Client, rsKey string) (urls []int, excludeStr int, diags diag.Diagnostics) {
	// UrlBmap aus SecondPart
	api2 := configPath(rsSecondTable, rsKey)
	item2, found, d := readItem(client, api2, rsSecondTable)
	if d.HasError() {
		return nil, 0, d
	}
	if found {
		if v, ok := item2["UrlBmap"]; ok {
			urls = decodeHexBitmap(asString(v))
		}
	}
	// ExcludeStr aus Haupttabelle
	api1 := configPath(rsMainTable, rsKey)
	item1, found1, d1 := readItem(client, api1, rsMainTable)
	if d1.HasError() {
		return urls, 0, d1
	}
	if found1 {
		if v, ok := item1["ExcludeStr"]; ok {
			excludeStr = asInt(v)
		}
	}
	return urls, excludeStr, nil
}

func rsL7ApplyUrlDelta(client *radwaregosdk.New_Client, rsKey string, want []int) diag.Diagnostics {
	have, _, diags := rsL7ReadUrls(client, rsKey)
	if diags.HasError() {
		return diags
	}
	add, rem := setDelta(want, have)
	api := configPath(rsMainTable, rsKey)
	for _, u := range add {
		if dd := writeItem(client, api, map[string]interface{}{"AddUrl": u}, false); dd.HasError() {
			return dd
		}
	}
	for _, u := range rem {
		if dd := writeItem(client, api, map[string]interface{}{"RemUrl": u}, false); dd.HasError() {
			return dd
		}
	}
	return nil
}

func rsL7Create(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*radwaregosdk.New_Client)
	rsKey := d.Get("real_server").(string)
	api := configPath(rsMainTable, rsKey)
	// ExcludeStr setzen
	if v, ok := d.GetOk("exclude_str"); ok {
		if dd := writeItem(client, api, map[string]interface{}{"ExcludeStr": v.(int)}, false); dd.HasError() {
			return dd
		}
	}
	d.SetId(rsKey)
	// URL-Delta
	if v, ok := d.GetOk("urls"); ok {
		want := interfaceListToInts(v.(*schema.Set).List())
		if diags := rsL7ApplyUrlDelta(client, rsKey, want); diags.HasError() {
			return diags
		}
	}
	return rsL7Read(ctx, d, m)
}

func rsL7Read(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*radwaregosdk.New_Client)
	rsKey := d.Id()
	urls, excludeStr, diags := rsL7ReadUrls(client, rsKey)
	if diags.HasError() {
		return diags
	}
	d.Set("real_server", rsKey)
	d.Set("exclude_str", excludeStr)
	d.Set("urls", urls)
	return nil
}

func rsL7Update(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*radwaregosdk.New_Client)
	rsKey := d.Id()
	if d.HasChange("exclude_str") {
		api := configPath(rsMainTable, rsKey)
		if dd := writeItem(client, api, map[string]interface{}{"ExcludeStr": d.Get("exclude_str").(int)}, false); dd.HasError() {
			return dd
		}
	}
	if d.HasChange("urls") {
		want := interfaceListToInts(d.Get("urls").(*schema.Set).List())
		if diags := rsL7ApplyUrlDelta(client, rsKey, want); diags.HasError() {
			return diags
		}
	}
	d.Set("last_updated", time.Now().Format(time.RFC3339))
	return rsL7Read(ctx, d, m)
}

func rsL7Delete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*radwaregosdk.New_Client)
	rsKey := d.Id()
	// Alle URLs entfernen und Exclude deaktivieren.
	if diags := rsL7ApplyUrlDelta(client, rsKey, nil); diags.HasError() {
		return diags
	}
	api := configPath(rsMainTable, rsKey)
	writeItem(client, api, map[string]interface{}{"ExcludeStr": 2}, false) // 2=disabled
	d.SetId("")
	return nil
}
