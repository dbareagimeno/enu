package runtime

import (
	"time"
)

// Watchdog de slice (api.md §1.3, sesión S09, inventario 🔒, HITO DE VETO). Cada
// **slice** —el tramo de ejecución Lua continua que una task corre con el token
// sin suspender— tiene un **presupuesto** (100 ms por defecto, configurable, ver
// `WithSliceBudget`). Excederlo **aborta la task** con `EBUDGET` de forma **NO
// capturable** y emite `core:plugin.misbehaved`. Es la otra mitad de la robustez
// de ADR-008 (aislamiento por tarea): la cancelación (S08) ataja a una task que
// *coopera* suspendiendo; el watchdog ataja a una que **no coopera** —un bucle de
// CPU puro (`while true do end`) que jamás suelta el token—.
//
// EL PUNTO DIFÍCIL: interrumpir un slice que NO suspende. Un ⏸ (suspend/await)
// tiene un punto de chequeo cooperativo donde mirar `cancelCh`; un bucle de CPU
// puro no tiene ninguno —monopoliza el token y el intérprete sin volver nunca a
// Go—. La única palanca es el propio bucle del intérprete: gopher-lua, cuando un
// `LState` tiene contexto (`SetContext`), corre `mainLoopWithContext`, que en
// CADA instrucción comprueba `ctx.Done()` y, si está cancelado, lanza un error
// Lua (`context canceled`) que rompe el bucle. Por eso `spawn` dota a cada thread
// de task de un `context.Context` propio cancelable (scheduler.go); el watchdog,
// al disparar, lo cancela. Verificado contra gopher-lua v1.1.2: cancelar el
// contexto rompe `while true do end`; el corte se manifiesta como un `*ApiError`
// con mensaje "context canceled".
//
// INTEGRACIÓN CON EL DESENROLLADO NO CAPTURABLE DE S08. El error que gopher-lua
// inyecta por contexto es, de partida, un error Lua **normal** —un `pcall` de
// usuario que envuelva el bucle lo atraparía—. Para hacerlo **no capturable**
// (§1.3) se reusa exactamente la maquinaria de S08: el watchdog, antes de
// cancelar el contexto, marca `t.budgetExceeded` (atómico, cruza goroutines). La
// goroutine de la task, al detectar el corte, "reclama" el aborto con
// `claimBudgetAbort` —pone `aborting`/`reason = abortBudget`/`canceled` (igual que
// hace `abort` para la cancelación)— y re-lanza el centinela `abortSignal`; a
// partir de ahí los wrappers de `pcall`/`xpcall` (cancel.go) ya lo cuelan por
// cualquier `pcall` del usuario hasta `runTask`. El claim lo hace SIEMPRE la
// goroutine de la task bajo el token, así que `aborting`/`reason`/`canceled`
// siguen siendo de un solo escritor —invariante de S08 intacto—; lo único que el
// watchdog toca desde fuera es el flag atómico y el `ctxCancel` (seguro
// concurrentemente).
//
// SIN CONGELAR EL LOOP. El watchdog corre en su PROPIA goroutine
// (`time.AfterFunc`), que **no tiene el token**: por eso puede cortar a una task
// que lo monopoliza mientras otras tasks y timers esperan. Cuando el corte
// desenrolla la task hasta `runTask`, esta suelta el token (`release`), y el
// resto del sistema —otra task lista, un `every` que tickea— progresa. El loop no
// queda congelado para siempre, solo lo que dura el slice excedido (el
// presupuesto).
//
// HANDLERS SÍNCRONOS Y CLEANUPS QUEDAN FUERA. `defer`/`every` y los `cleanup`
// corren sobre threads efímeros de `host` (que no tiene contexto), no sobre el
// `co` de una task, así que el watchdog no los vigila. El alcance de S09 es el
// **slice de una task** (api.md §1.3); vigilar handlers síncronos sería otra
// pieza, fuera de esta sesión.

// armWatchdog arranca el temporizador del slice en curso de la task `t`. Lo
// llaman los puntos donde la task **toma el token para correr Lua**: el inicio de
// `runTask` y la re-adquisición tras un ⏸ (`suspend`, `Task:await`,
// `Future:await`). Si el temporizador dispara antes de `disarmWatchdog`, el slice
// excedió el presupuesto: corre `fireWatchdog` (en la goroutine del timer).
//
// Lo llama siempre la goroutine de la task con el token tomado, así que escribir
// `t.budgetTimer` no compite con nadie. Un `budget <= 0` desactiva el watchdog
// (útil en tests que no lo quieren); en producción el default es 100 ms.
func (s *scheduler) armWatchdog(t *task) {
	if s.budget <= 0 || t.ctxCancel == nil {
		return
	}
	t.budgetTimer = time.AfterFunc(s.budget, func() { s.fireWatchdog(t) })
}

// disarmWatchdog para el temporizador del slice que acaba de terminar (la task
// suspende o retorna). Lo llaman los mismos puntos que `armWatchdog`, justo antes
// de **soltar el token**. Si el timer ya disparó, `Stop()` devuelve false y no
// pasa nada: el aborto ya está en marcha (lo reclamará la goroutine de la task).
// Lo llama la goroutine de la task con el token, así que leer `t.budgetTimer` es
// seguro.
func (s *scheduler) disarmWatchdog(t *task) {
	if t.budgetTimer != nil {
		t.budgetTimer.Stop()
		t.budgetTimer = nil
	}
}

// fireWatchdog es el disparo del watchdog: corre en la goroutine del
// `time.AfterFunc`, **sin el token**, cuando un slice excedió el presupuesto.
// Marca `budgetExceeded` (atómico, lo leerá la goroutine de la task) y **cancela
// el contexto** del thread de la task —lo que rompe el bucle del intérprete en su
// siguiente instrucción (incluido un `while true do end` que jamás suspendería)—.
// No toca `aborting`/`reason`/`canceled`: esos los pone la goroutine de la task
// al reclamar el aborto (`claimBudgetAbort`), manteniendo el invariante de un
// solo escritor de S08.
func (s *scheduler) fireWatchdog(t *task) {
	t.budgetExceeded.Store(true)
	if t.ctxCancel != nil {
		t.ctxCancel() // rompe el slice de CPU puro: ctx.Done() en la próxima instrucción
	}
}

// claimBudgetAbort la llama la **goroutine de la task** (bajo el token) cuando
// detecta que el watchdog disparó para esta task. Convierte el corte por contexto
// —de partida un error Lua normal— en el **aborto no capturable** de S08, con
// `reason = abortBudget`: pone `aborting`/`reason`/`canceled` (de un solo
// escritor, como `abort`) y devuelve true para que el llamante actúe (re-lanzar el
// centinela en los wrappers de `pcall`; descartar el desenlace y emitir
// `core:plugin.misbehaved` en `runTask`). Idempotente: si ya se reclamó (la task
// ya está `canceled` por `abortBudget`), devuelve true sin volver a tocar nada;
// si el watchdog no disparó, devuelve false y no hace nada.
func (s *scheduler) claimBudgetAbort(t *task) bool {
	if !t.budgetExceeded.Load() {
		return false
	}
	t.aborting = true
	t.reason = abortBudget
	t.canceled = true
	return true
}
