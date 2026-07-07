package runtime

import "os"

// VMBackend selecciona el motor de VM sobre el que corre el estado Lua del
// Runtime (migracion-vm.md M04, DM2). Es el eje del **patrón estrangulador**: el
// backend `wasm` (PUC-Lua oficial sobre wazero, internal/vmwasm) se construye en
// paralelo al `gopher` actual, detrás de este selector, hasta que la paridad
// fuese total (M15) y la conmutación (M16) cambiase el default. Desde M16 el
// default ES wasm; gopher queda como legacy accesible tras `backend = "gopher"`
// (o `NU_VM=gopher`) durante el ciclo de gracia. gopher-lua se retira en M17 y
// este selector con él.
type VMBackend int

const (
	// VMGopher es el backend legacy (gopher-lua). Desde M16 ya no es el default:
	// hay que pedirlo explícitamente (`backend = "gopher"` / `NU_VM=gopher`). Se
	// retira en M17.
	VMGopher VMBackend = iota
	// VMWasm es el backend por defecto desde M16 (PUC-Lua sobre wazero,
	// internal/vmwasm): la VM productiva del kernel.
	VMWasm
)

func (b VMBackend) String() string {
	if b == VMWasm {
		return "wasm"
	}
	return "gopher"
}

// parseVMBackend traduce un nombre ("gopher"|"wasm") a VMBackend. Desde M16 el
// default es wasm, así que sólo el nombre explícito "gopher" selecciona el
// backend legacy; cualquier otro valor (incluido un nombre desconocido) resuelve
// a wasm, coherente con el nuevo default.
func parseVMBackend(name string) VMBackend {
	if name == "gopher" {
		return VMGopher
	}
	return VMWasm
}

// resolveVMBackend fija el backend de esta construcción con la precedencia de
// DM2: la variable de entorno `NU_VM` (la vía de los tests: `NU_VM=gopher go test`)
// gana sobre `nu.toml [vm] backend`, que gana sobre el default. Desde M16 (la
// conmutación) ese default es **wasm**; gopher sólo entra si se pide explícito.
// La Option `WithVMBackend` (si se pasó) gana sobre todo (precedencia de test
// explícito, como el resto de Options).
func resolveVMBackend(cfg *config, tomlBackend string) VMBackend {
	if cfg.vmBackendSet {
		return cfg.vmBackend
	}
	if env := os.Getenv("NU_VM"); env != "" {
		return parseVMBackend(env)
	}
	if tomlBackend != "" {
		return parseVMBackend(tomlBackend)
	}
	return VMWasm
}
