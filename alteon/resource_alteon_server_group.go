package alteon

import (
	"context"
	"time"

	radwaregosdk "github.com/Radware/radware_go_sdk"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// alteon_server_group -- NEUES deklaratives Modell (sauberer Schnitt, ersetzt das
// alte elements/Add-Rem-Schema vollstaendig).
//
// Eine Gruppe = EINE Ressource. Die Mitglieder werden als vollstaendige Soll-Menge
// im Feld `servers` beschrieben (wie bei alteon_pip ports/vlans). Gruppeneinstellungen
// (name, metric, ...) sind flache Top-Level-Felder.
//
// Tabellen:
//   slbNewCfgEnhGroupTable            -- Kopf (Key Index); AddServer/RemoveServer Kommandos
//   slbNewCfgEnhGroupRealServerTable  -- Mitglieder (Key RealServGroupIndex/ServIndex, Feld State)
//
// Read: Ist-Mitglieder aus der Member-Tabelle (clientseitig nach Gruppe gefiltert).
// Update: Delta gegen `servers` -> AddServer/RemoveServer auf der Kopf-Tabelle.
//
// HINWEIS (am Geraet zu verifizieren): Es wird angenommen, dass die Member-Tabelle
// per Voll-GET gelesen und clientseitig nach dem Gruppen-Index (Spalte
// RealServGroupIndex) gefiltert werden kann. Falls das Geraet einen gefilterten
// Pfad /config/<table>/<gruppe> erwartet, ist groupReadMembers entsprechend
// anzupassen.

const (
	groupTable       = "slbNewCfgEnhGroupTable"
	groupMemberTable = "slbNewCfgEnhGroupRealServerTable"
)

var groupMetric = map[string]int{
	"roundrobin": 1, "leastconnections": 2, "minmisses": 3, "hash": 4,
	"response": 5, "bandwidth": 6, "phash": 7, "svcleast": 8, "hrw": 9,
}

var groupHealthLayer = map[string]int{
	"icmp": 1, "tcp": 2, "http": 3, "dns": 4, "smtp": 5, "link": 28, "ldap": 31,
}

func resource_alteon_server_group() *schema.Resource {
	return &schema.Resource{
		CreateContext: resource_alteon_server_group_create,
		ReadContext:   resource_alteon_server_group_read,
		UpdateContext: resource_alteon_server_group_update,
		DeleteContext: resource_alteon_server_group_delete,
		Importer:      &schema.ResourceImporter{StateContext: schema.ImportStatePassthroughContext},
		Schema: map[string]*schema.Schema{
			"index": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Group index (table key).",
			},
			"servers": {
				Type:        schema.TypeSet,
				Optional:    true,
				Elem:        &schema.Schema{Type: schema.TypeString},
				Description: "Complete set of real server indices that are members of this group (declarative).",
			},
			"name": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"metric": {
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Description: "Load balancing metric: roundrobin|leastconnections|minmisses|hash|response|bandwidth|phash|svcleast|hrw",
			},
			"health_check_layer": {
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Description: "Health check layer: icmp|tcp|http|dns|smtp|link|ldap (or numeric for others).",
			},
			"health_id": {
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Description: "Health check ID/name (HealthID).",
			},
			"backup_server": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"backup_group": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"real_threshold": {
				Type:     schema.TypeInt,
				Optional: true,
				Computed: true,
			},
			"slowstart": {
				Type:     schema.TypeInt,
				Optional: true,
				Computed: true,
			},
			"ip_ver": {
				Type:        schema.TypeInt,
				Optional:    true,
				Computed:    true,
				Description: "IP version (1=IPv4, 2=IPv6).",
			},
			"last_updated": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
		},
	}
}

// groupCreatePayload baut den Payload fuer Create (alle konfigurierten Felder).
func groupCreatePayload(d *schema.ResourceData) map[string]interface{} {
	p := map[string]interface{}{}
	if v, ok := d.GetOk("name"); ok {
		p["Name"] = v.(string)
	}
	if v, ok := d.GetOk("metric"); ok {
		if n, found := groupMetric[v.(string)]; found {
			p["Metric"] = n
		}
	}
	if v, ok := d.GetOk("health_check_layer"); ok {
		if n, found := groupHealthLayer[v.(string)]; found {
			p["HealthCheckLayer"] = n
		}
	}
	if v, ok := d.GetOk("health_id"); ok {
		p["HealthID"] = v.(string)
	}
	if v, ok := d.GetOk("backup_server"); ok {
		p["BackupServer"] = v.(string)
	}
	if v, ok := d.GetOk("backup_group"); ok {
		p["BackupGroup"] = v.(string)
	}
	if v, ok := d.GetOk("real_threshold"); ok {
		p["RealThreshold"] = v.(int)
	}
	if v, ok := d.GetOk("slowstart"); ok {
		p["Slowstart"] = v.(int)
	}
	if v, ok := d.GetOk("ip_ver"); ok {
		p["IpVer"] = v.(int)
	}
	return p
}

// groupUpdatePayload sendet NUR geaenderte Felder. Grund: Alteon nimmt Partial-PUTs
// an (bestaetigt per curl), aber bei interdependenten Feldern (z.B. HealthCheckLayer
// + HealthID) fuehrt das Mitsenden des alten Wertes dazu, dass Alteon den neuen
// Wert stillschweigend revidiert. Daher: nur senden, was sich tatsaechlich aendert.
// Fuer entfernte Felder (-> null/leer) wird der Reset-Wert explizit gesendet.
func groupUpdatePayload(d *schema.ResourceData) map[string]interface{} {
	p := map[string]interface{}{}
	if d.HasChange("name") {
		p["Name"] = d.Get("name").(string) // "" = Reset (CLI: name none)
	}
	if d.HasChange("metric") {
		if v := d.Get("metric").(string); v != "" {
			if n, found := groupMetric[v]; found {
				p["Metric"] = n
			}
		}
	}
	if d.HasChange("health_check_layer") {
		if v := d.Get("health_check_layer").(string); v != "" {
			if n, found := groupHealthLayer[v]; found {
				p["HealthCheckLayer"] = n
			}
		}
	}
	if d.HasChange("health_id") {
		p["HealthID"] = d.Get("health_id").(string)
	}
	if d.HasChange("backup_server") {
		p["BackupServer"] = d.Get("backup_server").(string)
	}
	if d.HasChange("backup_group") {
		p["BackupGroup"] = d.Get("backup_group").(string)
	}
	if d.HasChange("real_threshold") {
		p["RealThreshold"] = d.Get("real_threshold").(int)
	}
	if d.HasChange("slowstart") {
		p["Slowstart"] = d.Get("slowstart").(int)
	}
	if d.HasChange("ip_ver") {
		p["IpVer"] = d.Get("ip_ver").(int)
	}
	return p
}

// groupReadMembers liest die Ist-Mitglieder einer Gruppe aus der Member-Tabelle.
func groupReadMembers(client *radwaregosdk.New_Client, groupIndex string) ([]string, diag.Diagnostics) {
	rows, found, diags := readTable(client, groupMemberTable)
	if diags.HasError() || !found {
		return nil, diags
	}
	var members []string
	for _, row := range rows {
		// Nur Zeilen dieser Gruppe.
		if asString(row["RealServGroupIndex"]) == groupIndex {
			members = append(members, asString(row["ServIndex"]))
		}
	}
	return members, diags
}

// groupApplyMemberDelta bringt die Mitgliedschaft auf den Soll-Stand.
func groupApplyMemberDelta(client *radwaregosdk.New_Client, groupIndex string, want []string) diag.Diagnostics {
	have, diags := groupReadMembers(client, groupIndex)
	if diags.HasError() {
		return diags
	}
	wantSet := map[string]bool{}
	haveSet := map[string]bool{}
	for _, s := range want {
		wantSet[s] = true
	}
	for _, s := range have {
		haveSet[s] = true
	}
	api := configPath(groupTable, groupIndex)
	for s := range wantSet {
		if !haveSet[s] {
			if dd := writeItem(client, api, map[string]interface{}{"AddServer": s}, false); dd.HasError() {
				return dd
			}
		}
	}
	for s := range haveSet {
		if !wantSet[s] {
			if dd := writeItem(client, api, map[string]interface{}{"RemoveServer": s}, false); dd.HasError() {
				return dd
			}
		}
	}
	return diags
}

func serverGroupWantServers(d *schema.ResourceData) []string {
	raw := d.Get("servers").(*schema.Set).List()
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		out = append(out, v.(string))
	}
	return out
}

func resource_alteon_server_group_create(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*radwaregosdk.New_Client)
	idx := d.Get("index").(string)
	api := configPath(groupTable, idx)

	if diags := writeItem(client, api, groupCreatePayload(d), true); diags.HasError() {
		return diags
	}
	d.SetId(idx)

	if diags := groupApplyMemberDelta(client, idx, serverGroupWantServers(d)); diags.HasError() {
		return diags
	}
	return resource_alteon_server_group_read(ctx, d, m)
}

func resource_alteon_server_group_read(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*radwaregosdk.New_Client)
	idx := d.Id()
	api := configPath(groupTable, idx)

	item, found, diags := readItem(client, api, groupTable)
	if diags.HasError() {
		return diags
	}
	if !found {
		d.SetId("")
		return diags
	}
	d.Set("index", idx)
	if v, ok := item["Name"]; ok {
		d.Set("name", asString(v))
	}
	if v, ok := item["Metric"]; ok {
		for name, n := range groupMetric {
			if n == asInt(v) {
				d.Set("metric", name)
			}
		}
	}
	if v, ok := item["HealthCheckLayer"]; ok {
		matched := false
		for name, n := range groupHealthLayer {
			if n == asInt(v) {
				d.Set("health_check_layer", name)
				matched = true
			}
		}
		_ = matched
	}
	if v, ok := item["HealthID"]; ok {
		d.Set("health_id", asString(v))
	}
	if v, ok := item["BackupServer"]; ok {
		d.Set("backup_server", asString(v))
	}
	if v, ok := item["BackupGroup"]; ok {
		d.Set("backup_group", asString(v))
	}
	if v, ok := item["RealThreshold"]; ok {
		d.Set("real_threshold", asInt(v))
	}
	if v, ok := item["Slowstart"]; ok {
		d.Set("slowstart", asInt(v))
	}
	if v, ok := item["IpVer"]; ok {
		d.Set("ip_ver", asInt(v))
	}

	// Mitglieder aus der Member-Tabelle.
	members, mdiags := groupReadMembers(client, idx)
	if mdiags.HasError() {
		return mdiags
	}
	d.Set("servers", members)
	return diags
}

func resource_alteon_server_group_update(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*radwaregosdk.New_Client)
	idx := d.Id()
	api := configPath(groupTable, idx)

	if diags := writeItem(client, api, groupUpdatePayload(d), false); diags.HasError() {
		return diags
	}
	if d.HasChange("servers") {
		if diags := groupApplyMemberDelta(client, idx, serverGroupWantServers(d)); diags.HasError() {
			return diags
		}
	}
	d.Set("last_updated", time.Now().Format(time.RFC3339))
	return resource_alteon_server_group_read(ctx, d, m)
}

func resource_alteon_server_group_delete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	client := m.(*radwaregosdk.New_Client)
	api := configPath(groupTable, d.Id())
	if diags := deleteItem(client, api); diags.HasError() {
		return diags
	}
	d.SetId("")
	return nil
}
