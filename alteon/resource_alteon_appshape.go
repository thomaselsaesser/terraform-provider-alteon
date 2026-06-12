package alteon

import (
	"context"
	"strconv"
	"time"

	radwaregosdk "github.com/Radware/radware_go_sdk"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// alteon_appshape_script verwaltet die Metadaten eines AppShape++-Skripts
// (slbNewCfgAppShapeTable): Index (Key, String), Name, State, Default.
//
// HINWEIS zum Skript-INHALT: In der vorliegenden REST-Doku (RDWRAlteonRestDoc,
// FW 34.0.9+) ist KEIN dedizierter Endpunkt zum Setzen des Skript-Codes ueber
// diese Tabelle belegt (api_ops nennt appshape nur im Logging-Kontext). Daher
// verwaltet diese Ressource bewusst NUR die Metadaten. Der Code-Upload-Mechanismus
// ist am Geraet zu verifizieren (vermutlich eigener Import-Endpunkt); sobald
// geklaert, kann ein "content"-Feld ergaenzt werden. NICHT geraten.

const appShapeTable = "slbNewCfgAppShapeTable"

func resource_alteon_appshape_script() *schema.Resource {
	return &schema.Resource{
		CreateContext: resource_alteon_appshape_script_create,
		ReadContext:   resource_alteon_appshape_script_read,
		UpdateContext: resource_alteon_appshape_script_update,
		DeleteContext: resource_alteon_appshape_script_delete,
		Importer:      &schema.ResourceImporter{StateContext: schema.ImportStatePassthroughContext},
		Schema: map[string]*schema.Schema{
			"index":        {Type: schema.TypeString, Required: true, ForceNew: true, Description: "AppShape script index/name (table key)."},
			"name":         {Type: schema.TypeString, Optional: true, Description: "Descriptive name."},
			"state":        {Type: schema.TypeBool, Optional: true, Computed: true, Description: "Enable (true) / disable (false) the script."},
			"default":      {Type: schema.TypeBool, Optional: true, Computed: true, Description: "Whether this is a default script."},
			"last_updated": {Type: schema.TypeString, Optional: true, Computed: true},
		},
	}
}

func appShapePayload(d *schema.ResourceData) map[string]interface{} {
	p := map[string]interface{}{}
	if v, ok := d.GetOk("name"); ok {
		p["Name"] = v.(string)
	}
	if v, ok := d.GetOkExists("state"); ok {
		p["State"] = boolToEnable(v.(bool))
	}
	if v, ok := d.GetOkExists("default"); ok {
		if v.(bool) {
			p["Default"] = 2
		} else {
			p["Default"] = 1
		}
	}
	return p
}

func resource_alteon_appshape_script_create(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*radwaregosdk.New_Client)
	key := d.Get("index").(string)
	if diags := writeItem(client, configPath(appShapeTable, key), appShapePayload(d), true); diags.HasError() {
		return diags
	}
	d.SetId(key)
	return resource_alteon_appshape_script_read(ctx, d, m)
}

func resource_alteon_appshape_script_read(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*radwaregosdk.New_Client)
	item, found, diags := readItem(client, configPath(appShapeTable, d.Id()), appShapeTable)
	if diags.HasError() {
		return diags
	}
	if !found {
		d.SetId("")
		return diags
	}
	d.Set("index", d.Id())
	if v, ok := item["Name"]; ok {
		d.Set("name", asString(v))
	}
	if v, ok := item["State"]; ok {
		d.Set("state", enableToBool(v))
	}
	if v, ok := item["Default"]; ok {
		d.Set("default", asInt(v) == 2)
	}
	return diags
}

func resource_alteon_appshape_script_update(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*radwaregosdk.New_Client)
	if diags := writeItem(client, configPath(appShapeTable, d.Id()), appShapePayload(d), false); diags.HasError() {
		return diags
	}
	d.Set("last_updated", time.Now().Format(time.RFC3339))
	return resource_alteon_appshape_script_read(ctx, d, m)
}

func resource_alteon_appshape_script_delete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*radwaregosdk.New_Client)
	if diags := deleteItem(client, configPath(appShapeTable, d.Id())); diags.HasError() {
		return diags
	}
	d.SetId("")
	return nil
}

// --- alteon_appshape_binding -------------------------------------------------
// Bindet ein AppShape-Skript an einen Virtual Service ODER einen Filter (Enhanced).
// Service-Bindung: slbNewCfgEnhSerAppShapeTable, Key VirtServIndex/VirtServiceIndex/Priority.
// Filter-Bindung:  slbNewCfgFiltAppShapeTable,  Key FiltIndex/Priority.
// Feld in beiden: Index (= AppShape-Skript-Name).
//
// target = "service" oder "filter" steuert, welche Tabelle genutzt wird.

const (
	serAppShapeTable  = "slbNewCfgEnhSerAppShapeTable"
	filtAppShapeTable = "slbNewCfgFiltAppShapeTable"
)

func resource_alteon_appshape_binding() *schema.Resource {
	return &schema.Resource{
		CreateContext: resource_alteon_appshape_binding_create,
		ReadContext:   resource_alteon_appshape_binding_read,
		UpdateContext: resource_alteon_appshape_binding_update,
		DeleteContext: resource_alteon_appshape_binding_delete,
		Importer:      &schema.ResourceImporter{StateContext: schema.ImportStatePassthroughContext},
		Schema: map[string]*schema.Schema{
			"target": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Binding target: \"service\" or \"filter\".",
			},
			"virtual_server": {
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
				Description: "Virtual server index (VirtServIndex) -- required for target=service.",
			},
			"virtual_service": {
				Type:        schema.TypeInt,
				Optional:    true,
				ForceNew:    true,
				Description: "Virtual service index (VirtServiceIndex) -- required for target=service.",
			},
			"filter": {
				Type:        schema.TypeInt,
				Optional:    true,
				ForceNew:    true,
				Description: "Filter index (FiltIndex) -- required for target=filter.",
			},
			"priority": {
				Type:        schema.TypeInt,
				Required:    true,
				ForceNew:    true,
				Description: "Binding priority (Priority).",
			},
			"script_index": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "AppShape script index/name to bind (Index field).",
			},
		},
	}
}

// appShapeBindingTableKey liefert Tabelle + Key je nach target.
func appShapeBindingTableKey(d *schema.ResourceData) (table, key string) {
	if d.Get("target").(string) == "filter" {
		table = filtAppShapeTable
		key = strconv.Itoa(d.Get("filter").(int)) + "/" + strconv.Itoa(d.Get("priority").(int))
	} else {
		table = serAppShapeTable
		key = d.Get("virtual_server").(string) + "/" +
			strconv.Itoa(d.Get("virtual_service").(int)) + "/" +
			strconv.Itoa(d.Get("priority").(int))
	}
	return table, key
}

func resource_alteon_appshape_binding_create(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*radwaregosdk.New_Client)
	table, key := appShapeBindingTableKey(d)
	p := map[string]interface{}{"Index": d.Get("script_index").(string)}
	if diags := writeItem(client, configPath(table, key), p, true); diags.HasError() {
		return diags
	}
	d.SetId(d.Get("target").(string) + ":" + key)
	return resource_alteon_appshape_binding_read(ctx, d, m)
}

func resource_alteon_appshape_binding_read(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*radwaregosdk.New_Client)
	table, key := appShapeBindingTableKey(d)
	item, found, diags := readItem(client, configPath(table, key), table)
	if diags.HasError() {
		return diags
	}
	if !found {
		d.SetId("")
		return diags
	}
	if v, ok := item["Index"]; ok {
		d.Set("script_index", asString(v))
	}
	return diags
}

func resource_alteon_appshape_binding_update(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*radwaregosdk.New_Client)
	table, key := appShapeBindingTableKey(d)
	p := map[string]interface{}{"Index": d.Get("script_index").(string)}
	if diags := writeItem(client, configPath(table, key), p, false); diags.HasError() {
		return diags
	}
	return resource_alteon_appshape_binding_read(ctx, d, m)
}

func resource_alteon_appshape_binding_delete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*radwaregosdk.New_Client)
	table, key := appShapeBindingTableKey(d)
	if diags := deleteItem(client, configPath(table, key)); diags.HasError() {
		return diags
	}
	d.SetId("")
	return nil
}
