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

## B · El spike a `docs/archive/` (borrando el código muerto)

**Motivación.** `spike/lua-wasm/` era un módulo Go aislado
(`nu-spike-lua-wasm`), sin wiring de build, ya promovido a `internal/vmwasm/`
(migración M17). La auditoría H-14 (2026-07-08) ya pedía archivarlo.

**Qué se hizo.** `git mv` de `INFORME.md` →
`docs/archive/spike-lua-wasm-informe.md` (con frontmatter `type: archivo` +
banner «archivado, histórico»); `git rm -r spike/` (módulo Go, shim C,
`build.sh`, tests y benchmarks). El código es **reproducible** desde git y desde
la receta del propio informe; los aprendizajes viven en `internal/vmwasm/`.

**Enlaces reapuntados.** `docs/archive/migracion-vm.md` (el link de evidencia y
la prosa de «piezas heredadas») y el link de evidencia de **ADR-019** — solo el
*target* del enlace markdown; la prosa histórica del ADR **no se reescribe**
(convención: los ADR no se reescriben). Las auditorías cerradas (H-14) quedan
congeladas.

**Verificación.** Ningún enlace markdown colgando a `spike/`; `go build ./...`
verde (el módulo aislado ya no existe).

## C · El handoff destilado a `web/DISENO.md` (borrando la carpeta)

**Motivación.** `design_handoff_enu_web/` (380 KB) ya estaba implementado en
`web/` y **superado** por la auditoría web 2026-07-15 (W-02 contraste AA, W-03
IBM Plex Sans); su único enlace entrante (`web/README.md`) estaba **roto** por el
rename (apuntaba a `design_handoff_nu_web`).

**Qué se hizo.** Se creó `web/DISENO.md` como **guía de diseño viva**,
destilando del `README.md` del handoff lo escrito y no capturado en otro sitio
(gramática visual «reglas duras», dimensiones por pantalla, mapa de teclado +
mini-REPL easter-egg, copy de portada ES/EN, semántica de tokens, decisiones
abiertas), con `nu→enu` corregido y una nota explícita de que los **valores
vigentes mandan** desde `tokens.css` (W-02/W-03). Se arregló el enlace roto de
`web/README.md` y se reapuntaron a `DISENO.md` los **6 comentarios de código**
que citaban «el handoff» como rationale (`tokens.css`, `const.ts`, `search.ts`
×2, `404.astro` ×2, `i18n.ts`). `git rm -r design_handoff_enu_web/` (incluidos
el prototipo HTML de 139 KB, `support.js` generado de 64 KB y las 8 screenshots;
quedan en git si hicieran falta).

**Verificación.** `check-drift` verde (113 callables); ningún ref vivo a
«handoff» salvo el propio `DISENO.md`. `DISENO.md` vive en la raíz de `web/`
(como `README.md`): **no** es página publicada. El `npm run build` completo no se
corrió en local (sin `node_modules`); los cambios web son no-renderizables (un
doc de raíz + comentarios + un enlace), y el gate `docs.yml` lo cubre en CI.
