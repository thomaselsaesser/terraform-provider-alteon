# server_group — neues deklaratives Modell

## Die Kernidee in einem Satz

**Eine Gruppe = eine Terraform-Ressource. Die Mitglieder stehen als Liste drin.**
Nicht mehr: eine Ressource pro „füge Server X hinzu".

---

## Vorher (altes Modell) — so NICHT mehr

Drei Ressourcen für EINE Gruppe, alle mit `id = "12"`:

```hcl
resource "alteon_server_group" "server_group_12" {
  index = "12"
  elements { ipver = 1 }
}
resource "alteon_server_group" "server_group_12_add_15" {
  index = "12"
  depends_on = [alteon_server_group.server_group_12]
  elements { addserver = "15" }
}
resource "alteon_server_group" "server_group_12_add_16" {
  index = "12"
  depends_on = [alteon_server_group.server_group_12_add_15]
  elements { addserver = "16" }
}
```

**Warum das nicht funktioniert:** Drei State-Einträge zeigen auf dasselbe Geräte-
Objekt (Gruppe 12). Beim Read liest jeder die *ganze* Gruppe zurück → die beiden
`_add`-Ressourcen sehen Felder, die laut ihrer Config nicht da sein dürften →
Phantom-Diff, „change", „destroy", „Resource has no configuration". Sackgasse.

---

## Nachher (neues Modell) — so JETZT

EINE Ressource. Mitglieder als Set. Einstellungen flach daneben.

```hcl
resource "alteon_server_group" "grp12" {
  index   = "12"
  servers = ["15", "16"]      # vollständige Soll-Menge der Mitglieder

  name               = "intranet-wahlen"
  metric             = "leastconnections"
  health_check_layer = "tcp"
}
```

Server hinzufügen? Liste ergänzen:
```hcl
  servers = ["15", "16", "17"]
```
Server entfernen? Aus der Liste streichen. Terraform berechnet das Delta selbst
(intern AddServer/RemoveServer) — du beschreibst nur den Soll-Zustand.

---

## Felder

| Feld | Pflicht | Bedeutung |
|------|---------|-----------|
| `index` | ja | Gruppen-ID (Tabellen-Key) |
| `servers` | nein | Set der Real-Server-Indizes (Mitglieder) |
| `name` | nein | Anzeigename |
| `metric` | nein | `roundrobin\|leastconnections\|minmisses\|hash\|response\|bandwidth\|phash\|svcleast\|hrw` |
| `health_check_layer` | nein | `icmp\|tcp\|http\|dns\|smtp\|link\|ldap` |
| `health_id` | nein | Health-Check-Name |
| `backup_server`, `backup_group` | nein | Backup |
| `real_threshold`, `slowstart`, `ip_ver` | nein | weitere Einstellungen |

---

## Testablauf (am ausrangierten LB-Paar)

In einem **eigenen, leeren Verzeichnis** (nicht im Kollegen-Wirrwarr):

```bash
# 1. leeren Block anlegen mit echter Gruppen-ID
cat > grp.tf <<'EOF'
resource "alteon_server_group" "grp12" {
  index = "12"
}
EOF

# 2. importieren
terraform import alteon_server_group.grp12 12

# 3. ENTSCHEIDEND: plan ansehen
terraform plan
```

- **Plan leer** → das Modell sitzt. `servers` und Einstellungen sind aus dem Gerät
  gelesen; du kannst sie ins HCL übernehmen und ab da deklarativ verwalten.
- **Plan zeigt Diffs** → die `plan`-Ausgabe schicken. Dann sehen wir, welches Feld
  (oder die Member-Tabelle) anders zurückliest als erwartet.

---

## WICHTIG — noch am Gerät zu verifizieren

Das Lesen der Mitglieder geht über die Tabelle `slbNewCfgEnhGroupRealServerTable`,
clientseitig gefiltert nach Gruppen-Index. Falls das Gerät beim GET dieser Tabelle
einen anderen Pfad erwartet (z. B. `/config/<table>/<gruppe>` statt Voll-GET),
zeigt sich das beim ersten `plan`/Import — dann wird `groupReadMembers` angepasst.
Das ist der eine offene Punkt bei dieser Ressource.
