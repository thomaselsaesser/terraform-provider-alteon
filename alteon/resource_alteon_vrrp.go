package alteon

import (
	"context"
	"strconv"
	"time"

	radwaregosdk "github.com/Radware/radware_go_sdk"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// alteon_vrrp verwaltet einen einzelnen VRRP Virtual Router (/c/l3/vrrp/vr <n>)
// ueber die REST-Tabelle vrrpNewCfgVirtRtrTable.
//
// Designentscheidungen (mit Thomas abgestimmt):
//   - index (Indx, Tabellen-Key) und vrid (ID, die VRID auf dem Draht) sind GETRENNTE
//     Pflichtfelder, weil sie bei animate historisch teils auseinanderlaufen.
//   - index wird explizit im HCL vergeben (keine Auto-Vergabe).
//   - 1/2-Enums (preempt, state, sharing, alle track_*) sind im Schema bool.
//   - Echter Read fuer Drift-Detection (anders als der alte cli_command-Ansatz).

const vrrpVirtRtrTable = "vrrpNewCfgVirtRtrTable"

func resource_alteon_vrrp() *schema.Resource {
	return &schema.Resource{
		CreateContext: resource_alteon_vrrp_create,
		ReadContext:   resource_alteon_vrrp_read,
		UpdateContext: resource_alteon_vrrp_update,
		DeleteContext: resource_alteon_vrrp_delete,
		Importer:      &schema.ResourceImporter{StateContext: schema.ImportStatePassthroughContext},
		Schema: map[string]*schema.Schema{
			"index": {
				Type:        schema.TypeInt,
				Required:    true,
				ForceNew:    true,
				Description: "Table key (Indx) -- the VR slot/number (/c/l3/vrrp/vr <index>). Set explicitly.",
			},
			"vrid": {
				Type:        schema.TypeInt,
				Required:    true,
				Description: "The Virtual Router ID (ID) carried in VRRP advertisements. May differ from index.",
			},
			"addr": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "The IPv4 address of the virtual router.",
			},
			"if_index": {
				Type:        schema.TypeInt,
				Optional:    true,
				Description: "The IP interface (IfIndex) the virtual router is attached to.",
			},
			"interval": {
				Type:        schema.TypeInt,
				Optional:    true,
				Computed:    true,
				Description: "The advertisement interval in seconds (IPv4).",
			},
			"priority": {
				Type:        schema.TypeInt,
				Optional:    true,
				Computed:    true,
				Description: "The virtual router priority.",
			},
			"preempt": {
				Type:        schema.TypeBool,
				Optional:    true,
				Computed:    true,
				Description: "Enable preemption.",
			},
			"state": {
				Type:        schema.TypeBool,
				Optional:    true,
				Computed:    true,
				Description: "Enable (true) or disable (false) the virtual router.",
			},
			"sharing": {
				Type:        schema.TypeBool,
				Optional:    true,
				Computed:    true,
				Description: "Enable VRRP sharing.",
			},
			"version": {
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Description: "IP version of the virtual router: \"v4\" or \"v6\".",
			},
			"ipv6_addr": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "The IPv6 address of the virtual router (when version = v6).",
			},
			"ipv6_interval": {
				Type:        schema.TypeInt,
				Optional:    true,
				Computed:    true,
				Description: "The advertisement interval in seconds (IPv6).",
			},
			"ospf_cost": {
				Type:        schema.TypeInt,
				Optional:    true,
				Computed:    true,
				Description: "OSPF cost adjustment applied while this VR is master.",
			},
			// Tracking-Flags (alle 1=enabled / 2=disabled -> bool):
			"track_virt_rtr": {
				Type:        schema.TypeBool,
				Optional:    true,
				Computed:    true,
				Description: "Track other virtual routers (TckVirtRtr).",
			},
			"track_ip_intf": {
				Type:        schema.TypeBool,
				Optional:    true,
				Computed:    true,
				Description: "Track IP interfaces (TckIpIntf).",
			},
			"track_vlan_port": {
				Type:        schema.TypeBool,
				Optional:    true,
				Computed:    true,
				Description: "Track VLAN ports (TckVlanPort).",
			},
			"track_l4_port": {
				Type:        schema.TypeBool,
				Optional:    true,
				Computed:    true,
				Description: "Track SLB (Layer 4) ports (TckL4Port).",
			},
			"track_real_server": {
				Type:        schema.TypeBool,
				Optional:    true,
				Computed:    true,
				Description: "Track real servers (TckRServer).",
			},
			"track_hsrp": {
				Type:        schema.TypeBool,
				Optional:    true,
				Computed:    true,
				Description: "Track HSRP (TckHsrp).",
			},
			"track_hsrv": {
				Type:        schema.TypeBool,
				Optional:    true,
				Computed:    true,
				Description: "Track HSRP with VLAN (TckHsrv).",
			},
			"track_sw_ext": {
				Type:        schema.TypeBool,
				Optional:    true,
				Computed:    true,
				Description: "Track switch external (TckSwExt).",
			},
			"track_isl_port_include": {
				Type:        schema.TypeBool,
				Optional:    true,
				Computed:    true,
				Description: "ISL port tracking: include (true) or exclude (false) (TckIslPort).",
			},
			"last_updated": {
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Description: "Resource last updated time.",
			},
		},
	}
}

// vrrpPayload baut die JSON-Felder aus dem Schema. includeKeys steuert, ob ID/Indx
// mitgesendet werden (bei POST ja, bei PUT genuegt der Pfad, schadet aber nicht).
func vrrpPayload(d *schema.ResourceData) map[string]interface{} {
	p := map[string]interface{}{
		"Indx": d.Get("index").(int),
		"ID":   d.Get("vrid").(int),
	}
	if v, ok := d.GetOk("addr"); ok {
		p["Addr"] = v.(string)
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
	if v, ok := d.GetOk("ipv6_addr"); ok {
		p["Ipv6Addr"] = v.(string)
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
	// Bool-Felder: nur senden, wenn explizit gesetzt (GetOkExists), damit wir
	// nicht ungewollt Defaults ueberschreiben.
	boolFields := map[string]string{
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
	for schemaKey, apiKey := range boolFields {
		if v, ok := d.GetOkExists(schemaKey); ok {
			p[apiKey] = boolToEnable(v.(bool))
		}
	}
	return p
}

func resource_alteon_vrrp_create(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*radwaregosdk.New_Client)
	key := strconv.Itoa(d.Get("index").(int))
	api := configPath(vrrpVirtRtrTable, key)

	if diags := writeItem(client, api, vrrpPayload(d), true); diags.HasError() {
		return diags
	}
	d.SetId(key)
	return resource_alteon_vrrp_read(ctx, d, m)
}

func resource_alteon_vrrp_read(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*radwaregosdk.New_Client)
	key := d.Id()
	api := configPath(vrrpVirtRtrTable, key)

	item, found, diags := readItem(client, api, vrrpVirtRtrTable)
	if diags.HasError() {
		return diags
	}
	if !found {
		d.SetId("") // Objekt existiert nicht mehr -> aus dem State entfernen.
		return diags
	}

	if v, ok := item["Indx"]; ok {
		d.Set("index", asInt(v))
	}
	if v, ok := item["ID"]; ok {
		d.Set("vrid", asInt(v))
	}
	if v, ok := item["Addr"]; ok {
		d.Set("addr", asString(v))
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
	if v, ok := item["Ipv6Addr"]; ok {
		d.Set("ipv6_addr", asString(v))
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
	boolFields := map[string]string{
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
	for schemaKey, apiKey := range boolFields {
		if v, ok := item[apiKey]; ok {
			d.Set(schemaKey, enableToBool(v))
		}
	}
	return diags
}

func resource_alteon_vrrp_update(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*radwaregosdk.New_Client)
	key := d.Id()
	api := configPath(vrrpVirtRtrTable, key)

	if diags := writeItem(client, api, vrrpPayload(d), false); diags.HasError() {
		return diags
	}
	d.Set("last_updated", time.Now().Format(time.RFC3339))
	return resource_alteon_vrrp_read(ctx, d, m)
}

func resource_alteon_vrrp_delete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*radwaregosdk.New_Client)
	api := configPath(vrrpVirtRtrTable, d.Id())

	if diags := deleteItem(client, api); diags.HasError() {
		return diags
	}
	d.SetId("")
	return nil
}
