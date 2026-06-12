# Build & Transfer — terraform-provider-alteon

Drei Rechner, drei Rollen:

| Rechner   | Internet | Rolle                                        |
|-----------|----------|----------------------------------------------|
| Build-PC  | ja       | Go installiert, baut Binary + erzeugt vendor/ |
| Kunden-GitLab | (egal) | bekommt **Sourcecode** inkl. `vendor/`      |
| host    | nein     | Terraform-Rechner, bekommt **nur die Binary** |

Kernidee: Auf dem Build-PC wird einmal mit Internet alles besorgt
(`go mod vendor`). Danach ist das Repo offline-baubar, und sowohl Binary als auch
Quellcode lassen sich ohne Netz weiterverwenden.

---

## TEIL A — einmalig: Go auf dem Build-PC installieren

```bash
cd /tmp
curl -LO https://go.dev/dl/go1.22.5.linux-amd64.tar.gz
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.22.5.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' | sudo tee /etc/profile.d/go.sh
. /etc/profile.d/go.sh
go version          # muss >= 1.21 sein (go.mod verlangt 1.21.5)
```

---

## TEIL B — auf dem Build-PC: Code holen, vendorn, bauen

### B1. Repo vom GitHub-Clone holen / aktualisieren
```bash
git clone https://github.com/thomaselsaesser/terraform-provider-alteon.git
cd terraform-provider-alteon
# Die neuen Dateien (helpers.go, resource_alteon_vrrp*.go, angepasste provider.go,
# Makefile, .gitignore) hier einspielen, falls noch nicht im Clone.
```

### B2. Dependencies ins Repo holen (BRAUCHT INTERNET — der entscheidende Schritt)
```bash
make vendor
# entspricht:  go mod tidy && go mod vendor
```
Danach existiert `vendor/` mit allen Abhängigkeiten. Ab jetzt baut alles offline.

### B3. Statische linux/amd64-Binary bauen
```bash
make build
# entspricht:  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
#              go build -mod=vendor -trimpath -ldflags="-s -w" \
#              -o terraform-provider-alteon .
file terraform-provider-alteon
# Erwartung: "ELF 64-bit LSB executable, x86-64, ... statically linked"
```

### B4. Binary fürs Transfer-Paket schnüren (mit Prüfsumme)
```bash
make dist
# erzeugt:  dist/terraform-provider-alteon
#           dist/terraform-provider-alteon.sha256
```

---

## TEIL C — Sourcecode ins Kunden-GitLab

Der komplette Quellbaum INKLUSIVE `vendor/` geht ins GitLab. Die Binary NICHT
(die ist in `.gitignore` und gehört nicht in den Quellbaum).

```bash
# noch auf dem Build-PC, im Repo-Verzeichnis:
git add .
git status            # pruefen: vendor/ ist dabei, terraform-provider-alteon NICHT
git commit -m "VRRP-Ressourcen (Phase 0+1), vendored fuer Offline-Build"

# Kunden-GitLab als zweites Remote hinzufuegen:
git remote add gitlab https://<kunden-gitlab>/<gruppe>/terraform-provider-alteon.git
git push gitlab main
```

Warum `vendor/` mit muss: Damit jemand im abgeschotteten Kundennetz das Repo
auschecken und **ohne Internet** mit `make build` neu bauen kann. Ohne `vendor/`
braeuchte der Build dort einen Go-Modul-Proxy.

> Wenn der Push uebers Netz nicht geht (Kundennetz getrennt): Repo als Bundle
> transferieren — `git bundle create alteon.bundle --all` auf dem Build-PC,
> Datei ruebertragen, drueben `git clone alteon.bundle`.

---

## TEIL D — Binary auf host (Terraform-Rechner, kein Internet)

Terraform findet lokale Provider ueber einen festen Pfad-Aufbau:
`<plugin-dir>/<namespace>/<name>/<version>/<os>_<arch>/<binary>`

### D1. Binary + Prüfsumme transferieren
`dist/terraform-provider-alteon` und `.sha256` per scp/USB auf host bringen.
Auf host die Prüfsumme verifizieren:
```bash
sha256sum -c terraform-provider-alteon.sha256
```

### D2. In den Terraform-Plugin-Pfad legen
```bash
VER=0.1.0
NS=slb/alteon
DEST=~/.terraform.d/plugins/$NS/$VER/linux_amd64
mkdir -p "$DEST"
cp terraform-provider-alteon "$DEST/terraform-provider-alteon_v$VER"
chmod +x "$DEST/terraform-provider-alteon_v$VER"
```
(Der Versions-Suffix `_v$VER` im Dateinamen ist die von Terraform erwartete
Namenskonvention fuer den Filesystem-Mirror.)

### D3. Terraform-Konfiguration auf host
In eurer `.tf`-Konfiguration den Provider so referenzieren:
```hcl
terraform {
  required_providers {
    alteon = {
      source  = "slb/alteon"
      version = "0.1.0"
    }
  }
}

provider "alteon" {
  ip       = "10.32.69.17"          # oder per ALTEON_IP
  username = var.alteon_user        # oder per ALTEON_USERNAME
  password = var.alteon_pass        # oder per ALTEON_PASSWORD (sensitiv!)
}
```

### D4. Initialisieren — komplett offline
```bash
terraform init
# Terraform findet den Provider im lokalen Plugin-Pfad, KEIN Download.
terraform plan
```

> Falls Terraform meckert, es koenne den Provider nicht "verifizieren":
> `terraform init` mit der lokalen Quelle braucht keinen Registry-Zugriff, aber
> achte darauf, dass `source` exakt der Pfad-Namespace ist (slb/alteon)
> und die Verzeichnisstruktur darunter stimmt.

---

## Spaeter: neue Version bauen

Bei Code-Aenderungen (z.B. Phase 2 AdvHC):
1. `VERSION` im Makefile hochzaehlen (z.B. 0.2.0).
2. Wenn sich Dependencies geaenderten haben: `make vendor` (Internet).
   Sonst entfaellt das — `vendor/` bleibt gueltig.
3. `make dist` → neue Binary.
4. Code ins GitLab pushen, Binary auf host in den neuen `0.2.0`-Pfad legen.
5. `version` in der Terraform-Config anpassen, `terraform init -upgrade`.

---

## Schnell-Referenz (Make-Targets)

```
make vendor    # Internet noetig: Dependencies ins Repo holen
make build     # statische linux/amd64-Binary
make dist      # Binary + sha256 ins dist/ fuer Transfer
make verify    # prueft offline-Baubarkeit aus vendor/
make vet       # go vet
make clean     # aufraeumen
```
