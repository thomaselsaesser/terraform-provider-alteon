# terraform-provider-alteon — Erweiterung Phase 0–6

Vollständiger Stand. Kompiliert sauber (`go build ./...`, `go vet ./alteon/` → exit 0)
und baut als statische `linux/amd64`-Binary (`CGO_ENABLED=0`, ELF, statically linked,
stripped) gegen `github.com/Radware/radware_go_sdk` und terraform-plugin-sdk/v2 v2.34.0.

Alle REST-Tabellennamen, Felder und Enums wurden gegen die RDWRAlteonRestDoc
(FW 34.0.9+) verifiziert. Wo etwas nicht aus der Doku belegbar war, ist es als
offener, am Gerät zu klärender Punkt markiert — nichts geraten.

## Neue Dateien (nach alteon/ kopieren)

| Datei | Inhalt |
|-------|--------|
| `helpers.go` | Phase 0: CRUD-Helfer, Enum/Bool-Mapping, Bitmap-Dekoder, Set-Delta |
| `resource_alteon_vrrp.go` | Phase 1: Virtual Router |
| `resource_alteon_vrrp_group.go` | Phase 1: Switch-Based VR Group |
| `advhc_common.go` | Phase 2: generischer AdvHC-Builder |
| `advhc_types.go` | Phase 2: alle HC-Typen (tcp, icmp, udp, dns, http, smtp, sslhello, ldap, radius, arp, link, script) |
| `resource_alteon_pip.go` | Phase 3: PIP (Bitmap-Delta) + Peer-PIP |
| `resource_alteon_data_class.go` | Phase 4: Data Class + manual entries |
| `resource_alteon_content_class.go` | Phase 4: Content Class + 8 Match-Subtabellen |
| `resource_alteon_filter.go` | Phase 5: Filter + Filter-Port + Redirect-Mapping |
| `resource_alteon_appshape.go` | Phase 6: AppShape-Script + Binding |
| `provider.go` | alle 24 neuen Ressourcen registriert (geändert) |

## Registrierte Ressourcen (24 neu)

```
alteon_vrrp, alteon_vrrp_group
alteon_advhc_tcp, _icmp, _udp, _dns, _http, _smtp, _sslhello, _ldap, _radius, _arp, _link, _script
alteon_pip, alteon_peer_pip
alteon_data_class, alteon_content_class
alteon_filter, alteon_filter_port, alteon_filter_redirect_mapping
alteon_appshape_script, alteon_appshape_binding
```

Durchgängiges Designprinzip (mit dir abgestimmt): flaches Schema, echter Read
(Drift-Detection), DELETE-Methode zum Löschen, 1/2-Enums als bool, explizite Indizes.
Die bestehende `alteon_https_health_check` bleibt unangetastet; `alteon_advhc_http`
deckt HTTP **und** HTTPS ab und kann sie perspektivisch per `state mv` ablösen.

## Am Gerät zu verifizieren (in den Dateien als HINWEIS markiert)

1. **PIP-Bitmap-Nummerierung** (`resource_alteon_pip.go`, `decodeHexBitmap` in
   `helpers.go`): Format Hex-Byte-Bitmap ist sicher; die MSB-first/1-basierte
   Bit→Port/VLAN-Nummer ist die übliche Konvention, sollte aber gegen eine PIP mit
   bekannter Port-/VLAN-Zuordnung geprüft werden. Gleiches gilt für `FiltBmap` in
   `alteon_filter_port`.
2. **Content-Class-Aktiv-Flags**: Ob die Haupttabellen-Flags (HostName=1/2 …)
   automatisch beim Schreiben einer Subtabelle gesetzt werden, oder explizit gesetzt
   werden müssen. Aktuell verlassen wir uns auf automatisches Setzen.
3. **AppShape-Skript-INHALT**: In der vorliegenden Doku ist KEIN REST-Endpunkt zum
   Setzen des Skript-Codes belegt (nur Metadaten Name/State/Default). `alteon_appshape_script`
   verwaltet daher nur die Metadaten. Sobald der Upload-Mechanismus am Gerät geklärt
   ist, lässt sich ein `content`-Feld ergänzen. Bewusst nicht geraten.
4. **Entry-Drift**: Bei Data Class und Content Class wird Drift auf Kopf-Ebene
   erkannt; die nested entries werden im State belassen wie konfiguriert. Ein
   vollständiger Subtabellen-Read (gefiltert nach Parent-ID) kann bei Bedarf ergänzt
   werden, falls ihr Out-of-band-Änderungen an Einträgen erkennen wollt.

## Bauen & Transfer

Siehe `BUILD_UND_TRANSFER.md` (3-Rechner-Workflow). Kurz:
```bash
make vendor    # einmalig mit Internet
make build     # statische linux/amd64-Binary
make dist      # Binary + sha256 fuer lbmgmt
```

## Mitgelieferte Test-Binary

`terraform-provider-alteon.linux-amd64` (+ .sha256) ist die in dieser Sitzung
gebaute statische Binary. Du kannst sie direkt auf lbmgmt testen — ich empfehle aber,
nach dem `make vendor` auf deinem Build-Rechner neu zu bauen, damit Binary und
gepushter Sourcecode garantiert aus demselben Stand stammen.

## HCL-Beispiele (Auszug)

```hcl
resource "alteon_advhc_http" "web" {
  id_name        = "hc-web"
  dport          = 443
  https          = true
  method         = "get"
  path           = "/health"
  response_code  = "200"
  receive_string = "OK"
  interval       = 5
  retries        = 3
}

resource "alteon_pip" "pip1" {
  address = "10.12.188.53"
  ports   = [1, 2]
  vlans   = [100]
}

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

resource "alteon_filter" "allow_web" {
  index    = 100
  action   = "allow"
  protocol = 6
  dst_port_low  = 443
  dst_port_high = 443
  state    = true
}

resource "alteon_appshape_script" "redirect" {
  index = "as-redirect"
  state = true
}
resource "alteon_appshape_binding" "bind_redirect" {
  target          = "service"
  virtual_server  = "vs1"
  virtual_service = 1
  priority        = 100
  script_index    = alteon_appshape_script.redirect.index
}
```
