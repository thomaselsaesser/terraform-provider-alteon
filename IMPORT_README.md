# Import-Nachrüstung für Bestandsressourcen

Rüstet `terraform import` + echten Read für die bestehenden SLB-Ressourcen nach,
**ohne deren Schema zu ändern**. Read-Mapping gegen echte Geräte-GETs verifiziert.

## Neue Dateien (nach alteon/)
- `legacy_import.go` — gemeinsamer Read-/Import-Helfer (CamelCase→lowercase-Mapping)
- `legacy_import_specs.go` — Spec + Read/Import-Funktion je Ressource

## Geänderte Dateien (nur Resource-Definition: ReadContext + Importer)
- `resource_alteon_real_server.go`
- `resource_alteon_server_group.go`
- `resource_alteon_virtual_server.go`
- `resource_alteon_virtual_service.go`
- `resource_alteon_ssl_policy.go`
- `resource_alteon_http2_policy.go`
- `resource_alteon_https_health_check.go`

In jeder wurde nur `ReadContext` auf die neue Legacy-Read-Funktion umgebogen und
ein `Importer` ergänzt. Die alten `..._read`-Funktionen bleiben unangetastet im Code
(ungenutzt, aber harmlos). Schema und Create/Update/Delete sind unverändert.

## Import-Befehle

Einfacher Key:
```bash
terraform import alteon_real_server.web1   <real-server-index>
terraform import alteon_server_group.grp1  <group-index>
terraform import alteon_virtual_server.vs1 <virtual-server-index>
terraform import alteon_ssl_policy.p1      <nameidindex>
terraform import alteon_http2_policy.h2    <nameidindex>
terraform import alteon_https_health_check.hc <id>
```

Virtual Service — zweiteiliger Key `servindex/index` (z. B. VirtServer 96, Service 1):
```bash
terraform import alteon_virtual_service.svc1 96/1
```

## WICHTIG für den Test

1. Read-Mapping ist gegen echte Daten geprüft (CamelCase-JSON → lowercase-Schema
   per ToLower; Sonderfälle real_server `ProxyIpAddr`→`proxyipaddress` etc. sind in
   der Overrides-Map). Aber:
2. Der echte Lackmustest ist **`terraform import` gefolgt von `terraform plan`**.
   - `plan` leer → alles gut.
   - Phantom-Diffs (z. B. bei Add/Rem-Feldern wie `addserver`, die beim Read als 0
     zurückkommen) → bitte die `plan`-Ausgabe schicken. Korrektur ist dann gezielt:
     betroffene Felder in `legacy_import_specs.go` aus dem elements-Mapping filtern
     bzw. in die Overrides-Map aufnehmen. Kein Umbau nötig.
3. **virtual_service**: Vollständig gegen echte Daten verifiziert (Server 711,
   Service 1, alle sieben Part-Tabellen). 133/134 Felder treffen automatisch; der
   Sonderfall `ProxyIpNWclass` → Schema `proxyipnwsclass` (Tippfehler im Original-
   Schema) ist in der Overrides-Map abgefangen. Key-Form `96/1` bzw. `711/1`.
