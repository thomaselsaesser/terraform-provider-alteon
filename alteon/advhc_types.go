package alteon

import "github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

// Definition aller AdvHC-Typen ueber Feld-Specs. Das gemeinsame Grundgeruest kommt
// aus commonAdvhcFields(); hier werden nur die typspezifischen Felder ergaenzt.
//
// Verifiziert gegen die RDWRAlteonRestDoc (FW 34.0.9+). Felder mit 1/2-Enum sind
// als fBool modelliert; benannte Auswahlen (Method, AuthLevel, ...) als fEnum.

// withExtra haengt typspezifische Felder ans Grundgeruest.
func withExtra(extra ...advhcField) []advhcField {
	return append(commonAdvhcFields(), extra...)
}

// --- TCP ---------------------------------------------------------------------
func resource_alteon_advhc_tcp() *schema.Resource {
	return resourceFromAdvhcSpec(advhcTypeSpec{
		ResourceName: "alteon_advhc_tcp",
		Table:        "slbNewAdvhcTcpTable",
		Fields: withExtra(
			advhcField{Schema: "conn_term", API: "ConnTerm", Kind: fEnum,
				Desc: "Connection termination method.", Enum: map[string]int{"fin": 1, "rst": 2}},
			advhcField{Schema: "always", API: "Always", Kind: fBool, Desc: "Always perform the check."},
		),
	})
}

// --- ICMP --------------------------------------------------------------------
func resource_alteon_advhc_icmp() *schema.Resource {
	return resourceFromAdvhcSpec(advhcTypeSpec{
		ResourceName: "alteon_advhc_icmp",
		Table:        "slbNewAdvhcIcmpTable",
		Fields:       withExtra(),
	})
}

// --- UDP ---------------------------------------------------------------------
func resource_alteon_advhc_udp() *schema.Resource {
	return resourceFromAdvhcSpec(advhcTypeSpec{
		ResourceName: "alteon_advhc_udp",
		Table:        "slbNewAdvhcUdpTable",
		Fields: withExtra(
			advhcField{Schema: "padding", API: "Padding", Kind: fInt, Desc: "UDP padding length."},
		),
	})
}

// --- DNS ---------------------------------------------------------------------
func resource_alteon_advhc_dns() *schema.Resource {
	return resourceFromAdvhcSpec(advhcTypeSpec{
		ResourceName: "alteon_advhc_dns",
		Table:        "slbNewAdvhcDnsTable",
		Fields: withExtra(
			advhcField{Schema: "domain_name", API: "DomainName", Kind: fString, Desc: "Domain name to resolve."},
			advhcField{Schema: "transport", API: "Transport", Kind: fEnum,
				Desc: "Transport protocol.", Enum: map[string]int{"tcp": 1, "udp": 2}},
		),
	})
}

// --- HTTP (deckt HTTPS via field https mit ab) -------------------------------
func resource_alteon_advhc_http() *schema.Resource {
	return resourceFromAdvhcSpec(advhcTypeSpec{
		ResourceName: "alteon_advhc_http",
		Table:        "slbNewAdvhcHttpTable",
		Fields: withExtra(
			advhcField{Schema: "https", API: "Https", Kind: fBool, Desc: "Use HTTPS instead of HTTP."},
			advhcField{Schema: "host", API: "Host", Kind: fString, Desc: "Host header value."},
			advhcField{Schema: "path", API: "Path", Kind: fString, Desc: "Request path."},
			advhcField{Schema: "method", API: "Method", Kind: fEnum,
				Desc: "HTTP method.", Enum: map[string]int{"get": 1, "post": 2, "head": 3}},
			advhcField{Schema: "headers", API: "Headers", Kind: fString, Desc: "Additional request headers."},
			advhcField{Schema: "body", API: "Body", Kind: fString, Desc: "Request body."},
			advhcField{Schema: "auth_level", API: "AuthLevel", Kind: fEnum,
				Desc: "Authentication level.", Enum: map[string]int{"none": 1, "basic": 2, "ntlm2": 3, "ntlmssp": 4}},
			advhcField{Schema: "username", API: "UserName", Kind: fString, Desc: "Username for authentication."},
			advhcField{Schema: "password", API: "Password", Kind: fString, Desc: "Password for authentication."},
			advhcField{Schema: "response_type", API: "ResponseType", Kind: fEnum,
				Desc: "Response matching type.", Enum: map[string]int{"none": 1, "incl": 2, "excl": 4}},
			advhcField{Schema: "response_code", API: "ResponseCode", Kind: fString, Desc: "Expected response code(s)."},
			advhcField{Schema: "receive_string", API: "ReceiveString", Kind: fString, Desc: "String expected in the response."},
			advhcField{Schema: "overload_type", API: "OverloadType", Kind: fEnum,
				Desc: "Overload detection type.", Enum: map[string]int{"none": 1, "incl": 2}},
			advhcField{Schema: "overload_string", API: "OverloadString", Kind: fString, Desc: "String indicating overload."},
			advhcField{Schema: "response_code_overload", API: "ResponseCodeOverload", Kind: fString, Desc: "Response code indicating overload."},
			advhcField{Schema: "proxy", API: "Proxy", Kind: fBool, Desc: "Use proxy."},
			advhcField{Schema: "https_cipher_name", API: "HttpsCipherName", Kind: fEnum,
				Desc: "HTTPS cipher set.", Enum: map[string]int{"user_defined": 1, "low": 2, "medium": 3, "high": 4}},
			advhcField{Schema: "https_cipher_userdef", API: "HttpsCipherUserdef", Kind: fString, Desc: "User-defined cipher string."},
			advhcField{Schema: "http2", API: "Http2", Kind: fBool, Desc: "Use HTTP/2."},
			advhcField{Schema: "conn_tout", API: "ConnTout", Kind: fEnum,
				Desc: "Connection termination method.", Enum: map[string]int{"fin": 1, "rst": 2}},
		),
	})
}

// --- SMTP --------------------------------------------------------------------
func resource_alteon_advhc_smtp() *schema.Resource {
	return resourceFromAdvhcSpec(advhcTypeSpec{
		ResourceName: "alteon_advhc_smtp",
		Table:        "slbNewAdvhcSmtpTable",
		Fields: withExtra(
			advhcField{Schema: "username", API: "UserName", Kind: fString, Desc: "Username for SMTP check."},
		),
	})
}

// --- SSL Hello ---------------------------------------------------------------
func resource_alteon_advhc_sslhello() *schema.Resource {
	return resourceFromAdvhcSpec(advhcTypeSpec{
		ResourceName: "alteon_advhc_sslhello",
		Table:        "slbNewAdvhcSslHelloTable",
		Fields: withExtra(
			advhcField{Schema: "ssl_version", API: "SslVersion", Kind: fString, Desc: "SSL/TLS version."},
			advhcField{Schema: "cipher_name", API: "CipherName", Kind: fString, Desc: "Cipher set name."},
			advhcField{Schema: "cipher_userdef", API: "CipherUserdef", Kind: fString, Desc: "User-defined cipher string."},
		),
	})
}

// --- LDAP (deckt LDAPS via field ldaps mit ab) -------------------------------
func resource_alteon_advhc_ldap() *schema.Resource {
	return resourceFromAdvhcSpec(advhcTypeSpec{
		ResourceName: "alteon_advhc_ldap",
		Table:        "slbNewAdvhcLdapTable",
		Fields: withExtra(
			advhcField{Schema: "ldaps", API: "Ldaps", Kind: fBool, Desc: "Use LDAPS."},
			advhcField{Schema: "username", API: "UserName", Kind: fString, Desc: "Bind username."},
			advhcField{Schema: "password", API: "Password", Kind: fString, Desc: "Bind password."},
			advhcField{Schema: "base_object", API: "BaseObject", Kind: fString, Desc: "LDAP base object."},
			advhcField{Schema: "base_fmt", API: "BaseFmt", Kind: fString, Desc: "Base object format."},
		),
	})
}

// --- RADIUS ------------------------------------------------------------------
func resource_alteon_advhc_radius() *schema.Resource {
	return resourceFromAdvhcSpec(advhcTypeSpec{
		ResourceName: "alteon_advhc_radius",
		Table:        "slbNewAdvhcRadiusTable",
		Fields: withExtra(
			advhcField{Schema: "down_type", API: "DownType", Kind: fInt, Desc: "Condition treated as down."},
			advhcField{Schema: "username", API: "UserName", Kind: fString, Desc: "RADIUS username."},
			advhcField{Schema: "password", API: "Password", Kind: fString, Desc: "RADIUS password."},
			advhcField{Schema: "secret", API: "Secret", Kind: fString, Desc: "RADIUS shared secret."},
		),
	})
}

// --- ARP / Link (nur Grundgeruest) -------------------------------------------
func resource_alteon_advhc_arp() *schema.Resource {
	return resourceFromAdvhcSpec(advhcTypeSpec{
		ResourceName: "alteon_advhc_arp",
		Table:        "slbNewAdvhcArpTable",
		Fields:       withExtra(),
	})
}

func resource_alteon_advhc_link() *schema.Resource {
	return resourceFromAdvhcSpec(advhcTypeSpec{
		ResourceName: "alteon_advhc_link",
		Table:        "slbNewAdvhcLinkTable",
		Fields:       withExtra(),
	})
}

// --- Script ------------------------------------------------------------------
// Der Script-HC nutzt Add*/Remove*-Befehle zum Aufbau der Skriptsequenz. Wir bilden
// die Kommandos als einfache String-Felder ab; der komplexe sequenzielle Aufbau
// (AddSendCmd usw.) wird ueber das stringwertige string_val gesteuert.
func resource_alteon_advhc_script() *schema.Resource {
	return resourceFromAdvhcSpec(advhcTypeSpec{
		ResourceName: "alteon_advhc_script",
		Table:        "slbNewAdvhcScriptTable",
		Fields: withExtra(
			advhcField{Schema: "string_val", API: "StringVal", Kind: fString, Desc: "The full health-check script string."},
		),
	})
}
