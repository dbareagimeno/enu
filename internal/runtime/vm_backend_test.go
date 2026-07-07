package runtime

// Tests de M04: el selector de backend de VM (DM2) y la infraestructura de la
// suite DUAL. La resolución tiene precedencia WithVMBackend > NU_VM > nu.toml >
// default. Desde M16 (la conmutación) el default es **wasm**; gopher queda como
// legacy accesible tras `backend = "gopher"` / `NU_VM=gopher` hasta M17.

import (
	"os"
	"path/filepath"
	"testing"
)

// skipIfWasm es la costura de la LISTA DE SKIPS de la suite dual (migracion-vm.md
// §3). Un test que aún NO funciona sobre el backend wasm lo llama al empezar: en
// modo `NU_VM=wasm` se salta, en gopher corre normal. La regla es que cada sesión
// M05-M13 RECORTA la lista (quita skips conforme porta módulos), nunca la amplía;
// en M13 no debe quedar ninguno. Hasta que `New` ramifique por backend (M05+),
// la suite de internal/runtime corre siempre sobre gopher aunque NU_VM=wasm, así
// que este helper todavía no salta nada — queda cableado para cuando toque.
func skipIfWasm(t *testing.T) {
	t.Helper()
	if os.Getenv("NU_VM") == "wasm" {
		t.Skip("pendiente de portar al backend wasm (migracion-vm.md, lista de skips)")
	}
}

// TestVMBackendDefaultWasm: sin nada, el backend es wasm (el default desde M16).
func TestVMBackendDefaultWasm(t *testing.T) {
	// Aísla del entorno: si el runner puso NU_VM, lo neutralizamos para este caso.
	t.Setenv("NU_VM", "")
	rt := New(WithDataDir(t.TempDir()), WithConfigDir(t.TempDir()))
	defer rt.Close()
	if rt.VMBackend() != VMWasm {
		t.Fatalf("default: got %v, want wasm", rt.VMBackend())
	}
}

// TestVMBackendEnv: NU_VM selecciona el backend, incluido el path legacy gopher
// (vivo hasta M17). NU_VM=wasm confirma que el nombre explícito sigue resolviendo.
func TestVMBackendEnv(t *testing.T) {
	t.Setenv("NU_VM", "gopher")
	rt := New(WithDataDir(t.TempDir()), WithConfigDir(t.TempDir()))
	defer rt.Close()
	if rt.VMBackend() != VMGopher {
		t.Fatalf("NU_VM=gopher: got %v, want gopher (legacy)", rt.VMBackend())
	}

	t.Setenv("NU_VM", "wasm")
	rt2 := New(WithDataDir(t.TempDir()), WithConfigDir(t.TempDir()))
	defer rt2.Close()
	if rt2.VMBackend() != VMWasm {
		t.Fatalf("NU_VM=wasm: got %v, want wasm", rt2.VMBackend())
	}
}

// TestVMBackendToml: nu.toml [vm] backend selecciona cuando no hay NU_VM. Se
// prueba con el path legacy `backend = "gopher"`: al ser wasm el default desde
// M16, seleccionar gopher demuestra que el toml enruta al backend NO-default.
func TestVMBackendToml(t *testing.T) {
	t.Setenv("NU_VM", "")
	cfg := t.TempDir()
	if err := os.WriteFile(filepath.Join(cfg, "nu.toml"),
		[]byte("[vm]\nbackend = \"gopher\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rt := New(WithDataDir(t.TempDir()), WithConfigDir(cfg))
	defer rt.Close()
	if rt.VMBackend() != VMGopher {
		t.Fatalf("[vm] backend=gopher: got %v, want gopher (legacy)", rt.VMBackend())
	}
}

// TestVMBackendPrecedencia: la Option gana sobre NU_VM, que gana sobre nu.toml.
func TestVMBackendPrecedencia(t *testing.T) {
	t.Setenv("NU_VM", "gopher")
	cfg := t.TempDir()
	if err := os.WriteFile(filepath.Join(cfg, "nu.toml"),
		[]byte("[vm]\nbackend = \"wasm\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Option wasm gana sobre NU_VM=gopher gana sobre toml=wasm.
	rt := New(WithDataDir(t.TempDir()), WithConfigDir(cfg), WithVMBackend(VMWasm))
	defer rt.Close()
	if rt.VMBackend() != VMWasm {
		t.Fatalf("Option debía ganar: got %v, want wasm", rt.VMBackend())
	}
	// Sin la Option, NU_VM=gopher gana sobre el toml=wasm.
	rt2 := New(WithDataDir(t.TempDir()), WithConfigDir(cfg))
	defer rt2.Close()
	if rt2.VMBackend() != VMGopher {
		t.Fatalf("NU_VM debía ganar sobre toml: got %v, want gopher", rt2.VMBackend())
	}
}
