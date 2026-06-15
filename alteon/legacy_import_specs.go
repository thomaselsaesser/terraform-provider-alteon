package alteon

import (
	"context"
	"strconv"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// Specs + Read/Import-Funktionen fuer die nachgeruesteten Bestandsressourcen.
// Verifiziert gegen echte Geraete-GETs (Datei "zwi", FW-Stand des Kunden).
//
// Anbindung: in der jeweiligen Ressourcen-Datei
//   - Importer: &schema.ResourceImporter{StateContext: ...}
//   - ReadContext: <hier definierte Read-Funktion>

// --- server_group: ENTFERNT --------------------------------------------------
// server_group nutzt jetzt das neue deklarative Modell mit eigenem Read + Import
// (siehe resource_alteon_server_group.go) und braucht den Legacy-Pfad nicht mehr.

// --- real_server (drei Tabellen) ---------------------------------------------
func realServerImportSpec() legacyImportSpec {
	r := resource_alteon_real_server()
	return legacyImportSpec{
		Tables: []string{
			"SlbNewCfgEnhRealServerTable",
			"SlbNewCfgEnhRealServerSecondPartTable",
			"SlbNewCfgEnhRealServerThirdPartTable",
		},
		KeyFields:    map[string]bool{"Index": true},
		ElementsAttr: "elements",
		Overrides: map[string]string{
			"ProxyIpAddr":   "proxyipaddress",
			"ProxyIpv6Addr": "proxyipv6address",
		},
		ElementFields: elementFieldsFromResource(r, "elements"),
	}
}

func legacy_real_server_read(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	key := d.Get("index").(string)
	if key == "" {
		key = d.Id()
	}
	return legacyImportRead(ctx, d, m, realServerImportSpec(), key)
}

// --- virtual_server ----------------------------------------------------------
func virtualServerImportSpec() legacyImportSpec {
	r := resource_alteon_virtual_server()
	return legacyImportSpec{
		Tables:        []string{"SlbNewCfgEnhVirtServerTable"},
		KeyFields:     map[string]bool{"VirtServerIndex": true},
		ElementsAttr:  "elements",
		Overrides:     map[string]string{},
		ElementFields: elementFieldsFromResource(r, "elements"),
	}
}

func legacy_virtual_server_read(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	key := d.Get("index").(string)
	if key == "" {
		key = d.Id()
	}
	return legacyImportRead(ctx, d, m, virtualServerImportSpec(), key)
}

// --- ssl_policy --------------------------------------------------------------
func sslPolicyImportSpec() legacyImportSpec {
	r := resource_alteon_ssl_policy()
	return legacyImportSpec{
		Tables:        []string{"SlbNewSslCfgSSLPolTable"},
		KeyFields:     map[string]bool{"NameIdIndex": true},
		ElementsAttr:  "elements",
		Overrides:     map[string]string{},
		ElementFields: elementFieldsFromResource(r, "elements"),
	}
}

func legacy_ssl_policy_read(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	key := d.Get("nameidindex").(string)
	if key == "" {
		key = d.Id()
	}
	return legacyImportRead(ctx, d, m, sslPolicyImportSpec(), key)
}

// --- http2_policy ------------------------------------------------------------
func http2PolicyImportSpec() legacyImportSpec {
	r := resource_alteon_http2_policy()
	return legacyImportSpec{
		Tables:        []string{"SlbNewAcclCfgHttp2PolTable"},
		KeyFields:     map[string]bool{"NameIdIndex": true},
		ElementsAttr:  "elements",
		Overrides:     map[string]string{},
		ElementFields: elementFieldsFromResource(r, "elements"),
	}
}

func legacy_http2_policy_read(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	key := d.Get("nameidindex").(string)
	if key == "" {
		key = d.Id()
	}
	return legacyImportRead(ctx, d, m, http2PolicyImportSpec(), key)
}

// --- https_health_check ------------------------------------------------------
func httpsHealthCheckImportSpec() legacyImportSpec {
	r := resource_alteon_https_health_check()
	return legacyImportSpec{
		Tables:        []string{"SlbNewAdvhcHttpTable"},
		KeyFields:     map[string]bool{"ID": true},
		ElementsAttr:  "elements",
		Overrides:     map[string]string{},
		ElementFields: elementFieldsFromResource(r, "elements"),
	}
}

func legacy_https_health_check_read(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	key := d.Get("index").(string)
	if key == "" {
		key = d.Id()
	}
	return legacyImportRead(ctx, d, m, httpsHealthCheckImportSpec(), key)
}

// --- virtual_service (zweiteiliger Key servindex/index, sieben Tabellen) ------
func virtualServiceImportSpec() legacyImportSpec {
	r := resource_alteon_virtual_service()
	return legacyImportSpec{
		Tables: []string{
			"SlbNewCfgEnhVirtServicesTable",
			"SlbNewCfgEnhVirtServicesSecondPartTable",
			"SlbNewCfgEnhVirtServicesThirdPartTable",
			"SlbNewCfgEnhVirtServicesFourthPartTable",
			"SlbNewCfgEnhVirtServicesFifthPartTable",
			"SlbNewCfgEnhVirtServicesSixthPartTable",
			"SlbNewCfgEnhVirtServicesSeventhPartTable",
		},
		KeyFields: map[string]bool{
			"ServIndex": true, "Index": true,
			"ServSecondPartIndex": true, "SecondPartIndex": true,
			"ServThirdPartIndex": true, "ThirdPartIndex": true,
			"ServFourthPartIndex": true, "FourthPartIndex": true,
			"ServFifthPartIndex": true, "FifthPartIndex": true,
			"ServSixthPartIndex": true, "SixthPartIndex": true,
			"ServSeventhPartIndex": true, "SeventhPartIndex": true,
		},
		ElementsAttr: "elements",
		Overrides: map[string]string{
			// Schema-Feldname enthaelt einen Tippfehler ("nwsclass" statt "nwclass"),
			// daher hier explizit gemappt. Die v6-Variante trifft per ToLower.
			"ProxyIpNWclass": "proxyipnwsclass",
		},
		ElementFields: elementFieldsFromResource(r, "elements"),
	}
}

func legacy_virtual_service_read(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	serv := d.Get("servindex").(string)
	// index ist im Schema ein Integer (HCL: index=1), daher NICHT als string casten.
	idx := strconv.Itoa(d.Get("index").(int))
	keyPath := serv + "/" + idx
	if serv == "" || d.Get("index").(int) == 0 {
		keyPath = d.Id() // bei Import: Id ist "servindex/index"
	}
	return legacyImportRead(ctx, d, m, virtualServiceImportSpec(), keyPath)
}
