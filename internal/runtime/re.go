package runtime

import (
	"regexp"
)

// `nu.re` — expresiones regulares RE2 (api.md §10, sesión S26). Cuatro
// operaciones sobre un patrón compilado: `compile` lo prepara y devuelve un
// handle `Re` con tres métodos —`match` (primera coincidencia con sus
// capturas), `find_all` (todas las coincidencias como rangos) y `replace`
// (sustituye todas)—.
//
// TODAS SON [W] PERO NINGUNA ⏸ (§10, §16). Son **CPU puro**: compilan o casan
// un patrón contra un string ya en memoria, sin IO que esperar —como los
// codecs de S18 y el resto de `nu.text`—. Por eso NO usan el puente `suspend`
// ni `requireTask`: corren síncronas en el estado principal (y en workers
// cuando lleguen, S34). [W] marca "disponible en workers", no "suspende".
//
// POR QUÉ RE2 (el `regexp` de Go). La librería estándar de Go es una
// implementación de RE2: garantiza tiempo lineal sobre el tamaño de la entrada
// (sin backtracking catastrófico) a cambio de **no** soportar backreferences
// ni lookaround. Eso es exactamente lo que un harness quiere: un patrón
// venido de un agente o de la configuración NUNCA puede colgar el runtime con
// un ReDoS. El precio —no hay `\1` ni `(?=...)`— se documenta y se reporta
// como un `EINVAL` claro (el mensaje de `regexp.Compile` nombra qué construye
// no se soporta), no como un fallo silencioso.

// reTypeName identifica la metatabla del handle `Re` (lo que devuelve
// `nu.re.compile`). De ella cuelga el `__index` con `match`/`find_all`/`replace`.
const reTypeName = "nu.re.Re"

// luaRe es el contenido Go de un handle `Re`: el patrón ya compilado. El
// `*regexp.Regexp` de la stdlib es **seguro para uso concurrente** (su doc lo
// garantiza), así que un mismo `Re` puede casarse desde varias tasks sin
// candado —encaja con el modelo de concurrencia del navegador (ADR-004)—.
type luaRe struct {
	re *regexp.Regexp
}
