---
title: "Reorganización del repo: binario a `cmd/enu/`, archivo del spike, destilado del handoff y `docs/plan`·`docs/postponed` a un-fichero-por-entrada"
type: "sesion"
id: "REORG"
status: "en-curso"
date: "2026-07-19"
---
# REORG — Reorganización del repo (2026-07-19)

Esto **no es una sesión del plan** (el plan está cerrado): es una pasada de
organización pedida por el operador para «dejar el repo lo más organizado
posible». Cuatro frentes independientes, cada uno con su verificación. La API
sagrada (`api.md`) **no se toca**: es puramente estructura. Registrado por el
flujo de diseño (CLAUDE.md) con dos ADR de decisión ([ADR-030](../decisions/adr/adr-030-el-binario-vive-en-cmd-enu.md)
y el de `plan/postponed`) y esta entrada de worklog.

## A · El binario a `cmd/enu/` (ADR-030)

**Motivación.** 11 `.go` sueltos en la raíz (5 fuentes + 6 tests `main_*`, todos
`package main`) no es el layout idiomático de Go; el usuario lo señaló como el
síntoma principal («archivos en la raíz, algunos tests»).

**Qué se hizo.** `git mv` de las 5 fuentes a `cmd/enu/` (sigue `package main`,
sin cambios de imports ni `go:embed`) y de los 6 tests con renombrado a su
fuente (`main_doctor_test.go`→`doctor_test.go`, etc.; `main_test.go` se
conserva). El *target* de build pasó de `.` a `./cmd/enu` en **todos** los
puntos funcionales: `ci.yml`, `release.yml` (build + `go run` de la sonda de
versión), `smoke-instalacion.yml` (×2), `docker/Dockerfile`,
`docker/docker-compose.yml`, el harness de `e2e/` (`TestMain`), `install.sh`, la
doc de instalación de la web (ES/EN) y los tres ejemplos (`go run ./cmd/enu`).
También se actualizó la única prosa **viva** que ubicaba la CLI en `main.go`
(`docs/core/arquitectura.md` §nº5). La prosa **congelada** (ADR-026 §«vive en
`main.go`», auditorías cerradas, worklog previo, el ejemplo de ADR-013) **no se
reescribe**; ADR-030 refina la ubicación.

**Decisión de alcance.** `cmd/enu/` y no `internal/cli/` + `main.go` fino: un
único binario no justifica partir `package main` en un paquete exportado (YAGNI;
razonamiento en ADR-030).

**Residuo de ADR-022 corregido de paso.** `.gitignore` ignoraba `/nu` (nombre
pre-rename) y por tanto **no** ignoraba el binario compilado `/enu`; se cambió a
`/enu`. Se detectó porque un `enu` de 20 MB del build local casi entra en el
commit.

**Verificación.** `go build ./...` ✓ · `go build ./cmd/enu` ✓ · `gofmt -l .`
vacío ✓ · `go vet ./...` ✓ · `go test ./cmd/enu` ✓ · smoke
`enu -e 'return enu.version.api'` → `5` ✓ · e2e smoke (rebuild del binario vía
`TestMain` desde `./cmd/enu`) ✓. `go build ./...`, `gofmt` y golangci sobre todo
el repo siguen válidos: `cmd/enu` es un paquete más bajo `./...`.
