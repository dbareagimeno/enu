// Package runtime levanta el intérprete Lua del core de nu: construye el estado
// gopher-lua, aplica el baseline del sandbox (api.md §1.2), inyecta el global
// `nu` y expone la evaluación de código. Es la quilla sobre la que las sesiones
// posteriores cuelgan cada submódulo de la API (task, fs, http, ...).
package runtime

import (
	"path/filepath"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// defaultSliceBudget es el presupuesto por slice del watchdog (api.md §1.3, S09):
// el tiempo máximo que una task puede correr Lua de forma continua sin suspender.
// 100 ms por defecto; `WithSliceBudget` lo ajusta (el gancho que S11/S12
// cablearán a la lectura de `nu.toml`).
const defaultSliceBudget = 100 * time.Millisecond

// Runtime envuelve un estado Lua ya sandboxeado y con el global `nu` inyectado.
// El estado principal es single-threaded (ADR-004); un Runtime se usa desde una
// sola goroutine.
type Runtime struct {
	L *lua.LState

	// sched es el event loop y el scheduler de tasks (§1.3, §3). Es la quilla:
	// `nu.task`, los puntos de suspensión ⏸ y, en adelante, todo lo async cuelga
	// de él. Una sola goroutine (la del loop) lo toca.
	sched *scheduler

	// log respalda `nu.log` (§15): un fichero append-only en data_dir.
	log *logger
	// owner es el plugin de origen que se anota en cada línea de log. Por
	// defecto "user" (código del usuario vía `-e` o `init.lua`, sin plugin
	// dueño). S11 hará que siga la pila de plugins activa; las funciones de log
	// lo leen en cada llamada, así que ese cambio será transparente aquí.
	owner string
}

// config recoge los parámetros de construcción de un Runtime. Es interno: se
// configura con Options.
type config struct {
	dataDir string
	// sliceBudget es el presupuesto por slice del watchdog (S09). Cero o negativo
	// **desactiva** el watchdog —útil para tests que no lo quieren—; el default de
	// producción es `defaultSliceBudget` (100 ms).
	sliceBudget time.Duration
}

// Option ajusta la construcción de un Runtime. El default sirve para producción
// (`nu -e`); los tests inyectan, p. ej., un data_dir temporal.
type Option func(*config)

// WithDataDir fija el directorio donde vive el estado en disco (de momento, solo
// el fichero de `nu.log`). Los tests lo apuntan a un `t.TempDir()` para no
// escribir en el data_dir real del usuario.
func WithDataDir(dir string) Option {
	return func(c *config) { c.dataDir = dir }
}

// WithSliceBudget ajusta el presupuesto por slice del watchdog (S09, api.md
// §1.3). Es el **gancho de configuración** que S11/S12 cablearán a `nu.toml`; por
// ahora lo usan los tests para fijar un presupuesto pequeño (corte rápido) o
// desactivar el watchdog (`<= 0`). En producción, sin opción, rige
// `defaultSliceBudget` (100 ms).
func WithSliceBudget(d time.Duration) Option {
	return func(c *config) { c.sliceBudget = d }
}

// New construye un Runtime listo para ejecutar Lua: abre solo las librerías
// permitidas por el baseline (§1.2), recorta `os`, elimina `io`/`dofile`/
// `loadfile`, redirige `print` a `nu.log.info` e inyecta el global `nu` con sus
// submódulos disponibles en esta sesión.
func New(opts ...Option) *Runtime {
	cfg := config{dataDir: defaultDataDir(), sliceBudget: defaultSliceBudget}
	for _, o := range opts {
		o(&cfg)
	}

	// SkipOpenLibs: abrimos a mano solo lo que el baseline permite, en vez de
	// abrir todo y desactivar después; así una librería peligrosa nueva de
	// gopher-lua no entra por defecto (deny-by-default, coherente con las caps
	// de los workers, §13).
	L := lua.NewState(lua.Options{SkipOpenLibs: true})

	rt := &Runtime{
		L:     L,
		log:   newLogger(filepath.Join(cfg.dataDir, logFileName)),
		owner: "user",
	}
	rt.sched = newScheduler(rt, cfg.sliceBudget)
	applySandbox(L)
	registerNu(rt)
	return rt
}

// emitMisbehaved es el **gancho interno** de `core:plugin.misbehaved` (api.md
// §1.3, §4, S09). El watchdog (`runTask`) lo invoca cuando una task se abortó por
// exceder el presupuesto de un slice. El bus de eventos `nu.events` llega en S10:
// hasta entonces, esto solo deja constancia en el log (best-effort, como el resto
// de fallos de task). **S10 lo cableará** a
// `nu.events.emit("core:plugin.misbehaved", { plugin = owner, reason = ... })`,
// sin cambiar la superficie pública —el watchdog ya llama a este punto único—.
func (rt *Runtime) emitMisbehaved(owner, reason string) {
	_ = rt.log.write(levelError, owner, "core:plugin.misbehaved: "+reason)
}

// Close libera el estado Lua subyacente, corta los timers periódicos activos
// (sus goroutines de ticker, para no dejarlas colgadas) y cierra el fichero de
// log si llegó a abrirse.
func (rt *Runtime) Close() {
	if rt.sched != nil {
		rt.sched.stopAllTimers()
	}
	if rt.log != nil {
		_ = rt.log.close()
	}
	rt.L.Close()
}
