package alteon

import (
	"context"
	"strconv"
	"time"

	radwaregosdk "github.com/Radware/radware_go_sdk"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// alteon_vrrp_group verwaltet eine Switch-Based VRRP Group (/c/l3/vrrp/group <n>)
// ueber die REST-Tabelle vrrpNewCfgVirtRtrGrpTable.
//
// Feldgleich mit alteon_vrrp, jedoch OHNE addr/ipv6_addr -- die Gruppe traegt keine
// eigene IP, sondern wirkt geraeteweit fuer das gesamte HA-Paar.

const vrrpVirtRtrGrpTable = "vrrpNewCfgVirtRtrGrpTable"
const vrGrpMemberTable = "vrrpNewCfgVirtRtrVrGrpTable"

func resource_alteon_vrrp_group() *schema.Resource {
	return &schema.Resource{
		CreateContext: resource_alteon_vrrp_group_create,
		ReadContext:   resource_alteon_vrrp_group_read,
		UpdateContext: resource_alteon_vrrp_group_update,
		DeleteContext: resource_alteon_vrrp_group_delete,
		Importer:      &schema.ResourceImporter{StateContext: schema.ImportStatePassthroughContext},
		Schema: map[string]*schema.Schema{
			"index": {
				Type:        schema.TypeInt,
				Required:    true,
				ForceNew:    true,
				Description: "Table key (Indx) -- the VR group slot/number. Set explicitly.",
			},
			"vrid": {
				Type:        schema.TypeInt,
				Required:    true,
				Description: "The Virtual Router ID (ID) for the group. May differ from index.",
			},
			"virtual_routers": {
				Type:        schema.TypeSet,
				Optional:    true,
				Elem:        &schema.Schema{Type: schema.TypeInt},
				Description: "Complete set of Virtual Router indices that are members of this group (declarative). Uses Bmap/Add/Rem on vrrpNewCfgVirtRtrVrGrpTable.",
			},
			"if_index": {
				Type:        schema.TypeInt,
				Optional:    true,
				Description: "The IP interface (IfIndex) the group is attached to.",
			},
			"interval": {
				Type:     schema.TypeInt,
				Optional: true,
				Computed: true,
			},
			"priority": {
				Type:     schema.TypeInt,
				Optional: true,
				Computed: true,
			},
			"preempt": {
				Type:     schema.TypeBool,
				Optional: true,
				Computed: true,
			},
			"state": {
				Type:        schema.TypeBool,
				Optional:    true,
				Computed:    true,
				Description: "Enable (true) or disable (false) the VR group.",
			},
			"sharing": {
				Type:     schema.TypeBool,
				Optional: true,
				Computed: true,
			},
			"version": {
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Description: "IP version: \"v4\" or \"v6\".",
			},
			"ipv6_interval": {
				Type:     schema.TypeInt,
				Optional: true,
				Computed: true,
			},
			"ospf_cost": {
				Type:     schema.TypeInt,
				Optional: true,
				Computed: true,
			},
			"track_virt_rtr":    {Type: schema.TypeBool, Optional: true, Computed: true},
			"track_ip_intf":     {Type: schema.TypeBool, Optional: true, Computed: true},
			"track_vlan_port":   {Type: schema.TypeBool, Optional: true, Computed: true},
			"track_l4_port":     {Type: schema.TypeBool, Optional: true, Computed: true},
			"track_real_server": {Type: schema.TypeBool, Optional: true, Computed: true},
			"track_hsrp":        {Type: schema.TypeBool, Optional: true, Computed: true},
			"track_hsrv":        {Type: schema.TypeBool, Optional: true, Computed: true},
			"track_sw_ext":      {Type: schema.TypeBool, Optional: true, Computed: true},
			"track_isl_port_include": {
				Type:        schema.TypeBool,
				Optional:    true,
				Computed:    true,
				Description: "ISL port tracking: include (true) or exclude (false).",
			},
			"last_updated": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
		},
	}
}

var vrrpGroupBoolFields = map[string]string{
	"preempt":                "Preempt",
	"state":                  "State",
	"sharing":                "Sharing",
	"track_virt_rtr":         "TckVirtRtr",
	"track_ip_intf":          "TckIpIntf",
	"track_vlan_port":        "TckVlanPort",
	"track_l4_port":          "TckL4Port",
	"track_real_server":      "TckRServer",
	"track_hsrp":             "TckHsrp",
	"track_hsrv":             "TckHsrv",
	"track_sw_ext":           "TckSwExt",
	"track_isl_port_include": "TckIslPort",
}

// vrGrpReadMembers liest die Ist-Mitglieder aus dem Bmap der Member-Tabelle.
func vrGrpReadMembers(client *radwaregosdk.New_Client, groupKey string) ([]int, diag.Diagnostics) {
	api := configPath(vrGrpMemberTable, groupKey)
	item, found, diags := readItem(client, api, vrGrpMemberTable)
	if diags.HasError() || !found {
		return nil, diags
	}
	if v, ok := item["Bmap"]; ok {
		return decodeHexBitmap(asString(v)), nil
	}
	return nil, diags
}

// vrGrpApplyMemberDelta bringt die VR-Mitgliedschaft auf den Soll-Stand.
func vrGrpApplyMemberDelta(client *radwaregosdk.New_Client, groupKey string, want []int) diag.Diagnostics {
	have, diags := vrGrpReadMembers(client, groupKey)
	if diags.HasError() {
		return diags
	}
	add, rem := setDelta(want, have)
	api := configPath(vrGrpMemberTable, groupKey)
	for _, v := range add {
		if dd := writeItem(client, api, map[string]interface{}{"Add": v}, false); dd.HasError() {
			return dd
		}
	}
	for _, v := range rem {
		if dd := writeItem(client, api, map[string]interface{}{"Rem": v}, false); dd.HasError() {
			return dd
		}
	}
	return nil
}

func vrrpGroupPayload(d *schema.ResourceData) map[string]interface{} {
	p := map[string]interface{}{
		"Indx": d.Get("index").(int),
		"ID":   d.Get("vrid").(int),
	}
	if v, ok := d.GetOk("if_index"); ok {
		p["IfIndex"] = v.(int)
	}
	if v, ok := d.GetOk("interval"); ok {
		p["Interval"] = v.(int)
	}
	if v, ok := d.GetOk("priority"); ok {
		p["Priority"] = v.(int)
	}
	if v, ok := d.GetOk("ipv6_interval"); ok {
		p["Ipv6Interval"] = v.(int)
	}
	if v, ok := d.GetOk("ospf_cost"); ok {
		p["OspfCost"] = v.(int)
	}
	if v, ok := d.GetOk("version"); ok {
		if v.(string) == "v6" {
			p["Version"] = 2
		} else {
			p["Version"] = 1
		}
	}
	for schemaKey, apiKey := range vrrpGroupBoolFields {
		if v, ok := d.GetOkExists(schemaKey); ok {
			p[apiKey] = boolToEnable(v.(bool))
		}
	}
	return p
}

func resource_alteon_vrrp_group_create(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*radwaregosdk.New_Client)
	key := strconv.Itoa(d.Get("index").(int))
	api := configPath(vrrpVirtRtrGrpTable, key)

	if diags := writeItem(client, api, vrrpGroupPayload(d), true); diags.HasError() {
		return diags
	}
	d.SetId(key)

	// VR-Mitglieder hinzufuegen.
	if v, ok := d.GetOk("virtual_routers"); ok {
		want := interfaceListToInts(v.(*schema.Set).List())
		if diags := vrGrpApplyMemberDelta(client, key, want); diags.HasError() {
			return diags
		}
	}

	return resource_alteon_vrrp_group_read(ctx, d, m)
}

func resource_alteon_vrrp_group_read(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*radwaregosdk.New_Client)
	api := configPath(vrrpVirtRtrGrpTable, d.Id())

	item, found, diags := readItem(client, api, vrrpVirtRtrGrpTable)
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
	if v, ok := item["ID"]; ok {
		d.Set("vrid", asInt(v))
	}
	if v, ok := item["IfIndex"]; ok {
		d.Set("if_index", asInt(v))
	}
	if v, ok := item["Interval"]; ok {
		d.Set("interval", asInt(v))
	}
	if v, ok := item["Priority"]; ok {
		d.Set("priority", asInt(v))
	}
	if v, ok := item["Ipv6Interval"]; ok {
		d.Set("ipv6_interval", asInt(v))
	}
	if v, ok := item["OspfCost"]; ok {
		d.Set("ospf_cost", asInt(v))
	}
	if v, ok := item["Version"]; ok {
		if asInt(v) == 2 {
			d.Set("version", "v6")
		} else {
			d.Set("version", "v4")
		}
	}
	for schemaKey, apiKey := range vrrpGroupBoolFields {
		if v, ok := item[apiKey]; ok {
			d.Set(schemaKey, enableToBool(v))
		}
	}
	// VR-Mitglieder aus der Member-Tabelle lesen.
	members, mdiags := vrGrpReadMembers(client, d.Id())
	if mdiags.HasError() {
		return mdiags
	}
	d.Set("virtual_routers", members)

	return diags
}

func resource_alteon_vrrp_group_update(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*radwaregosdk.New_Client)
	api := configPath(vrrpVirtRtrGrpTable, d.Id())

	if diags := writeItem(client, api, vrrpGroupPayload(d), false); diags.HasError() {
		return diags
	}
	// VR-Mitglieder aktualisieren.
	if d.HasChange("virtual_routers") {
		want := interfaceListToInts(d.Get("virtual_routers").(*schema.Set).List())
		if diags := vrGrpApplyMemberDelta(client, d.Id(), want); diags.HasError() {
			return diags
		}
	}
	d.Set("last_updated", time.Now().Format(time.RFC3339))
	return resource_alteon_vrrp_group_read(ctx, d, m)
}

func resource_alteon_vrrp_group_delete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*radwaregosdk.New_Client)
	api := configPath(vrrpVirtRtrGrpTable, d.Id())

	if diags := deleteItem(client, api); diags.HasError() {
		return diags
	}
	d.SetId("")
	return nil
}
