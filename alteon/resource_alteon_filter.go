package alteon

import (
	"context"
	"strconv"
	"time"

	radwaregosdk "github.com/Radware/radware_go_sdk"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// alteon_filter ist jetzt in flat_specs.go (flat_spec-basiert, alle 120+ Felder).
// Hier verbleiben nur filter_port und filter_redirect_mapping.

const filterPortTable = "fltNewCfgPortTable"

func resource_alteon_filter_port() *schema.Resource {
	return &schema.Resource{
		CreateContext: resource_alteon_filter_port_create,
		ReadContext:   resource_alteon_filter_port_read,
		UpdateContext: resource_alteon_filter_port_update,
		DeleteContext: resource_alteon_filter_port_delete,
		Importer:      &schema.ResourceImporter{StateContext: schema.ImportStatePassthroughContext},
		Schema: map[string]*schema.Schema{
			"port":  {Type: schema.TypeInt, Required: true, ForceNew: true, Description: "Port index (Indx)."},
			"state": {Type: schema.TypeBool, Optional: true, Computed: true, Description: "Enable filtering on the port."},
			"filters": {
				Type:        schema.TypeSet,
				Optional:    true,
				Elem:        &schema.Schema{Type: schema.TypeInt},
				Description: "Complete set of filter indices applied to this port (declarative).",
			},
			"last_updated": {Type: schema.TypeString, Optional: true, Computed: true},
		},
	}
}

func filterPortReadRules(client *radwaregosdk.New_Client, port string) (rules []int, state bool, found bool, diags diag.Diagnostics) {
	item, ok, d := readItem(client, configPath(filterPortTable, port), filterPortTable)
	if d.HasError() || !ok {
		return nil, false, ok, d
	}
	if v, ok := item["FiltBmap"]; ok {
		rules = decodeHexBitmap(asString(v), 1)
	}
	if v, ok := item["State"]; ok {
		state = enableToBool(v)
	}
	return rules, state, true, diags
}

func filterPortApply(client *radwaregosdk.New_Client, port string, wantRules []int, state bool, setState bool) diag.Diagnostics {
	api := configPath(filterPortTable, port)
	if setState {
		if dd := writeItem(client, api, map[string]interface{}{"State": boolToEnable(state)}, false); dd.HasError() {
			return dd
		}
	}
	have, _, _, d := filterPortReadRules(client, port)
	if d.HasError() {
		return d
	}
	add, rem := setDelta(wantRules, have)
	for _, r := range add {
		if dd := writeItem(client, api, map[string]interface{}{"AddFiltRule": r}, false); dd.HasError() {
			return dd
		}
	}
	for _, r := range rem {
		if dd := writeItem(client, api, map[string]interface{}{"RemFiltRule": r}, false); dd.HasError() {
			return dd
		}
	}
	return nil
}

func resource_alteon_filter_port_create(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*radwaregosdk.New_Client)
	port := strconv.Itoa(d.Get("port").(int))
	d.SetId(port)
	want := interfaceListToInts(d.Get("filters").(*schema.Set).List())
	_, stateSet := d.GetOkExists("state")
	if diags := filterPortApply(client, port, want, d.Get("state").(bool), stateSet); diags.HasError() {
		return diags
	}
	return resource_alteon_filter_port_read(ctx, d, m)
}

func resource_alteon_filter_port_read(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*radwaregosdk.New_Client)
	rules, state, found, diags := filterPortReadRules(client, d.Id())
	if diags.HasError() {
		return diags
	}
	if !found {
		d.SetId("")
		return diags
	}
	if p, err := strconv.Atoi(d.Id()); err == nil {
		d.Set("port", p)
	}
	d.Set("filters", rules)
	d.Set("state", state)
	return diags
}

func resource_alteon_filter_port_update(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*radwaregosdk.New_Client)
	want := interfaceListToInts(d.Get("filters").(*schema.Set).List())
	if diags := filterPortApply(client, d.Id(), want, d.Get("state").(bool), d.HasChange("state")); diags.HasError() {
		return diags
	}
	d.Set("last_updated", time.Now().Format(time.RFC3339))
	return resource_alteon_filter_port_read(ctx, d, m)
}

func resource_alteon_filter_port_delete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	// Eine Port-Zeile wird nicht "geloescht"; wir entfernen alle Regeln und
	// deaktivieren das Filtering auf dem Port.
	client := m.(*radwaregosdk.New_Client)
	if diags := filterPortApply(client, d.Id(), nil, false, true); diags.HasError() {
		return diags
	}
	d.SetId("")
	return nil
}

// --- alteon_filter_redirect_mapping (fltNewCfgHttpRedirMappingTable) ----------
// Key: Filter (int) + FromStr (int). Feld: ToStr.

const filterRedirMapTable = "fltNewCfgHttpRedirMappingTable"

func resource_alteon_filter_redirect_mapping() *schema.Resource {
	return &schema.Resource{
		CreateContext: resource_alteon_filter_redirect_mapping_create,
		ReadContext:   resource_alteon_filter_redirect_mapping_read,
		UpdateContext: resource_alteon_filter_redirect_mapping_update,
		DeleteContext: resource_alteon_filter_redirect_mapping_delete,
		Importer:      &schema.ResourceImporter{StateContext: schema.ImportStatePassthroughContext},
		Schema: map[string]*schema.Schema{
			"filter":   {Type: schema.TypeInt, Required: true, ForceNew: true, Description: "Filter index (Filter)."},
			"from_str": {Type: schema.TypeInt, Required: true, ForceNew: true, Description: "From-string index (FromStr)."},
			"to_str":   {Type: schema.TypeString, Optional: true, Description: "Target string (ToStr)."},
		},
	}
}

func filterRedirMapKey(d *schema.ResourceData) string {
	return strconv.Itoa(d.Get("filter").(int)) + "/" + strconv.Itoa(d.Get("from_str").(int))
}

func resource_alteon_filter_redirect_mapping_create(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*radwaregosdk.New_Client)
	key := filterRedirMapKey(d)
	p := map[string]interface{}{}
	if v, ok := d.GetOk("to_str"); ok {
		p["ToStr"] = v.(string)
	}
	if diags := writeItem(client, configPath(filterRedirMapTable, key), p, true); diags.HasError() {
		return diags
	}
	d.SetId(key)
	return resource_alteon_filter_redirect_mapping_read(ctx, d, m)
}

func resource_alteon_filter_redirect_mapping_read(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*radwaregosdk.New_Client)
	item, found, diags := readItem(client, configPath(filterRedirMapTable, d.Id()), filterRedirMapTable)
	if diags.HasError() {
		return diags
	}
	if !found {
		d.SetId("")
		return diags
	}
	if v, ok := item["ToStr"]; ok {
		d.Set("to_str", asString(v))
	}
	return diags
}

func resource_alteon_filter_redirect_mapping_update(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*radwaregosdk.New_Client)
	p := map[string]interface{}{}
	if v, ok := d.GetOk("to_str"); ok {
		p["ToStr"] = v.(string)
	}
	if diags := writeItem(client, configPath(filterRedirMapTable, d.Id()), p, false); diags.HasError() {
		return diags
	}
	return resource_alteon_filter_redirect_mapping_read(ctx, d, m)
}

func resource_alteon_filter_redirect_mapping_delete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*radwaregosdk.New_Client)
	if diags := deleteItem(client, configPath(filterRedirMapTable, d.Id())); diags.HasError() {
		return diags
	}
	d.SetId("")
	return nil
}
