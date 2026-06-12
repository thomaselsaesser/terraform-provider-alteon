package alteon

import (
	"context"
	"time"

	radwaregosdk "github.com/Radware/radware_go_sdk"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// alteon_content_class verwaltet eine Content Class (layer7NewCfgContentClassTable)
// samt ihrer Match-Eintraege in den acht Subtabellen (HostName, Path, FileName,
// FileType, Header, Cookie, Text, XML).
//
// Modellierung: nested blocks pro Match-Typ. logical_expression ist optional und
// bleibt bei euch leer (alle Content Classes nutzen genau einen Match-Typ -- daher
// keine Verknuepfungslogik noetig). Die Aktiv-Flags der Haupttabelle (HostName=1,
// Path=2, ...) werden beim Schreiben der jeweiligen Subtabelle vom Geraet gesetzt;
// wir setzen sie nicht zusaetzlich explizit (am Geraet zu verifizieren).
//
// Match-Type-Enums (verifiziert):
//   url-basiert (hostname/path/filename/filetype): 1=sufx 2=prefx 3=equal 4=include 5=regex
//   header/cookie name+val:                        3=equal 4=include 5=regex
//   text:                                          4=include 5=regex; lookup 1=header 2=body 3=both
//   xml tag name:                                  1=sufx 3=equal; val: 1=sufx 3=equal 4=include

const (
	ccTable         = "layer7NewCfgContentClassTable"
	ccHostNameTable = "layer7NewCfgContentClassHostNameTable"
	ccPathTable     = "layer7NewCfgContentClassPathTable"
	ccFileNameTable = "layer7NewCfgContentClassFileNameTable"
	ccFileTypeTable = "layer7NewCfgContentClassFileTypeTable"
	ccHeaderTable   = "layer7NewCfgContentClassHeaderTable"
	ccCookieTable   = "layer7NewCfgContentClassCookieTable"
	ccTextTable     = "layer7NewCfgContentClassTextTable"
	ccXmlTable      = "layer7NewCfgContentClassXmlTable"
)

var urlMatchType = map[string]int{"sufx": 1, "prefx": 2, "equal": 3, "include": 4, "regex": 5}
var hdrMatchType = map[string]int{"equal": 3, "include": 4, "regex": 5}
var textMatchType = map[string]int{"include": 4, "regex": 5}
var textLookup = map[string]int{"header": 1, "body": 2, "both": 3}
var xmlNameMatch = map[string]int{"sufx": 1, "equal": 3}
var xmlValMatch = map[string]int{"sufx": 1, "equal": 3, "include": 4}

func reverseMap(m map[string]int) map[int]string {
	r := map[int]string{}
	for k, v := range m {
		r[v] = k
	}
	return r
}

func enumToInt(m map[string]int, s string) (int, bool) { v, ok := m[s]; return v, ok }
func intToEnum(m map[string]int, n int) string {
	for k, v := range m {
		if v == n {
			return k
		}
	}
	return ""
}

func matchEntryID() *schema.Schema {
	return &schema.Schema{
		Type:        schema.TypeString,
		Required:    true,
		Description: "Match entry ID (unique within this match type for the content class).",
	}
}

func resource_alteon_content_class() *schema.Resource {
	return &schema.Resource{
		CreateContext: resource_alteon_content_class_create,
		ReadContext:   resource_alteon_content_class_read,
		UpdateContext: resource_alteon_content_class_update,
		DeleteContext: resource_alteon_content_class_delete,
		Importer:      &schema.ResourceImporter{StateContext: schema.ImportStatePassthroughContext},
		Schema: map[string]*schema.Schema{
			"id_name": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Content class ID (table key).",
			},
			"name": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"type": {
				Type:        schema.TypeInt,
				Optional:    true,
				Computed:    true,
				Description: "Content class type.",
			},
			"logical_expression": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Logical expression combining match entries. Usually empty (single match type).",
			},
			"hostname": {
				Type:     schema.TypeList,
				Optional: true,
				Elem: &schema.Resource{Schema: map[string]*schema.Schema{
					"id":            matchEntryID(),
					"host_name":     {Type: schema.TypeString, Required: true},
					"match_type":    {Type: schema.TypeString, Optional: true, Default: "equal", Description: "sufx|prefx|equal|include|regex"},
					"data_class_id": {Type: schema.TypeString, Optional: true},
				}},
			},
			"path": {
				Type:     schema.TypeList,
				Optional: true,
				Elem: &schema.Resource{Schema: map[string]*schema.Schema{
					"id":            matchEntryID(),
					"file_path":     {Type: schema.TypeString, Required: true},
					"match_type":    {Type: schema.TypeString, Optional: true, Default: "equal", Description: "sufx|prefx|equal|include|regex"},
					"case":          {Type: schema.TypeBool, Optional: true},
					"data_class_id": {Type: schema.TypeString, Optional: true},
				}},
			},
			"filename": {
				Type:     schema.TypeList,
				Optional: true,
				Elem: &schema.Resource{Schema: map[string]*schema.Schema{
					"id":         matchEntryID(),
					"file_name":  {Type: schema.TypeString, Required: true},
					"match_type": {Type: schema.TypeString, Optional: true, Default: "equal", Description: "sufx|prefx|equal|include|regex"},
					"case":       {Type: schema.TypeBool, Optional: true},
				}},
			},
			"filetype": {
				Type:     schema.TypeList,
				Optional: true,
				Elem: &schema.Resource{Schema: map[string]*schema.Schema{
					"id":         matchEntryID(),
					"file_type":  {Type: schema.TypeString, Required: true},
					"match_type": {Type: schema.TypeString, Optional: true, Default: "equal", Description: "sufx|prefx|equal|include|regex"},
					"case":       {Type: schema.TypeBool, Optional: true},
				}},
			},
			"header": {
				Type:     schema.TypeList,
				Optional: true,
				Elem: &schema.Resource{Schema: map[string]*schema.Schema{
					"id":              matchEntryID(),
					"name":            {Type: schema.TypeString, Required: true},
					"value":           {Type: schema.TypeString, Optional: true},
					"match_type_name": {Type: schema.TypeString, Optional: true, Default: "equal", Description: "equal|include|regex"},
					"match_type_val":  {Type: schema.TypeString, Optional: true, Default: "equal", Description: "equal|include|regex"},
					"case":            {Type: schema.TypeBool, Optional: true},
				}},
			},
			"cookie": {
				Type:     schema.TypeList,
				Optional: true,
				Elem: &schema.Resource{Schema: map[string]*schema.Schema{
					"id":             matchEntryID(),
					"key":            {Type: schema.TypeString, Required: true},
					"value":          {Type: schema.TypeString, Optional: true},
					"match_type_key": {Type: schema.TypeString, Optional: true, Default: "equal", Description: "equal|include|regex"},
					"match_type_val": {Type: schema.TypeString, Optional: true, Default: "equal", Description: "equal|include|regex"},
					"case":           {Type: schema.TypeBool, Optional: true},
				}},
			},
			"text": {
				Type:     schema.TypeList,
				Optional: true,
				Elem: &schema.Resource{Schema: map[string]*schema.Schema{
					"id":          matchEntryID(),
					"text":        {Type: schema.TypeString, Required: true},
					"match_type":  {Type: schema.TypeString, Optional: true, Default: "include", Description: "include|regex"},
					"lookup_area": {Type: schema.TypeString, Optional: true, Default: "both", Description: "header|body|both"},
					"case":        {Type: schema.TypeBool, Optional: true},
				}},
			},
			"xml": {
				Type:     schema.TypeList,
				Optional: true,
				Elem: &schema.Resource{Schema: map[string]*schema.Schema{
					"id":              matchEntryID(),
					"tag_name":        {Type: schema.TypeString, Required: true},
					"tag_value":       {Type: schema.TypeString, Optional: true},
					"match_type_name": {Type: schema.TypeString, Optional: true, Default: "equal", Description: "sufx|equal"},
					"match_type_val":  {Type: schema.TypeString, Optional: true, Default: "equal", Description: "sufx|equal|include"},
					"case":            {Type: schema.TypeBool, Optional: true},
				}},
			},
			"last_updated": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
		},
	}
}

func ccHeadPayload(d *schema.ResourceData) map[string]interface{} {
	p := map[string]interface{}{}
	if v, ok := d.GetOk("name"); ok {
		p["Name"] = v.(string)
	}
	if v, ok := d.GetOk("type"); ok {
		p["Type"] = v.(int)
	}
	if v, ok := d.GetOk("logical_expression"); ok {
		p["LogicalExpression"] = v.(string)
	}
	return p
}

// ccWriteMatchEntries schreibt alle nested Match-Eintraege in ihre Subtabellen.
func ccWriteMatchEntries(client *radwaregosdk.New_Client, ccID string, d *schema.ResourceData) diag.Diagnostics {
	var diags diag.Diagnostics

	for _, raw := range d.Get("hostname").([]interface{}) {
		e := raw.(map[string]interface{})
		p := map[string]interface{}{"HostName": e["host_name"].(string)}
		if n, ok := enumToInt(urlMatchType, e["match_type"].(string)); ok {
			p["MatchType"] = n
		}
		if v := e["data_class_id"].(string); v != "" {
			p["DataclassID"] = v
		}
		if dd := writeItem(client, configPath(ccHostNameTable, ccID+"/"+e["id"].(string)), p, true); dd.HasError() {
			return dd
		}
	}
	for _, raw := range d.Get("path").([]interface{}) {
		e := raw.(map[string]interface{})
		p := map[string]interface{}{"FilePath": e["file_path"].(string)}
		if n, ok := enumToInt(urlMatchType, e["match_type"].(string)); ok {
			p["MatchType"] = n
		}
		p["Case"] = boolToEnable(e["case"].(bool))
		if v := e["data_class_id"].(string); v != "" {
			p["DataclassID"] = v
		}
		if dd := writeItem(client, configPath(ccPathTable, ccID+"/"+e["id"].(string)), p, true); dd.HasError() {
			return dd
		}
	}
	for _, raw := range d.Get("filename").([]interface{}) {
		e := raw.(map[string]interface{})
		p := map[string]interface{}{"FileName": e["file_name"].(string)}
		if n, ok := enumToInt(urlMatchType, e["match_type"].(string)); ok {
			p["MatchType"] = n
		}
		p["Case"] = boolToEnable(e["case"].(bool))
		if dd := writeItem(client, configPath(ccFileNameTable, ccID+"/"+e["id"].(string)), p, true); dd.HasError() {
			return dd
		}
	}
	for _, raw := range d.Get("filetype").([]interface{}) {
		e := raw.(map[string]interface{})
		p := map[string]interface{}{"FileType": e["file_type"].(string)}
		if n, ok := enumToInt(urlMatchType, e["match_type"].(string)); ok {
			p["MatchType"] = n
		}
		p["Case"] = boolToEnable(e["case"].(bool))
		if dd := writeItem(client, configPath(ccFileTypeTable, ccID+"/"+e["id"].(string)), p, true); dd.HasError() {
			return dd
		}
	}
	for _, raw := range d.Get("header").([]interface{}) {
		e := raw.(map[string]interface{})
		p := map[string]interface{}{"Name": e["name"].(string)}
		if v := e["value"].(string); v != "" {
			p["Val"] = v
		}
		if n, ok := enumToInt(hdrMatchType, e["match_type_name"].(string)); ok {
			p["MatchTypeName"] = n
		}
		if n, ok := enumToInt(hdrMatchType, e["match_type_val"].(string)); ok {
			p["MatchTypeVal"] = n
		}
		p["Case"] = boolToEnable(e["case"].(bool))
		if dd := writeItem(client, configPath(ccHeaderTable, ccID+"/"+e["id"].(string)), p, true); dd.HasError() {
			return dd
		}
	}
	for _, raw := range d.Get("cookie").([]interface{}) {
		e := raw.(map[string]interface{})
		p := map[string]interface{}{"Key": e["key"].(string)}
		if v := e["value"].(string); v != "" {
			p["Val"] = v
		}
		if n, ok := enumToInt(hdrMatchType, e["match_type_key"].(string)); ok {
			p["MatchTypeKey"] = n
		}
		if n, ok := enumToInt(hdrMatchType, e["match_type_val"].(string)); ok {
			p["MatchTypeVal"] = n
		}
		p["Case"] = boolToEnable(e["case"].(bool))
		if dd := writeItem(client, configPath(ccCookieTable, ccID+"/"+e["id"].(string)), p, true); dd.HasError() {
			return dd
		}
	}
	for _, raw := range d.Get("text").([]interface{}) {
		e := raw.(map[string]interface{})
		p := map[string]interface{}{"Text": e["text"].(string)}
		if n, ok := enumToInt(textMatchType, e["match_type"].(string)); ok {
			p["MatchType"] = n
		}
		if n, ok := enumToInt(textLookup, e["lookup_area"].(string)); ok {
			p["LookupArea"] = n
		}
		p["Case"] = boolToEnable(e["case"].(bool))
		if dd := writeItem(client, configPath(ccTextTable, ccID+"/"+e["id"].(string)), p, true); dd.HasError() {
			return dd
		}
	}
	for _, raw := range d.Get("xml").([]interface{}) {
		e := raw.(map[string]interface{})
		p := map[string]interface{}{"TagName": e["tag_name"].(string)}
		if v := e["tag_value"].(string); v != "" {
			p["TagVal"] = v
		}
		if n, ok := enumToInt(xmlNameMatch, e["match_type_name"].(string)); ok {
			p["TagMatchTypeName"] = n
		}
		if n, ok := enumToInt(xmlValMatch, e["match_type_val"].(string)); ok {
			p["TagMatchTypeVal"] = n
		}
		p["TagCase"] = boolToEnable(e["case"].(bool))
		if dd := writeItem(client, configPath(ccXmlTable, ccID+"/"+e["id"].(string)), p, true); dd.HasError() {
			return dd
		}
	}
	return diags
}

func resource_alteon_content_class_create(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*radwaregosdk.New_Client)
	id := d.Get("id_name").(string)
	if diags := writeItem(client, configPath(ccTable, id), ccHeadPayload(d), true); diags.HasError() {
		return diags
	}
	d.SetId(id)
	if diags := ccWriteMatchEntries(client, id, d); diags.HasError() {
		return diags
	}
	return resource_alteon_content_class_read(ctx, d, m)
}

func resource_alteon_content_class_read(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*radwaregosdk.New_Client)
	id := d.Id()
	item, found, diags := readItem(client, configPath(ccTable, id), ccTable)
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
	if v, ok := item["Type"]; ok {
		d.Set("type", asInt(v))
	}
	if v, ok := item["LogicalExpression"]; ok {
		d.Set("logical_expression", asString(v))
	}
	// Match-Eintraege werden im State belassen wie konfiguriert (Kopf-Drift wird
	// erkannt; vollstaendiger Subtabellen-Read kann bei Bedarf ergaenzt werden).
	return diags
}

func resource_alteon_content_class_update(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*radwaregosdk.New_Client)
	id := d.Id()
	if diags := writeItem(client, configPath(ccTable, id), ccHeadPayload(d), false); diags.HasError() {
		return diags
	}
	// Entfernte Match-Eintraege loeschen (DELETE je Subtabelle), aktuelle neu schreiben.
	if diags := ccDeleteRemovedEntries(client, id, d); diags.HasError() {
		return diags
	}
	if diags := ccWriteMatchEntries(client, id, d); diags.HasError() {
		return diags
	}
	d.Set("last_updated", time.Now().Format(time.RFC3339))
	return resource_alteon_content_class_read(ctx, d, m)
}

// ccDeleteRemovedEntries loescht Match-Eintraege, die nicht mehr konfiguriert sind.
func ccDeleteRemovedEntries(client *radwaregosdk.New_Client, ccID string, d *schema.ResourceData) diag.Diagnostics {
	var diags diag.Diagnostics
	blocks := map[string]string{
		"hostname": ccHostNameTable,
		"path":     ccPathTable,
		"filename": ccFileNameTable,
		"filetype": ccFileTypeTable,
		"header":   ccHeaderTable,
		"cookie":   ccCookieTable,
		"text":     ccTextTable,
		"xml":      ccXmlTable,
	}
	for blockName, table := range blocks {
		if !d.HasChange(blockName) {
			continue
		}
		old, nw := d.GetChange(blockName)
		newIDs := map[string]bool{}
		for _, raw := range nw.([]interface{}) {
			newIDs[raw.(map[string]interface{})["id"].(string)] = true
		}
		for _, raw := range old.([]interface{}) {
			oid := raw.(map[string]interface{})["id"].(string)
			if !newIDs[oid] {
				if dd := deleteItem(client, configPath(table, ccID+"/"+oid)); dd.HasError() {
					return dd
				}
			}
		}
	}
	return diags
}

func resource_alteon_content_class_delete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*radwaregosdk.New_Client)
	if diags := deleteItem(client, configPath(ccTable, d.Id())); diags.HasError() {
		return diags
	}
	d.SetId("")
	return nil
}
