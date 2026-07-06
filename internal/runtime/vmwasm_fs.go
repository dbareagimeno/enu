package runtime

// Catálogo de nu.fs sobre el backend wasm (M13b, §5). Contraparte de fs.go: las
// mismas primitivas (read/write/append/stat/list/mkdir/remove/rename/copy/tmpdir/
// cwd) reusando las MISMAS funciones Go VM-agnósticas (writeAtomic/writeExclusive/
// copyFile/fsState.ensureTmpdir) y las mismas constantes de permisos. Todas ⏸
// salvo cwd (consulta pura). El HostFn suspendente corre en una goroutine de fondo
// (contrato de RegisterSuspending): no toca la Instance, pero sí el `os` y el
// `rt.fs` (mutex-safe) — igual que la deliverFn del backend gopher hace el IO fuera
// del token.
//
// La guardia requireTask del backend gopher aquí la impone el propio mecanismo: un
// thunk ⏸ hace coroutine.yield, que fuera de una task (coroutine) ya falla.

import (
	"errors"
	"os"

	"github.com/dbareagimeno/nu/internal/vmwasm"
)

// mapFsErrorWasm traduce un error del SO al error estructurado del core (§1.4),
// mismo mapeo que mapFsError del backend gopher.
func mapFsErrorWasm(err error) error {
	switch {
	case errors.Is(err, os.ErrNotExist):
		return &vmwasm.StructuredError{Code: "ENOENT", Message: err.Error()}
	case errors.Is(err, os.ErrExist):
		return &vmwasm.StructuredError{Code: "EEXIST", Message: err.Error()}
	case errors.Is(err, os.ErrPermission):
		return &vmwasm.StructuredError{Code: "EACCES", Message: err.Error()}
	default:
		return &vmwasm.StructuredError{Code: "EIO", Message: err.Error()}
	}
}

func registerFsWasm(p *vmwasm.Pool, rt *Runtime) {
	// nu.fs.read(path) -> string ⏸
	p.RegisterSuspending("fs.read", func(inst *vmwasm.Instance, args []any) ([]any, error) {
		path, _ := args[0].(string)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, mapFsErrorWasm(err)
		}
		return []any{string(data)}, nil
	})

	// nu.fs.write(path, data, opts?) ⏸ — escritura atómica; opts.exclusive = O_EXCL.
	p.RegisterSuspending("fs.write", func(inst *vmwasm.Instance, args []any) ([]any, error) {
		path, _ := args[0].(string)
		data := []byte(argString(args, 1))
		exclusive := false
		if opts, ok := arg(args, 2).(map[string]any); ok {
			exclusive, _ = opts["exclusive"].(bool)
		}
		var err error
		if exclusive {
			err = writeExclusive(path, data)
		} else {
			err = writeAtomic(path, data)
		}
		if err != nil {
			return nil, mapFsErrorWasm(err)
		}
		return nil, nil
	})

	// nu.fs.append(path, data) ⏸
	p.RegisterSuspending("fs.append", func(inst *vmwasm.Instance, args []any) ([]any, error) {
		path, _ := args[0].(string)
		data := []byte(argString(args, 1))
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, fsFilePerm)
		if err == nil {
			_, err = f.Write(data)
			if cerr := f.Close(); err == nil {
				err = cerr
			}
		}
		if err != nil {
			return nil, mapFsErrorWasm(err)
		}
		return nil, nil
	})

	// nu.fs.stat(path) -> {size, mtime_ms, is_dir, mode}? ⏸ — inexistente → nil.
	p.RegisterSuspending("fs.stat", func(inst *vmwasm.Instance, args []any) ([]any, error) {
		path, _ := args[0].(string)
		info, err := os.Stat(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return []any{nil}, nil // inexistente → nil, NO lanza (§5)
			}
			return nil, mapFsErrorWasm(err)
		}
		return []any{map[string]any{
			"size":     info.Size(),
			"mtime_ms": info.ModTime().UnixMilli(),
			"is_dir":   info.IsDir(),
			"mode":     int64(info.Mode().Perm()),
		}}, nil
	})

	// nu.fs.list(dir) -> {name, is_dir}[] ⏸ — inexistente → ENOENT.
	p.RegisterSuspending("fs.list", func(inst *vmwasm.Instance, args []any) ([]any, error) {
		dir, _ := args[0].(string)
		des, err := os.ReadDir(dir)
		if err != nil {
			return nil, mapFsErrorWasm(err)
		}
		arr := make([]any, len(des))
		for i, de := range des {
			arr[i] = map[string]any{"name": de.Name(), "is_dir": de.IsDir()}
		}
		return []any{arr}, nil
	})

	// nu.fs.mkdir(path) ⏸ — MkdirAll (mkdir -p), idempotente.
	p.RegisterSuspending("fs.mkdir", func(inst *vmwasm.Instance, args []any) ([]any, error) {
		path, _ := args[0].(string)
		if err := os.MkdirAll(path, fsDirPerm); err != nil {
			return nil, mapFsErrorWasm(err)
		}
		return nil, nil
	})

	// nu.fs.remove(path, opts?) ⏸ — inexistente → no-op; dir no vacío exige recursive.
	p.RegisterSuspending("fs.remove", func(inst *vmwasm.Instance, args []any) ([]any, error) {
		path, _ := args[0].(string)
		recursive := false
		if opts, ok := arg(args, 1).(map[string]any); ok {
			recursive, _ = opts["recursive"].(bool)
		}
		var err error
		if recursive {
			err = os.RemoveAll(path)
		} else {
			err = os.Remove(path)
			if errors.Is(err, os.ErrNotExist) {
				err = nil // idempotente
			}
		}
		if err != nil {
			return nil, mapFsErrorWasm(err)
		}
		return nil, nil
	})

	// nu.fs.rename(from, to) ⏸
	p.RegisterSuspending("fs.rename", func(inst *vmwasm.Instance, args []any) ([]any, error) {
		if err := os.Rename(argString(args, 0), argString(args, 1)); err != nil {
			return nil, mapFsErrorWasm(err)
		}
		return nil, nil
	})

	// nu.fs.copy(from, to) ⏸ — copia en streaming.
	p.RegisterSuspending("fs.copy", func(inst *vmwasm.Instance, args []any) ([]any, error) {
		if err := copyFile(argString(args, 0), argString(args, 1)); err != nil {
			return nil, mapFsErrorWasm(err)
		}
		return nil, nil
	})

	// nu.fs.tmpdir() -> string ⏸ — el scratch de la sesión (compartido, rt.fs).
	p.RegisterSuspending("fs.tmpdir", func(inst *vmwasm.Instance, args []any) ([]any, error) {
		dir, err := rt.fs.ensureTmpdir()
		if err != nil {
			return nil, mapFsErrorWasm(err)
		}
		return []any{dir}, nil
	})

	// nu.fs.cwd() -> string — la ÚNICA de fs que NO es ⏸ (consulta pura).
	p.Register("fs.cwd", func(inst *vmwasm.Instance, args []any) ([]any, error) {
		dir, err := os.Getwd()
		if err != nil {
			return nil, mapFsErrorWasm(err)
		}
		return []any{dir}, nil
	})
}

// arg / argString: acceso seguro a args de un HostFn.
func arg(args []any, i int) any {
	if i >= len(args) {
		return nil
	}
	return args[i]
}

func argString(args []any, i int) string {
	s, _ := arg(args, i).(string)
	return s
}
