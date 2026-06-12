# Makefile fuer terraform-provider-alteon
# Ziel: statische linux/amd64-Binary, offline-faehiges Repo via vendor/.

BINARY      := terraform-provider-alteon
# Version fuer den Terraform-Plugin-Pfad. Bei jeder Aenderung hochzaehlen.
VERSION     := 0.1.0
# Terraform-Plugin-Namespace (lokaler Provider). Passt zu required_providers unten.
NAMESPACE   := slb/alteon

# Statisch + reproduzierbar: CGO aus, Pfade getrimmt, Symbole gestrippt.
GOFLAGS     := -trimpath
LDFLAGS     := -s -w
BUILDENV    := CGO_ENABLED=0 GOOS=linux GOARCH=amd64

.PHONY: all build vendor verify clean dist install-local fmt vet

all: build

## build: statische linux/amd64-Binary aus vendor/ bauen
build:
	$(BUILDENV) go build -mod=vendor $(GOFLAGS) -ldflags="$(LDFLAGS)" -o $(BINARY) .
	@echo "Gebaut: $(BINARY)"
	@file $(BINARY) || true

## vendor: alle Abhaengigkeiten ins Repo holen (BRAUCHT INTERNET, einmalig/bei Dep-Aenderung)
vendor:
	go mod tidy
	go mod vendor
	@echo "vendor/ erzeugt. Jetzt ist das Repo offline-baubar."

## verify: pruefen, dass vendor/ konsistent ist (kein Netz noetig)
verify:
	go mod verify
	go build -mod=vendor ./... >/dev/null && echo "OK: baut offline aus vendor/"

## fmt/vet: Hygiene
fmt:
	gofmt -l -w .
vet:
	go vet -mod=vendor ./...

## dist: Binary fuer den Transfer zu host vorbereiten (mit Pruefsumme)
dist: build
	@mkdir -p dist
	@cp $(BINARY) dist/
	@cd dist && sha256sum $(BINARY) > $(BINARY).sha256
	@echo "dist/$(BINARY) + .sha256 bereit zum Transfer auf host."

## install-local: Binary auf DIESEM Rechner in den Terraform-Plugin-Pfad legen
## (zum Testen auf dem Build-Rechner; auf host macht das die Anleitung manuell)
install-local: build
	@mkdir -p ~/.terraform.d/plugins/$(NAMESPACE)/$(VERSION)/linux_amd64
	@cp $(BINARY) ~/.terraform.d/plugins/$(NAMESPACE)/$(VERSION)/linux_amd64/
	@echo "Installiert nach ~/.terraform.d/plugins/$(NAMESPACE)/$(VERSION)/linux_amd64/"

clean:
	rm -f $(BINARY)
	rm -rf dist
