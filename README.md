# terraform-provider-alteon — Erweiterung

Terraform-Provider für Radware Alteon Loadbalancer (REST-API, FW 34.0.9+).
Erweitert den von Radware geclonten Provider um vollständiges CRUD über die
REST-Tabellen mit echtem Read (Drift-Detection), flachem Schema und Import-Support
für alle Ressourcen.

**46 Ressourcen** (30 managed + 9 Data Sources + 7 Operations).

---

## Übersetzen / Bauen

### Voraussetzungen
- Go >= 1.21 (getestet mit 1.22)
- Zum Bauen ohne Internet: `vendor/`-Verzeichnis muss vorhanden sein

### Mit Makefile

```bash
make vendor   # einmalig MIT Internet
make build    # statische linux/amd64-Binary
make dist     # Binary + SHA256 nach dist/
```

### Manuell

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -mod=vendor -trimpath -ldflags="-s -w" -o terraform-provider-alteon .
```

### Transfer auf lbmgmt

Binary in den Plugin-Pfad:
```
~/.terraform.d/plugins/animate.de/slb/alteon/0.1.0/linux_amd64/terraform-provider-alteon_v0.1.0
```

Oder per dev-override in `~/.terraformrc`:
```hcl
provider_installation {
  dev_overrides {
    "radware/alteon" = "/data/loadbalancer/terraform/dev"
  }
  direct {}
}
```

---

## Provider-Konfiguration

```hcl
terraform {
  required_providers {
    alteon = {
      source  = "animate.de/slb/alteon"
      version = "0.1.0"
    }
  }
}

provider "alteon" {
  ip       = "10.32.69.17"
  username = var.alteon_user
  password = var.alteon_pass
}
```

Zugangsdaten per Umgebungsvariable: `ALTEON_IP`, `ALTEON_USERNAME`, `ALTEON_PASSWORD`.

> Nach schreibenden Operationen braucht Alteon `alteon_apply` + `alteon_save`,
> damit die Konfiguration aktiv und persistent wird.

---

## Ressourcenübersicht

### SLB-Kern (flaches Schema, Import + Read)

| Ressource | Key | Tabelle(n) |
|-----------|-----|------------|
| `alteon_real_server` | `index` (string) | EnhRealServerTable + SecondPart + ThirdPart |
| `alteon_server_group` | `index` (string) | EnhGroupTable + Member-Tabelle |
| `alteon_virtual_server` | `index` (string) | EnhVirtServerTable |
| `alteon_virtual_service` | `servindex` + `index` (string/int) | EnhVirtServicesTable (7 Teile) |

### SSL / TLS

| Ressource | Key | Beschreibung |
|-----------|-----|--------------|
| `alteon_ssl_policy` | `nameidindex` (string) | SSL-Policy (Frontend + Backend) |
| `alteon_ssl_cert` | `cert_id` + `cert_type` (string/int) | SSL-Zertifikat |
| `alteon_ssl_cert_group` | `group_id` (string) | Zertifikatsgruppe mit deklarativer Cert-Liste |
| `alteon_http2_policy` | `nameidindex` (string) | HTTP/2-Policy |

### Health Checks (12 Typen + Alias)

| Ressource | Key | Typspezifische Felder |
|-----------|-----|-----------------------|
| `alteon_advhc_tcp` | `id_name` | `conn_term` |
| `alteon_advhc_icmp` | `id_name` | (nur Grundgerüst) |
| `alteon_advhc_udp` | `id_name` | `padding` |
| `alteon_advhc_dns` | `id_name` | `domain_name`, `transport` |
| `alteon_advhc_http` | `id_name` | `https`, `method`, `path`, `response_code`, ... |
| `alteon_advhc_smtp` | `id_name` | `username` |
| `alteon_advhc_sslhello` | `id_name` | `ssl_version`, `cipher_name` |
| `alteon_advhc_ldap` | `id_name` | `ldaps`, `base_object` |
| `alteon_advhc_radius` | `id_name` | `secret`, `down_type` |
| `alteon_advhc_arp` | `id_name` | (nur Grundgerüst) |
| `alteon_advhc_link` | `id_name` | (nur Grundgerüst) |
| `alteon_advhc_script` | `id_name` | `string_val` |
| `alteon_https_health_check` | `id_name` | Alias für `alteon_advhc_http` |

Gemeinsame Felder aller HC-Typen: `name`, `dport`, `ip_version`, `host_name`,
`transparent`, `interval`, `retries`, `restore_retries`, `timeout`, `overflow`,
`down_interval`, `invert`, `snat`.

### VRRP

| Ressource | Key | Beschreibung |
|-----------|-----|--------------|
| `alteon_vrrp` | `index` (int) | Virtual Router |
| `alteon_vrrp_group` | `index` (int) | VR Group mit deklarativer `virtual_routers`-Liste |

### Filter

| Ressource | Key | Beschreibung |
|-----------|-----|--------------|
| `alteon_filter` | `index` (int) | Filter (alle 120+ Felder: L3/L4, SSL, ADV, ACL) |
| `alteon_filter_port` | `port` (int) | Filter-Zuordnung zu Port (deklarative `filters`-Liste) |
| `alteon_filter_redirect_mapping` | `filter` + `from_str` | HTTP-Redirect-Mapping |

### Proxy IP

| Ressource | Key | Beschreibung |
|-----------|-----|--------------|
| `alteon_pip` | `address` (string) | PIP mit deklarativen `ports`/`vlans`-Listen |
| `alteon_peer_pip` | `index` (int) | Peer-PIP |

### Traffic Match

| Ressource | Key | Beschreibung |
|-----------|-----|--------------|
| `alteon_data_class` | `id_name` (string) | Data Class mit nested `entry`-Blöcken |
| `alteon_content_class` | `id_name` (string) | Content Class mit Match-Blöcken |

### AppShape

| Ressource | Key | Beschreibung |
|-----------|-----|--------------|
| `alteon_appshape_script` | `index` (string) | Script-Metadaten (Name/State) |
| `alteon_appshape_binding` | `target` + Keys | Bindung an Service oder Filter |

### Operations (aus Basis-Provider)

| Ressource | Beschreibung |
|-----------|--------------|
| `alteon_apply` | Pending Config aktivieren |
| `alteon_save` | Running Config persistent speichern |
| `alteon_revert` | Pending Config verwerfen |
| `alteon_cli_command` | Einzelnes CLI-Kommando absetzen |

---

## HCL-Beispiele

### Real Server + Server Group + Virtual Server/Service

```hcl
resource "alteon_real_server" "web1" {
  index   = "15"
  ip_addr = "10.12.188.31"
  name    = "sfivgwawpapp01_443"
  state   = 2
  weight  = 1
  ip_ver  = 1
}

resource "alteon_server_group" "grp12" {
  index              = "12"
  name               = "intranet-wahlen"
  servers            = ["15", "16"]
  metric             = "leastconnections"
  health_check_layer = "tcp"
  health_id          = "tcp"
}

resource "alteon_virtual_server" "vs1" {
  index                      = "1"
  virt_server_ip_address     = "10.12.188.38"
  virt_server_state          = 2
  virt_server_ip_ver         = 1
}

resource "alteon_virtual_service" "svc1" {
  servindex  = "1"
  index      = 1          # Service-Index, NICHT der Port
  virt_port  = 443
  real_port  = 443
  real_group = "100"
  d_bind     = 2
  status     = 1
}
```

### SSL Policy

```hcl
resource "alteon_ssl_policy" "hessenforst" {
  nameidindex         = "hessenforst"
  cipher_name         = 1   # 1=user-defined
  cipher_userdef      = "ECDHE-RSA-AES256-GCM-SHA384:ECDHE-RSA-AES128-GCM-SHA256:..."
  intermca_chain_name = "4"
  intermca_chain_type = 2   # 2=group
  authpol             = "10"
  secreneg            = 0
  admin_status        = 1   # enabled
}
```

### SSL Cert Group

```hcl
resource "alteon_ssl_cert_group" "grp4" {
  group_id     = "4"
  name         = "hessenforst-certs"
  type         = 5            # 5=intermediate
  certificates = [1, 2, 3]   # deklarative Cert-Index-Liste
}
```

### Health Check (HTTP/HTTPS)

```hcl
resource "alteon_advhc_http" "web" {
  id_name         = "hc-web"
  dport           = 443
  https           = true
  method          = "get"
  path            = "/health"
  response_code   = "200"
  receive_string  = "OK"
  interval        = 5
  retries         = 3
  restore_retries = 2
}
```

### VRRP Group mit Virtual Routern

```hcl
resource "alteon_vrrp" "vr1" {
  index    = 1
  vrid     = 10
  addr     = "10.12.188.1"
  if_index = 1
  priority = 100
  state    = true
}

resource "alteon_vrrp" "vr2" {
  index    = 2
  vrid     = 20
  addr     = "10.12.188.2"
  if_index = 1
  priority = 100
  state    = true
}

resource "alteon_vrrp_group" "grp1" {
  index           = 1
  vrid            = 1
  priority        = 100
  state           = true
  virtual_routers = [1, 2]
}
```

### Filter (mit SSL-Inspection)

```hcl
resource "alteon_filter" "ssl_inspect" {
  index              = 100
  name               = "ssl-inspection"
  action             = 3    # redirect
  state              = 1    # enabled
  protocol           = "6"  # TCP
  range_low_dst_port = "443"
  range_high_dst_port = "443"
  ssl_inspection_ena = 1
  ssl_policy         = "hessenforst"
  srv_cert_group     = 1    # 1=group
  srv_cert           = "4"
  ssl_l7_action      = 3    # inspect
  log                = 1
}

resource "alteon_filter_port" "port1" {
  port    = 1
  state   = true
  filters = [100, 101]
}
```

### Proxy IP

```hcl
resource "alteon_pip" "pip1" {
  address = "10.12.188.53"
  ports   = [1, 2]
  vlans   = [100]
}
```

### Data Class + Content Class

```hcl
resource "alteon_data_class" "blocklist" {
  id_name   = "bad-ips"
  data_type = "ip"
  entry { id = 1  key = "203.0.113.5" }
  entry { id = 2  key = "198.51.100.9" }
}

resource "alteon_content_class" "api" {
  id_name = "cc-api"
  path {
    id         = "1"
    file_path  = "/api/"
    match_type = "prefx"
  }
}
```

### AppShape

```hcl
resource "alteon_appshape_script" "redirect" {
  index = "as-redirect"
  state = true
}

resource "alteon_appshape_binding" "bind" {
  target          = "service"
  virtual_server  = "vs1"
  virtual_service = 1
  priority        = 100
  script_index    = alteon_appshape_script.redirect.index
}
```

---

## Import

### Einfache Keys

```bash
terraform import alteon_real_server.web1 15
terraform import alteon_server_group.grp12 12
terraform import alteon_virtual_server.vs1 1
terraform import alteon_ssl_policy.hf hessenforst
terraform import alteon_http2_policy.h2 http2pol_drupal
terraform import alteon_advhc_http.web hc-web
terraform import alteon_ssl_cert_group.grp4 4
terraform import alteon_filter.f100 100
terraform import alteon_vrrp.vr1 1
terraform import alteon_vrrp_group.grp1 1
terraform import alteon_pip.pip1 10.12.188.53
```

### Zweiteilige Keys

```bash
# Virtual Service: servindex/index (Server-Index / Service-Index, NICHT der Port!)
terraform import alteon_virtual_service.svc1 1/1

# SSL Cert: cert_id/cert_type
terraform import alteon_ssl_cert.mycert mycert/3
```

### Import-Blöcke (Terraform >= 1.5)

```hcl
import {
  to = alteon_server_group.grp12
  id = "12"
}

import {
  to = alteon_virtual_service.svc1
  id = "1/1"
}
```

Jeder `import`-Block braucht einen passenden `resource`-Block. Alternativ:
```bash
terraform plan -generate-config-out=generated.tf
```

---

## Deklarative Mengen

Mehrere Ressourcen nutzen deklarative Sets statt Add/Rem-Kommandos:

| Ressource | Feld | Beschreibung |
|-----------|------|--------------|
| `alteon_server_group` | `servers` | Real-Server-Mitglieder |
| `alteon_vrrp_group` | `virtual_routers` | VR-Mitglieder |
| `alteon_ssl_cert_group` | `certificates` | Zertifikate in der Gruppe |
| `alteon_pip` | `ports`, `vlans` | Port-/VLAN-Zuordnung |
| `alteon_filter_port` | `filters` | Filter-Regeln auf dem Port |

Hinzufügen = Element in die Liste aufnehmen. Entfernen = rausnehmen.
Terraform berechnet das Delta und feuert die passenden Add/Rem-Befehle.

---

## Enum-Werte

Die meisten Felder nutzen numerische Enum-Werte (wie die REST-API).
Ausnahmen: `server_group.metric` und `server_group.health_check_layer`
akzeptieren Strings (`"leastconnections"`, `"tcp"`, `"icmp"` usw.).

Häufige Werte:

| Enum | Werte |
|------|-------|
| State/enabled | 1=enabled, 2=disabled |
| Filter Action | 1=allow, 2=deny, 3=redirect, 4=nat, 5=goto, 6=outbound-llb, 7=monitor |
| SSL Cert Type | 3=server, 4=trusted, 5=intermediate |
| SSL CertGroup | 1=group, 2=cert, 3=none |
| IP Version | 1=IPv4, 2=IPv6 |
| SG Metric | roundrobin, leastconnections, minmisses, hash, response, bandwidth, phash, svcleast, hrw |
| SG HealthLayer | icmp, tcp, http, dns, smtp, link, ldap |

Im Zweifel: erst importieren, dann `terraform show` — die Ist-Werte als Zahlen
können direkt ins HCL übernommen werden.

---

## Empfohlenes Vorgehen

1. **Am Testpaar anfangen** — nicht produktiv
2. **Importieren** (`terraform import`) + `terraform plan` → Plan muss leer sein
3. Erst dann Änderungen via `terraform apply`
4. `alteon_apply` + `alteon_save` als letzten Schritt einplanen
5. Schrittweise ausrollen: erst eine VIP-Kette, dann weitere

---

## Bekannte Einschränkungen

- **Bitmap-Felder** (VirtServerRule, UrlBmap, PortsIngress/Egress) sind nicht als
  deklarative Sets abgebildet. Können bei Bedarf ergänzt werden.
- **AppShape-Skript-Inhalt** wird nicht verwaltet (kein REST-Endpunkt in der Doku).
- **PIP-/Filter-Bitmap-Nummerierung** (MSB-first, 1-basiert) ist die übliche
  Alteon-Konvention, sollte gegen bekannte Zuordnungen verifiziert werden.
- **Content-Class-Aktivflags**: Es wird angenommen, dass sie beim Subtabellen-Write
  automatisch gesetzt werden.
- Alteon ignoriert **Partial-PUTs** in manchen Kontexten und hat
  **interdependente Felder** (z.B. HealthCheckLayer/HealthID). Updates senden daher
  nur geänderte Felder (`HasChange`), um Widersprüche zu vermeiden.
