package runtime

import (
	"context"
	"errors"
	"strconv"
	"strings"
)

// EvalString compila y ejecuta `code` como un chunk Lua sobre el ESTADO PRINCIPAL
// de la Instance wasm (no como task) y devuelve sus valores de retorno convertidos
// a string (vía `tostring`), en orden. Es lo que respalda `nu -e`: el chunk
// `return nu.version.api` produce `["2"]` (G32 lo subió de 1).
//
// El chunk puede lanzar tasks con `nu.task.spawn` pero no usar funciones ⏸ (que
// exigen estar en una task, §1.3). Tras evaluarlo, `RunTasks` drena las tasks que
// haya lanzado (el equivalente de `waitIdle`) antes de devolver sus valores.
//
// Si el chunk lanza un error estructurado del core (§1.4), se devuelve como
// `*StructuredError` con su `code`/`message`; un error de sintaxis o un
// `error("string")` cualquiera se devuelve tal cual. Un fallo de construcción del
// estado wasm (`buildWasmState`) se reporta aquí, aplazado desde `New` (rt.wasmErr).
func (rt *Runtime) EvalString(code string) ([]string, error) {
	if rt.wasmErr != nil {
		return nil, rt.wasmErr
	}
	// El chunk se envuelve en un `pcall` cuyos retornos se capturan con table.pack.
	// Así se logran DOS cosas a la vez:
	//  1) Se preserva el RECUENTO EXACTO de valores de retorno: un `return ""` da UN
	//     valor "" (no cero), y un `return a, b` da dos.
	//  2) El error se captura COMO VALOR Lua (la tabla estructurada intacta), no como
	//     texto ya rendido por el shim. Sin el pcall, un `error({code=...})` en el
	//     ESTADO PRINCIPAL se popea en nu_eval y sólo sobrevive su `luaL_tolstring`
	//     ("table: 0x..."), perdiendo el code/message. Con él leemos e.code/e.message
	//     en Lua y reconstruimos el *StructuredError fiel (mismo truco que el camino
	//     de task en evalTaskWrapper). El chunk corre en el ESTADO PRINCIPAL (no task):
	//     puede lanzar tasks pero no usar ⏸ directo (§1.3).
	_, luaErr, goErr := rt.wasm.Eval(evalStringWrapper(code))
	if goErr != nil {
		return nil, goErr // trap del motor wasm: fallo duro
	}
	if luaErr != "" {
		// El wrapper no compiló (sintaxis en `code`): sólo hay texto.
		return nil, wasmChunkError(luaErr)
	}
	// Sondea el desenlace: si el chunk lanzó, reconstruye el error ANTES de drenar
	// tasks (el fallo del chunk se devuelve sin drenar).
	okStr, _, _ := rt.wasm.Eval("return tostring(__es_ok)")
	if okStr != "true" {
		codeStr, _, _ := rt.wasm.Eval("return tostring(__es_err_code)")
		if codeStr != "nil" {
			msgStr, _, _ := rt.wasm.Eval("return tostring(__es_err_msg or '')")
			return nil, &StructuredError{Code: codeStr, Message: msgStr}
		}
		strStr, _, _ := rt.wasm.Eval("return tostring(__es_err_str)")
		return nil, wasmChunkError(strStr)
	}
	// Drena las tasks que el chunk haya lanzado (sus efectos y liberaciones deben
	// completar antes de devolver).
	if err := rt.wasm.RunTasks(context.Background()); err != nil {
		return nil, err
	}
	// Lee el recuento y serializa cada valor con tostring (leído de uno en uno para
	// no depender de un delimitador que un valor podría contener).
	nStr, _, _ := rt.wasm.Eval("return tostring(__es_n)")
	n, err := strconv.Atoi(nStr)
	if err != nil || n < 0 {
		return nil, nil
	}
	results := make([]string, 0, n)
	for i := 1; i <= n; i++ {
		v, lerr, gerr := rt.wasm.Eval("return tostring(__es[" + strconv.Itoa(i) + "])")
		if gerr != nil {
			return nil, gerr
		}
		if lerr != "" {
			return nil, wasmChunkError(lerr)
		}
		results = append(results, v)
	}
	return results, nil
}

// evalStringWrapper envuelve `code` en un pcall que captura su desenlace (recuento de
// retornos y error estructurado) en globales `__es_*`, de forma análoga a
// evalTaskWrapper pero SIN spawnear una task: el chunk corre en el estado principal.
// Los globales se reinician al principio para que una llamada previa no filtre estado.
func evalStringWrapper(code string) string {
	return `__es_ok = nil; __es_n = 0; __es = nil
__es_err_code = nil; __es_err_msg = nil; __es_err_str = nil
local __packed = table.pack(pcall(function()
` + code + `
end))
__es_ok = __packed[1]
if __es_ok then
  __es_n = __packed.n - 1
  __es = {}
  for i = 2, __packed.n do __es[i - 1] = __packed[i] end
else
  local e = __packed[2]
  if type(e) == "table" and type(e.code) == "string" then
    __es_err_code = e.code
    __es_err_msg = e.message
  else
    __es_err_str = tostring(e)
  end
end`
}

// EvalTaskString compila `code` y lo ejecuta **como una task** (§3), no como el
// chunk principal: a diferencia de `EvalString`, aquí el chunk corre sobre su propio
// thread con el puente de suspensión disponible, de modo que puede llamar directamente
// a `nu.fs.read`, `nu.http.stream`, `Session:send` del agente, etc. Espera a que la
// task —y cualquier otra que ella lance— termine, y devuelve sus valores de retorno
// convertidos a string (vía `tostring`), en orden.
//
// Es el **ejecutor headless** del binario: respalda los modos del CLI que orquestan
// extensiones suspendientes sin TTY (un turno de agente headless, `--continue`), la
// contraparte ⏸ de `nu -e`. NO es superficie Lua sagrada (igual que `EvalString` o
// `RenderBareScreen`): es la interfaz Go del ejecutable, fuera de api.md. El core
// sigue sin saber lo que es un agente (ADR-003): aquí solo corre un chunk Lua a
// término; la lógica de agente vive en la extensión `agent` y en el driver Lua que
// el CLI le pasa (main.go).
//
// El chunk se envuelve en una task cuyo `pcall` captura su desenlace (primer valor de
// retorno o error) en globales, para que la task nunca lance —el scheduler Lua captura
// los errores por task, no escapan de RunTasks— y podamos leer el resultado tras
// drenar el bucle. El error estructurado SÍ cruza fiel aquí (se lee la tabla en Lua),
// a diferencia de EvalString, donde el puente sólo expone el texto del error.
func (rt *Runtime) EvalTaskString(code string) ([]string, error) {
	if rt.wasmErr != nil {
		return nil, rt.wasmErr
	}
	if _, luaErr, goErr := rt.wasm.Eval(evalTaskWrapper(code)); goErr != nil {
		return nil, goErr
	} else if luaErr != "" {
		// El wrapper no compiló (sintaxis en `code`) o el propio spawn falló.
		return nil, wasmChunkError(luaErr)
	}
	if err := rt.wasm.RunTasks(context.Background()); err != nil {
		return nil, err
	}
	outcome, luaErr, goErr := rt.wasm.Eval(evalTaskProbe)
	if goErr != nil {
		return nil, goErr
	}
	if luaErr != "" {
		return nil, errors.New(luaErr)
	}
	n, err := parseEvalTaskOutcome(outcome)
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, nil
	}
	// Lee cada valor de retorno con tostring, de uno en uno (como EvalString): no
	// depende de un delimitador que un valor podría contener. Es lo que deja al CLI leer
	// `results[2]` (el estado DENIED/OK del driver), no sólo el texto.
	results := make([]string, 0, n)
	for i := 1; i <= n; i++ {
		v, lerr, gerr := rt.wasm.Eval("return tostring(__eval_results[" + strconv.Itoa(i) + "])")
		if gerr != nil {
			return nil, gerr
		}
		if lerr != "" {
			return nil, wasmChunkError(lerr)
		}
		results = append(results, v)
	}
	return results, nil
}

// evalTaskWrapper envuelve `code` en una task que captura su desenlace en globales.
// `code` se inserta como CUERPO de una función (no como string literal: sin
// escapado ni riesgo de inyección), con saltos de línea alrededor para que un
// comentario de línea al final de `code` no se trague el `end`. `table.pack`
// preserva el recuento de retornos (multi-valor futuro). Los globales se reinician
// al principio para que una llamada previa no filtre estado.
func evalTaskWrapper(code string) string {
	return `__eval_ok = nil; __eval_n = 0; __eval_results = nil
__eval_err_code = nil; __eval_err_msg = nil; __eval_err_str = nil
nu.task.spawn(function()
  local packed = table.pack(pcall(function()
` + code + `
  end))
  __eval_ok = packed[1]
  if __eval_ok then
    __eval_n = packed.n - 1
    __eval_results = {}
    for i = 2, packed.n do __eval_results[i - 1] = packed[i] end
  else
    local e = packed[2]
    if type(e) == "table" and type(e.code) == "string" then
      __eval_err_code = e.code
      __eval_err_msg = e.message
    else
      __eval_err_str = tostring(e)
    end
  end
end)`
}

// evalTaskProbe es el chunk que lee el desenlace que dejó evalTaskWrapper y lo
// codifica en un string delimitado por 0x01 (SOH, que no aparece en códigos ni
// mensajes normales): "N" sin valores, "V\1<n>" con N valores de retorno (que el
// llamante lee de uno en uno de __eval_results, para no depender de un delimitador
// que un valor podría contener), "E\1<code>\1<msg>" error estructurado, "X\1<texto>"
// error simple.
const evalTaskProbe = `
if __eval_ok ~= true then
  if __eval_err_code ~= nil then
    return "E\1" .. __eval_err_code .. "\1" .. tostring(__eval_err_msg or "")
  end
  return "X\1" .. tostring(__eval_err_str or "la task del CLI no produjo resultado")
end
if __eval_n == 0 then return "N" end
return "V\1" .. tostring(__eval_n)`

// parseEvalTaskOutcome traduce el header que emitió evalTaskProbe. Para el caso "V"
// devuelve el NÚMERO de valores en `n` (con ok=true); el llamante los lee de
// __eval_results. Para "N"/"E"/"X" el desenlace es completo (n=0).
func parseEvalTaskOutcome(outcome string) (n int, err error) {
	switch {
	case outcome == "N":
		return 0, nil // sin valores de retorno (slice vacío)
	case strings.HasPrefix(outcome, "V\x01"):
		count, cerr := strconv.Atoi(outcome[len("V\x01"):])
		if cerr != nil || count < 0 {
			return 0, nil
		}
		return count, nil
	case strings.HasPrefix(outcome, "E\x01"):
		parts := strings.SplitN(outcome[len("E\x01"):], "\x01", 2)
		se := &StructuredError{Code: parts[0]}
		if len(parts) == 2 {
			se.Message = parts[1]
		}
		return 0, se
	case strings.HasPrefix(outcome, "X\x01"):
		return 0, errors.New(outcome[len("X\x01"):])
	default:
		return 0, errors.New("EvalTaskString: desenlace de task no reconocido: " + outcome)
	}
}

// wasmChunkError traduce el mensaje de error (string) que el backend wasm entrega
// al evaluar un chunk en el ESTADO PRINCIPAL (Instance.Eval) a un error Go. El
// puente sólo expone el error ya rendido a texto por el shim (luaL_tolstring): la
// tabla estructurada original se popea en nu_eval y no sobrevive (a diferencia de
// EvalTaskString, que la lee en Lua). Se recupera como *StructuredError si el texto
// tiene la forma "CODE: mensaje" con un code reservado (best-effort); si no, un
// error simple.
func wasmChunkError(msg string) error {
	if code, rest, ok := strings.Cut(msg, ": "); ok && IsReservedCode(code) {
		return &StructuredError{Code: code, Message: rest}
	}
	return errors.New(msg)
}

// SetStringGlobal fija un global Lua de tipo string desde Go. Es la vía por la que
// el BINARIO (main.go) pasa sus argumentos de línea de comandos —el prompt del
// agente, el modelo, los flags— al **driver Lua** del CLI SIN interpolarlos en el
// código (lo que abriría una inyección a través de un prompt con comillas o saltos
// de línea). Igual que `EvalTaskString`/`RenderBareScreen`, es interfaz Go del
// ejecutable, NO superficie Lua sagrada (fuera de api.md). El global vive en el
// estado Lua de la Instance (`SetGlobalString`, sin interpolar). Un fallo del motor
// wasm es best-effort aquí (la firma no devuelve error).
func (rt *Runtime) SetStringGlobal(name, value string) {
	if rt.wasm != nil {
		_ = rt.wasm.SetGlobalString(name, value)
	}
}
