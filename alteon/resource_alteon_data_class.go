package alteon

import (
	"context"
	"time"

	radwaregosdk "github.com/Radware/radware_go_sdk"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// alteon_data_class verwaltet eine Data Class (slbNewDataclassCfgDataClassesTable)
// samt ihrer manuellen Eintraege (slbNewDataclassCfgManualEntriesTable).
//
// Kopf-Key: Id (String). Eintrags-Key: DcId (=Kopf-Id) + Id (Integer, je Eintrag).
// Eintraege werden ueber Del:2 geloescht (NICHT per DELETE-Methode -- das ist die
// dokumentierte Ausnahme bei dieser Tabelle).
//
// Nur "manual entries" werden abgebildet (wie abgestimmt).

const (
	dataClassTable      = "slbNewDataclassCfgDataClassesTable"
	dataClassEntryTable = "slbNewDataclassCfgManualEntriesTable"
)

func resource_alteon_data_class() *schema.Resource {
	return &schema.Resource{
		CreateContext: resource_alteon_data_class_create,
		ReadContext:   resource_alteon_data_class_read,
		UpdateContext: resource_alteon_data_class_update,
		DeleteContext: resource_alteon_data_class_delete,
		Importer:      &schema.ResourceImporter{StateContext: schema.ImportStatePassthroughContext},
		Schema: map[string]*schema.Schema{
			"id_name": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Data class ID (table key).",
			},
			"name": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Descriptive name.",
			},
			"data_type": {
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Description: "Data type: \"string\" or \"ip\".",
			},
			"default": {
				Type:        schema.TypeBool,
				Optional:    true,
				Computed:    true,
				Description: "Whether this is the default data class.",
			},
			"entry": {
				Type:        schema.TypeList,
				Optional:    true,
				Description: "Manual entries of the data class.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"id": {
							Type:        schema.TypeInt,
							Required:    true,
							Description: "Entry ID (numeric, unique within the data class).",
						},
						"key": {
							Type:        schema.TypeString,
							Required:    true,
							Description: "Entry key.",
						},
						"value": {
							Type:        schema.TypeString,
							Optional:    true,
							Description: "Entry value.",
						},
					},
				},
			},
			"last_updated": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
		},
	}
}

func dataClassHeadPayload(d *schema.ResourceData) map[string]interface{} {
	p := map[string]interface{}{}
	if v, ok := d.GetOk("name"); ok {
		p["Name"] = v.(string)
	}
	if v, ok := d.GetOk("data_type"); ok {
		if v.(string) == "ip" {
			p["DataType"] = 2
		} else {
			p["DataType"] = 1
		}
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

// dataClassWriteEntries schreibt alle aktuell konfigurierten Eintraege.
func dataClassWriteEntries(client *radwaregosdk.New_Client, dcID string, d *schema.ResourceData) diag.Diagnostics {
	var diags diag.Diagnostics
	entries := d.Get("entry").([]interface{})
	for _, raw := range entries {
		e := raw.(map[string]interface{})
		entryKey := dcID + "/" + intToStr(e["id"].(int))
		api := configPath(dataClassEntryTable, entryKey)
		payload := map[string]interface{}{
			"Key": e["key"].(string),
		}
		if val, ok := e["value"]; ok && val.(string) != "" {
			payload["Value"] = val.(string)
		}
		if dd := writeItem(client, api, payload, true); dd.HasError() {
			return dd
		}
	}
	return diags
}

func resource_alteon_data_class_create(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*radwaregosdk.New_Client)
	id := d.Get("id_name").(string)
	api := configPath(dataClassTable, id)

	if diags := writeItem(client, api, dataClassHeadPayload(d), true); diags.HasError() {
		return diags
	}
	d.SetId(id)

	if diags := dataClassWriteEntries(client, id, d); diags.HasError() {
		return diags
	}
	return resource_alteon_data_class_read(ctx, d, m)
}

func resource_alteon_data_class_read(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*radwaregosdk.New_Client)
	id := d.Id()
	api := configPath(dataClassTable, id)

	item, found, diags := readItem(client, api, dataClassTable)
	if diags.HasError() {
		return diags
	}
	if !found {
		d.SetId("")
		return diags
	}
	d.Set("id_name", id)
	if v, ok := item["Name"]; ok {
		d.Set("name", asString(v))
	}
	if v, ok := item["DataType"]; ok {
		if asInt(v) == 2 {
			d.Set("data_type", "ip")
		} else {
			d.Set("data_type", "string")
		}
	}
	if v, ok := item["Default"]; ok {
		d.Set("default", asInt(v) == 2)
	}
	// Eintraege werden im State belassen wie konfiguriert; ein vollstaendiger
	// Entry-Read wuerde die gesamte Eintragstabelle gefiltert nach DcId erfordern.
	// Drift auf Kopf-Ebene wird erkannt; Entry-Drift kann bei Bedarf ergaenzt werden.
	return diags
}

func resource_alteon_data_class_update(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*radwaregosdk.New_Client)
	id := d.Id()
	api := configPath(dataClassTable, id)

	if diags := writeItem(client, api, dataClassHeadPayload(d), false); diags.HasError() {
		return diags
	}
	// Eintraege: entfernte Eintraege per Del:2 loeschen, aktuelle (neu)schreiben.
	if d.HasChange("entry") {
		old, nw := d.GetChange("entry")
		if diags := dataClassDeleteRemovedEntries(client, id, old.([]interface{}), nw.([]interface{})); diags.HasError() {
			return diags
		}
	}
	if diags := dataClassWriteEntries(client, id, d); diags.HasError() {
		return diags
	}
	d.Set("last_updated", time.Now().Format(time.RFC3339))
	return resource_alteon_data_class_read(ctx, d, m)
}

// dataClassDeleteRemovedEntries loescht Eintraege, die in old, aber nicht mehr in nw sind.
func dataClassDeleteRemovedEntries(client *radwaregosdk.New_Client, dcID string, old, nw []interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	newIDs := map[int]bool{}
	for _, raw := range nw {
		newIDs[raw.(map[string]interface{})["id"].(int)] = true
	}
	for _, raw := range old {
		oid := raw.(map[string]interface{})["id"].(int)
		if !newIDs[oid] {
			entryKey := dcID + "/" + intToStr(oid)
			api := configPath(dataClassEntryTable, entryKey)
			// Loeschen ueber Del:2 (Ausnahme dieser Tabelle).
			if dd := writeItem(client, api, map[string]interface{}{"Del": 2}, false); dd.HasError() {
				return dd
			}
		}
	}
	return diags
}

func resource_alteon_data_class_delete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*radwaregosdk.New_Client)
	api := configPath(dataClassTable, d.Id())
	if diags := deleteItem(client, api); diags.HasError() {
		return diags
	}
	d.SetId("")
	return nil
}
