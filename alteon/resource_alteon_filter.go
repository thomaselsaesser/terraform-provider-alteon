package alteon

import (
	"context"
	"strconv"
	"time"

	radwaregosdk "github.com/Radware/radware_go_sdk"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// alteon_filter verwaltet einen Filter (fltNewCfgTable). Key: Indx (int).
//
// Sinnvolle Teilmenge der sehr grossen Tabelle: L3/L4-Matching, Action, NAT,
// Redirect, Content-Class-Bezug, State/Log/Name. KEIN L2, KEINE Pattern-Match-Group
// (wird bei animate nicht genutzt).
//
// action-Enum:  allow|deny|redirect|nat|goto|outbound-llb|monitor
// nat-Enum:     destination-address|source-address|multicast-address

const filterTable = "fltNewCfgTable"

var filterAction = map[string]int{
	"allow": 1, "deny": 2, "redirect": 3, "nat": 4, "goto": 5, "outbound-llb": 6, "monitor": 7,
}
var filterNat = map[string]int{
	"destination-address": 1, "source-address": 2, "multicast-address": 3,
}

func resource_alteon_filter() *schema.Resource {
	return &schema.Resource{
		CreateContext: resource_alteon_filter_create,
		ReadContext:   resource_alteon_filter_read,
		UpdateContext: resource_alteon_filter_update,
		DeleteContext: resource_alteon_filter_delete,
		Importer:      &schema.ResourceImporter{StateContext: schema.ImportStatePassthroughContext},
		Schema: map[string]*schema.Schema{
			"index":         {Type: schema.TypeInt, Required: true, ForceNew: true, Description: "Filter index (Indx)."},
			"name":          {Type: schema.TypeString, Optional: true},
			"src_ip":        {Type: schema.TypeString, Optional: true, Description: "Source IP (SrcIp)."},
			"src_mask":      {Type: schema.TypeString, Optional: true, Description: "Source IP mask (SrcIpMask)."},
			"dst_ip":        {Type: schema.TypeString, Optional: true, Description: "Destination IP (DstIp)."},
			"dst_mask":      {Type: schema.TypeString, Optional: true, Description: "Destination IP mask (DstIpMask)."},
			"protocol":      {Type: schema.TypeInt, Optional: true, Description: "IP protocol number."},
			"src_port_low":  {Type: schema.TypeInt, Optional: true, Description: "Source port range low."},
			"src_port_high": {Type: schema.TypeInt, Optional: true, Description: "Source port range high."},
			"dst_port_low":  {Type: schema.TypeInt, Optional: true, Description: "Destination port range low."},
			"dst_port_high": {Type: schema.TypeInt, Optional: true, Description: "Destination port range high."},
			"action": {Type: schema.TypeString, Optional: true, Computed: true,
				Description: "allow|deny|redirect|nat|goto|outbound-llb|monitor"},
			"redirect_port":  {Type: schema.TypeInt, Optional: true, Description: "Redirect port (RedirPort)."},
			"redirect_group": {Type: schema.TypeString, Optional: true, Description: "Redirect server group (RedirGroup)."},
			"nat": {Type: schema.TypeString, Optional: true,
				Description: "destination-address|source-address|multicast-address"},
			"goto_filter": {Type: schema.TypeInt, Optional: true, Description: "Target filter for action=goto (GotoFilter)."},
			"vlan":        {Type: schema.TypeInt, Optional: true, Description: "VLAN to match."},
			"invert":      {Type: schema.TypeBool, Optional: true, Computed: true, Description: "Invert match."},
			"log":         {Type: schema.TypeBool, Optional: true, Computed: true, Description: "Enable logging."},
			"state":       {Type: schema.TypeBool, Optional: true, Computed: true, Description: "Enable (true) / disable (false) the filter."},
			"content_class": {Type: schema.TypeString, Optional: true,
				Description: "Associated content class ID (Layer7 deny / content switching)."},
			"last_updated": {Type: schema.TypeString, Optional: true, Computed: true},
		},
	}
}

func filterPayload(d *schema.ResourceData) map[string]interface{} {
	p := map[string]interface{}{"Indx": d.Get("index").(int)}
	str := map[string]string{
		"name": "Name", "src_ip": "SrcIp", "src_mask": "SrcIpMask",
		"dst_ip": "DstIp", "dst_mask": "DstIpMask", "redirect_group": "RedirGroup",
	}
	for sk, ak := range str {
		if v, ok := d.GetOk(sk); ok {
			p[ak] = v.(string)
		}
	}
	ints := map[string]string{
		"protocol": "Protocol", "src_port_low": "RangeLowSrcPort", "src_port_high": "RangeHighSrcPort",
		"dst_port_low": "RangeLowDstPort", "dst_port_high": "RangeHighDstPort",
		"redirect_port": "RedirPort", "goto_filter": "GotoFilter", "vlan": "Vlan",
	}
	for sk, ak := range ints {
		if v, ok := d.GetOk(sk); ok {
			p[ak] = v.(int)
		}
	}
	if v, ok := d.GetOk("action"); ok {
		if n, found := filterAction[v.(string)]; found {
			p["Action"] = n
		}
	}
	if v, ok := d.GetOk("nat"); ok {
		if n, found := filterNat[v.(string)]; found {
			p["Nat"] = n
		}
	}
	bools := map[string]string{"invert": "Invert", "log": "Log", "state": "State"}
	for sk, ak := range bools {
		if v, ok := d.GetOkExists(sk); ok {
			p[ak] = boolToEnable(v.(bool))
		}
	}
	// Content-Class-Bezug: bei aktiviertem Layer7-Deny ueber Add-URL; hier setzen
	// wir den einfachen Fall (Zuordnung) -- erweiterbar je nach Nutzung.
	if v, ok := d.GetOk("content_class"); ok {
		p["Layer7DenyAddUrl"] = v.(string)
	}
	return p
}

func resource_alteon_filter_create(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*radwaregosdk.New_Client)
	key := strconv.Itoa(d.Get("index").(int))
	if diags := writeItem(client, configPath(filterTable, key), filterPayload(d), true); diags.HasError() {
		return diags
	}
	d.SetId(key)
	return resource_alteon_filter_read(ctx, d, m)
}

func resource_alteon_filter_read(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*radwaregosdk.New_Client)
	item, found, diags := readItem(client, configPath(filterTable, d.Id()), filterTable)
	if diags.HasError() {
		return diags
	}
	if !found {
		d.SetId("")
		return diags
	}
	if v, ok := item["Indx"]; ok {
		d.Set("index", asInt(v))
	}
	strMap := map[string]string{
		"Name": "name", "SrcIp": "src_ip", "SrcIpMask": "src_mask",
		"DstIp": "dst_ip", "DstIpMask": "dst_mask", "RedirGroup": "redirect_group",
	}
	for ak, sk := range strMap {
		if v, ok := item[ak]; ok {
			d.Set(sk, asString(v))
		}
	}
	intMap := map[string]string{
		"Protocol": "protocol", "RangeLowSrcPort": "src_port_low", "RangeHighSrcPort": "src_port_high",
		"RangeLowDstPort": "dst_port_low", "RangeHighDstPort": "dst_port_high",
		"RedirPort": "redirect_port", "GotoFilter": "goto_filter", "Vlan": "vlan",
	}
	for ak, sk := range intMap {
		if v, ok := item[ak]; ok {
			d.Set(sk, asInt(v))
		}
	}
	if v, ok := item["Action"]; ok {
		d.Set("action", intToEnum(filterAction, asInt(v)))
	}
	if v, ok := item["Nat"]; ok {
		d.Set("nat", intToEnum(filterNat, asInt(v)))
	}
	for ak, sk := range map[string]string{"Invert": "invert", "Log": "log", "State": "state"} {
		if v, ok := item[ak]; ok {
			d.Set(sk, enableToBool(v))
		}
	}
	return diags
}

func resource_alteon_filter_update(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*radwaregosdk.New_Client)
	if diags := writeItem(client, configPath(filterTable, d.Id()), filterPayload(d), false); diags.HasError() {
		return diags
	}
	d.Set("last_updated", time.Now().Format(time.RFC3339))
	return resource_alteon_filter_read(ctx, d, m)
}

func resource_alteon_filter_delete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*radwaregosdk.New_Client)
	if diags := deleteItem(client, configPath(filterTable, d.Id())); diags.HasError() {
		return diags
	}
	d.SetId("")
	return nil
}

// --- alteon_filter_port (fltNewCfgPortTable) ---------------------------------
// Deklarative Zuordnung von Filter-Regeln zu einem Port. Key: Port-Index (Indx).
// FiltBmap (Ist) -> AddFiltRule/RemFiltRule (Delta).

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
		rules = decodeHexBitmap(asString(v))
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
