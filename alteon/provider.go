package alteon

import (
	"context"
	"strconv"

	radwaregosdk "github.com/Radware/radware_go_sdk"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// Provider -
func Provider() *schema.Provider {
	return &schema.Provider{
		Schema: map[string]*schema.Schema{
			"username": &schema.Schema{
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("ALTEON_USERNAME", nil),
				Description: "Alteon Username.",
			},
			"password": &schema.Schema{
				Type:        schema.TypeString,
				Optional:    true,
				Sensitive:   true,
				DefaultFunc: schema.EnvDefaultFunc("ALTEON_PASSWORD", nil),
				Description: "Alteon Password.",
			},
			"ip": &schema.Schema{
				Type:        schema.TypeString,
				Optional:    true,
				Sensitive:   true,
				DefaultFunc: schema.EnvDefaultFunc("ALTEON_IP", nil),
				Description: "Management IP of Alteon.",
			},
		},
		ResourcesMap: map[string]*schema.Resource{
			"alteon_real_server":        resource_alteon_real_server(),
			"alteon_real_server_layer7": resource_alteon_real_server_layer7(),
			"alteon_url_lb_path":        resource_alteon_url_lb_path(),
			"alteon_server_group":       resource_alteon_server_group(),
			"alteon_cli_command":        resource_alteon_cli_command(),
			"alteon_apply":              resource_alteon_apply(),
			"alteon_save":               resource_alteon_save(),
			"alteon_revert":             resource_alteon_revert(),
			"alteon_revert_apply":       resource_alteon_revert_apply(),
			"alteon_virtual_server":     resource_alteon_virtual_server(),
			"alteon_virtual_service":    resource_alteon_virtual_service(),
			"alteon_ssl_policy":         resource_alteon_ssl_policy(),
			"alteon_ssl_cert":           resource_alteon_ssl_cert(),
			"alteon_ssl_cert_group":     resource_alteon_ssl_cert_group(),
			"alteon_http2_policy":       resource_alteon_http2_policy(),
			"alteon_https_health_check": resource_alteon_https_health_check(),
			"alteon_vrrp":               resource_alteon_vrrp(),
			"alteon_vrrp_group":         resource_alteon_vrrp_group(),
			// Phase 2: Advanced Health Checks
			"alteon_advhc_tcp":      resource_alteon_advhc_tcp(),
			"alteon_advhc_icmp":     resource_alteon_advhc_icmp(),
			"alteon_advhc_udp":      resource_alteon_advhc_udp(),
			"alteon_advhc_dns":      resource_alteon_advhc_dns(),
			"alteon_advhc_http":     resource_alteon_advhc_http(),
			"alteon_advhc_smtp":     resource_alteon_advhc_smtp(),
			"alteon_advhc_sslhello": resource_alteon_advhc_sslhello(),
			"alteon_advhc_ldap":     resource_alteon_advhc_ldap(),
			"alteon_advhc_radius":   resource_alteon_advhc_radius(),
			"alteon_advhc_arp":      resource_alteon_advhc_arp(),
			"alteon_advhc_link":     resource_alteon_advhc_link(),
			"alteon_advhc_script":   resource_alteon_advhc_script(),
			// Phase 3: Proxy IP
			"alteon_pip":      resource_alteon_pip(),
			"alteon_peer_pip": resource_alteon_peer_pip(),
			// Phase 4: Traffic Match Criteria
			"alteon_data_class":    resource_alteon_data_class(),
			"alteon_content_class": resource_alteon_content_class(),
			// Phase 5: Filters
			"alteon_filter":                  resource_alteon_filter(),
			"alteon_filter_port":             resource_alteon_filter_port(),
			"alteon_filter_redirect_mapping": resource_alteon_filter_redirect_mapping(),
			// Phase 6: AppShape
			"alteon_appshape_script":  resource_alteon_appshape_script(),
			"alteon_appshape_binding": resource_alteon_appshape_binding(),
		},
		DataSourcesMap: map[string]*schema.Resource{
			//"real_server": dataSourceRealServer(),
			"alteon_real_server_data":        data_source_alteon_real_server(),
			"alteon_server_group_data":       data_source_alteon_server_group(),
			"alteon_virtual_server_data":     data_source_alteon_virtual_server(),
			"alteon_https_health_check_data": data_source_alteon_https_health_check(),
			"alteon_ssl_policy_data":         data_source_alteon_ssl_policy(),
			"alteon_http2_policy_data":       data_source_alteon_http2_policy(),
			"alteon_virtual_service_data":    data_source_alteon_virtual_service(),
			"alteon_apply_status_data":       data_source_alteon_apply_status(),
			"alteon_apply_table_data":        data_source_alteon_apply_table(),
		},
		ConfigureContextFunc: providerConfigure,
	}
}

func providerConfigure(ctx context.Context, d *schema.ResourceData) (interface{}, diag.Diagnostics) {
	username := d.Get("username").(string)
	password := d.Get("password").(string)
	host := d.Get("ip").(string)

	// Warning or errors can be collected in a slice type
	var diags diag.Diagnostics

	/*
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Warning,
			Summary:  "Warning Message Summary",
			Detail:   "This is the detailed warning message from providerConfigure",
		})*/

	if (username != "") && (password != "") {
		client, status, message, err := radwaregosdk.Login("ALTEON", host, username, password)
		//client.HostIP = host
		detail := strconv.Itoa(status) + message

		if err != nil {
			diags = append(diags, diag.Diagnostic{
				Severity: diag.Error,
				Summary:  "Unable to connect to Alteon." + detail + err.Error(),
				Detail:   detail + err.Error(),
			})
			return nil, diags
		}

		if client == nil {
			diags = append(diags, diag.Diagnostic{
				Severity: diag.Error,
				Summary:  "Authentication Failed" + detail,
				Detail:   detail,
			})
			return nil, diags
		}

		return client, diags
	}

	return 1, diags
}
