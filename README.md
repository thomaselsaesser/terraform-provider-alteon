# Phase 0 + 1 — Fundament und VRRP

Stand: kompiliert sauber (`go build ./...` und `go vet ./alteon/` → exit 0) gegen
`github.com/Radware/radware_go_sdk` und terraform-plugin-sdk/v2 v2.34.0.

## Gelieferte Dateien

- `helpers.go` — Phase 0, gemeinsame CRUD-Helfer (neu)
- `resource_alteon_vrrp.go` — Phase 1, Virtual Router (neu)
- `resource_alteon_vrrp_group.go` — Phase 1, Switch-Based VR Group (neu)
- `provider.go` — zwei Zeilen in der ResourcesMap ergänzt (geändert)

Die drei neuen Dateien kommen nach `alteon/`. In `provider.go` sind nur die
beiden Registrierungszeilen neu — Diff:

```
  "alteon_https_health_check": resource_alteon_https_health_check(),
+ "alteon_vrrp":               resource_alteon_vrrp(),
+ "alteon_vrrp_group":         resource_alteon_vrrp_group(),
```

## Phase 0 — helpers.go

Bündelt das CRUD-Boilerplate, das im Altbestand in jeder Ressource dupliziert war:

- `configPath(table, key)` — baut `/config/<Table>/<Key>/`
- `writeItem(...)` — POST (Create) bzw. PUT (Update), einheitliche Antwortprüfung
  (HTTP 200 UND Body `"status":"ok"`)
- `deleteItem(...)` — echtes HTTP DELETE (von Radware empfohlen statt DeleteStatus:2)
- `readItem(...)` — GET, parst `{"<Table>":[{...}]}`; liefert `found=false` bei
  404 / leerer Antwort / `"status":"err"` → Aufrufer setzt dann `d.SetId("")`
  (Drift: Objekt am Gerät gelöscht)
- `boolToEnable` / `enableToBool` — 1/2-Enum ↔ bool
- `asInt` / `asString` — robustes Lesen der JSON-Werte (JSON-Zahlen sind float64)

## Phase 1 — VRRP

Zwei flache Ressourcen mit echtem Read (Drift-Detection), wie abgestimmt:

### Designentscheidungen (umgesetzt)
- **`index` (Indx) und `vrid` (ID) getrennt** — beide Pflichtfelder, weil sie bei
  euch historisch teils auseinanderlaufen. `index` ist `ForceNew` (Key-Änderung =
  Neuanlage).
- **Explizite Index-Vergabe** — keine Auto-Vergabe.
- **Flaches Schema** statt `elements`-Block.
- **1/2-Enums als bool** — `preempt`, `state`, `sharing`, alle `track_*`.
  `track_isl_port_include` mappt 1=include/2=exclude.
- **`version`** als `"v4"`/`"v6"` (intern 1/2).
- **Import** via `terraform import alteon_vrrp.<name> <index>`.

### alteon_vrrp (vrrpNewCfgVirtRtrTable)
Voller Feldsatz inkl. `addr`, `ipv6_addr`, `if_index`, `interval`, `priority`,
`ospf_cost` und alle neun Tracking-Flags.

### alteon_vrrp_group (vrrpNewCfgVirtRtrGrpTable)
Feldgleich, aber **ohne** `addr`/`ipv6_addr` (die Gruppe trägt keine eigene IP).

## HCL-Beispiel

```hcl
resource "alteon_vrrp" "vr_web" {
  index    = 1            # /c/l3/vrrp/vr 1  (Tabellen-Key Indx)
  vrid     = 10           # VRID auf dem Draht (ID) — darf abweichen
  addr     = "10.12.188.1"
  if_index = 1
  priority = 100
  preempt  = true
  state    = true

  track_ip_intf     = true
  track_real_server = true
}

resource "alteon_vrrp_group" "grp_ha" {
  index    = 1
  vrid     = 1
  if_index = 1
  priority = 100
  preempt  = true
  state    = true
}
```

## Hinweis zum lokalen Bauen

In der Build-Sandbox waren die Go-Vanity-Domains (`golang.org/x/*`,
`google.golang.org/*`, `gopkg.in/*`) gesperrt. Für den Testbuild wurden temporäre
`replace`-Direktiven auf GitHub-Mirrors gesetzt; diese sind **nicht** Teil der
Abgabe — die mitgelieferte `go.mod`-Logik bleibt unverändert. In eurer Umgebung mit
normalem `GOPROXY` baut alles ohne Replaces.

## Nächste Schritte (am echten Gerät zu verifizieren)
- Ob das Gerät beim Read zusätzliche Felder zurückliefert, die wir noch nicht
  mappen (unkritisch — werden ignoriert).
- Verhalten von `interval`/`ipv6_interval` als Computed, falls das Gerät Defaults setzt.
- Dann Phase 2 (AdvHC) auf demselben Helfer-Fundament.
```
