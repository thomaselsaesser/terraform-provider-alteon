package alteon

import (
	"context"
	"strconv"
	"time"

	radwaregosdk "github.com/Radware/radware_go_sdk"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// alteon_pip verwaltet eine Proxy-IP (pipNewCfgTable) inkl. ihrer Port- und/oder
// VLAN-Zuordnung. Key ist die PIP-Adresse selbst.
//
// Deklaratives Modell: ports/vlans im HCL beschreiben die VOLLSTAENDIGE Soll-Menge.
// Read dekodiert PortMap/VlanMap zur Ist-Menge; Create/Update berechnen das Delta
// und feuern die noetigen AddPort/RemovePort/AddVlan/RemoveVlan-Aufrufe.
//
// IPv4 jetzt (pipNewCfgTable). Das Schema ist v6-faehig vorbereitet; der v6-Pfad
// (pip6NewCfgTable) wird ergaenzt, sobald an einem v6-Geraet getestet werden kann.
//
// HINWEIS: Die Bit->Nummer-Konvention der PortMap/VlanMap (MSB-first, 1-basiert)
// ist die uebliche Alteon-Konvention, sollte aber am Geraet gegen ein bekanntes
// Beispiel verifiziert werden (siehe decodeHexBitmap in helpers.go).

const pipTable = "pipNewCfgTable"

func resource_alteon_pip() *schema.Resource {
	return &schema.Resource{
		CreateContext: resource_alteon_pip_create,
		ReadContext:   resource_alteon_pip_read,
		UpdateContext: resource_alteon_pip_update,
		DeleteContext: resource_alteon_pip_delete,
		Importer:      &schema.ResourceImporter{StateContext: schema.ImportStatePassthroughContext},
		Schema: map[string]*schema.Schema{
			"address": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "The proxy IP address (table key). IPv4.",
			},
			"ports": {
				Type:        schema.TypeSet,
				Optional:    true,
				Elem:        &schema.Schema{Type: schema.TypeInt},
				Description: "Complete set of port numbers this PIP is assigned to (declarative).",
			},
			"vlans": {
				Type:        schema.TypeSet,
				Optional:    true,
				Elem:        &schema.Schema{Type: schema.TypeInt},
				Description: "Complete set of VLAN IDs this PIP is assigned to (declarative).",
			},
			"last_updated": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
		},
	}
}

// pipReadMaps liest die aktuellen Port-/VLAN-Mengen vom Geraet.
func pipReadMaps(client *radwaregosdk.New_Client, addr string) (ports, vlans []int, found bool, diags diag.Diagnostics) {
	api := configPath(pipTable, addr)
	item, ok, d := readItem(client, api, pipTable)
	if d.HasError() || !ok {
		return nil, nil, ok, d
	}
	if v, ok := item["PortMap"]; ok {
		ports = decodeHexBitmap(asString(v), 1)
	}
	if v, ok := item["VlanMap"]; ok {
		vlans = decodeHexBitmap(asString(v), 0)
	}
	return ports, vlans, true, diags
}

// pipApplyDelta bringt die Geraete-Mengen auf den gewuenschten Stand.
func pipApplyDelta(client *radwaregosdk.New_Client, addr string, wantPorts, wantVlans []int) diag.Diagnostics {
	var diags diag.Diagnostics
	api := configPath(pipTable, addr)

	havePorts, haveVlans, _, d := pipReadMaps(client, addr)
	if d.HasError() {
		return d
	}

	addP, remP := setDelta(wantPorts, havePorts)
	addV, remV := setDelta(wantVlans, haveVlans)

	// Jede Add/Rem-Operation ist ein eigener PUT auf die PIP-Zeile.
	for _, p := range addP {
		if dd := writeItem(client, api, map[string]interface{}{"AddPort": p}, false); dd.HasError() {
			return dd
		}
	}
	for _, p := range remP {
		if dd := writeItem(client, api, map[string]interface{}{"RemovePort": p}, false); dd.HasError() {
			return dd
		}
	}
	for _, v := range addV {
		if dd := writeItem(client, api, map[string]interface{}{"AddVlan": v}, false); dd.HasError() {
			return dd
		}
	}
	for _, v := range remV {
		if dd := writeItem(client, api, map[string]interface{}{"RemoveVlan": v}, false); dd.HasError() {
			return dd
		}
	}
	return diags
}

func resource_alteon_pip_create(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*radwaregosdk.New_Client)
	addr := d.Get("address").(string)
	api := configPath(pipTable, addr)

	// PIP-Zeile anlegen (POST). Die Adresse ist der Key; ein leerer Body genuegt,
	// die Zuordnung folgt ueber die Delta-Aufrufe.
	if diags := writeItem(client, api, map[string]interface{}{}, true); diags.HasError() {
		return diags
	}
	d.SetId(addr)

	wantPorts := interfaceListToInts(d.Get("ports").(*schema.Set).List())
	wantVlans := interfaceListToInts(d.Get("vlans").(*schema.Set).List())
	if diags := pipApplyDelta(client, addr, wantPorts, wantVlans); diags.HasError() {
		return diags
	}
	return resource_alteon_pip_read(ctx, d, m)
}

func resource_alteon_pip_read(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*radwaregosdk.New_Client)
	ports, vlans, found, diags := pipReadMaps(client, d.Id())
	if diags.HasError() {
		return diags
	}
	if !found {
		d.SetId("")
		return diags
	}
	d.Set("address", d.Id())
	d.Set("ports", ports)
	d.Set("vlans", vlans)
	return diags
}

func resource_alteon_pip_update(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*radwaregosdk.New_Client)
	addr := d.Id()
	wantPorts := interfaceListToInts(d.Get("ports").(*schema.Set).List())
	wantVlans := interfaceListToInts(d.Get("vlans").(*schema.Set).List())
	if diags := pipApplyDelta(client, addr, wantPorts, wantVlans); diags.HasError() {
		return diags
	}
	d.Set("last_updated", time.Now().Format(time.RFC3339))
	return resource_alteon_pip_read(ctx, d, m)
}

func resource_alteon_pip_delete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*radwaregosdk.New_Client)
	api := configPath(pipTable, d.Id())
	if diags := deleteItem(client, api); diags.HasError() {
		return diags
	}
	d.SetId("")
	return nil
}

// --- Peer-PIP (slbNewCfgPeerPIPTable) ----------------------------------------
// Flache Tabelle, Key = PIPIndex (int). v4 und v6 in einer Tabelle ueber PIPVersion.

const peerPipTable = "slbNewCfgPeerPIPTable"

func resource_alteon_peer_pip() *schema.Resource {
	return &schema.Resource{
		CreateContext: resource_alteon_peer_pip_create,
		ReadContext:   resource_alteon_peer_pip_read,
		UpdateContext: resource_alteon_peer_pip_update,
		DeleteContext: resource_alteon_peer_pip_delete,
		Importer:      &schema.ResourceImporter{StateContext: schema.ImportStatePassthroughContext},
		Schema: map[string]*schema.Schema{
			"index": {
				Type:        schema.TypeInt,
				Required:    true,
				ForceNew:    true,
				Description: "Peer PIP table index (PIPIndex).",
			},
			"address": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Peer PIP IPv4 address (PIPAddr).",
			},
			"ipv6_address": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Peer PIP IPv6 address (PIPv6Addr).",
			},
			"version": {
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Description: "IP version: \"v4\" or \"v6\".",
			},
			"last_updated": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
		},
	}
}

func peerPipPayload(d *schema.ResourceData) map[string]interface{} {
	p := map[string]interface{}{"PIPIndex": d.Get("index").(int)}
	if v, ok := d.GetOk("address"); ok {
		p["PIPAddr"] = v.(string)
	}
	if v, ok := d.GetOk("ipv6_address"); ok {
		p["PIPv6Addr"] = v.(string)
	}
	if v, ok := d.GetOk("version"); ok {
		if v.(string) == "v6" {
			p["PIPVersion"] = 6
		} else {
			p["PIPVersion"] = 4
		}
	}
	return p
}

func resource_alteon_peer_pip_create(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*radwaregosdk.New_Client)
	key := strconv.Itoa(d.Get("index").(int))
	api := configPath(peerPipTable, key)
	if diags := writeItem(client, api, peerPipPayload(d), true); diags.HasError() {
		return diags
	}
	d.SetId(key)
	return resource_alteon_peer_pip_read(ctx, d, m)
}

func resource_alteon_peer_pip_read(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*radwaregosdk.New_Client)
	api := configPath(peerPipTable, d.Id())
	item, found, diags := readItem(client, api, peerPipTable)
	if diags.HasError() {
		return diags
	}
	if !found {
		d.SetId("")
		return diags
	}
	if v, ok := item["PIPIndex"]; ok {
		d.Set("index", asInt(v))
	}
	if v, ok := item["PIPAddr"]; ok {
		d.Set("address", asString(v))
	}
	if v, ok := item["PIPv6Addr"]; ok {
		d.Set("ipv6_address", asString(v))
	}
	if v, ok := item["PIPVersion"]; ok {
		if asInt(v) == 6 {
			d.Set("version", "v6")
		} else {
			d.Set("version", "v4")
		}
	}
	return diags
}

func resource_alteon_peer_pip_update(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*radwaregosdk.New_Client)
	api := configPath(peerPipTable, d.Id())
	if diags := writeItem(client, api, peerPipPayload(d), false); diags.HasError() {
		return diags
	}
	d.Set("last_updated", time.Now().Format(time.RFC3339))
	return resource_alteon_peer_pip_read(ctx, d, m)
}

func resource_alteon_peer_pip_delete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*radwaregosdk.New_Client)
	api := configPath(peerPipTable, d.Id())
	if diags := deleteItem(client, api); diags.HasError() {
		return diags
	}
	d.SetId("")
	return nil
}
