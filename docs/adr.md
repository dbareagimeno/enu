# Registro de decisiones técnicas (ADR)

Formato ligero: contexto → decisión → consecuencias. Una entrada por decisión,
numeradas, nunca se reescriben: si una decisión cambia, se añade una nueva que
la reemplaza (supersede).

Estados: **Aceptada** · **Propuesta** · **Abierta** (aún sin decisión) ·
**Reemplazada por ADR-NNN**.

---

## ADR-001 · Go como lenguaje del core

**Estado:** Aceptada · 2026-06

**Contexto.** El proyecto nace como reacción al dependency hell de JS/TS en los
harnesses actuales. Necesitamos: binario único sin runtime, cross-compile
trivial, buen soporte de concurrencia (streaming SSE, subprocesos, UI
concurrente) y velocidad de iteración alta mientras la API de extensiones está
en flujo. Candidatos evaluados: Go, Rust, Zig, C.

**Decisión.** Go, con `CGO_ENABLED=0`.

**Razonamiento.**
- Binario estático y cross-compile resuelven la distribución (la antítesis de
  npm).
- El trabajo real del harness (IO concurrente) es el punto fuerte de Go.
- Prior art directo: Crush (Charm) y la TUI original de OpenCode son Go.
- Rust (ratatui + mlua) fue el segundo candidato serio; se descarta por
  velocidad de iteración en fase de diseño, no por capacidad. Codex CLI
  (reescrito de TS a Rust) valida que ambos caminos funcionan.
- Zig/C descartados: meses de infraestructura que Go/Rust regalan.

**Consecuencias.** Renunciamos a LuaJIT embebido (requeriría cgo). El
rendimiento del scripting queda acotado por gopher-lua → refuerza ADR-004.

---

## ADR-002 · Lua (gopher-lua) como lenguaje de extensión

**Estado:** Aceptada · 2026-06

**Contexto.** La extensibilidad es el producto. Candidatos: Lua (gopher-lua o
LuaJIT/cgo), Starlark, Risor/Tengo, JS vía goja, WASM.

**Decisión.** Lua 5.1 embebido vía gopher-lua (Go puro).

**Razonamiento.**
- Lua está culturalmente probado como lenguaje de extensión (Neovim, wezterm,
  mpv, hammerspoon): la familiaridad del usuario es una feature.
- gopher-lua mantiene el binario estático sin cgo (coherente con ADR-001).
  LuaJIT daría rendimiento real pero rompe el cross-compile y el binario único.
- Starlark: paralelizable pero deliberadamente limitado (sin while ni
  recursión); incompatible con "Lua puede hacer cualquier cosa".
- goja (JS): mismo modelo monohilo, y reintroduce la cultura que evitamos.
- WASM: sandboxing y multi-lenguaje, pero DX de autoría muy inferior a 30
  líneas de Lua. Se reconsiderará solo si el sandboxing de terceros se vuelve
  requisito duro.

**Consecuencias.** Lua 5.1 (no 5.4). Rendimiento de intérprete: el trabajo
pesado debe vivir en primitivas Go (ADR-004). gopher-lua no es thread-safe →
condiciona todo el modelo de concurrencia.

---

## ADR-003 · Core mínimo: el agente y MCP son extensiones oficiales

**Estado:** Aceptada · 2026-06

**Contexto.** Dos modelos posibles: core-con-hooks (Neovim: el programa
principal en nativo, extensiones decoran) o kernel-runtime (Emacs/Textadept:
el programa entero escrito en el lenguaje de extensión sobre un kernel de
primitivas).

**Decisión.** Kernel-runtime. El core Go no contiene lógica de agente, MCP,
chat ni comandos: todo eso son extensiones Lua oficiales, embebidas en el
binario con `go:embed` pero sin ningún privilegio arquitectónico.

**Razonamiento.**
- "Lua puede hacer cualquier cosa" exige que las features oficiales sean
  construibles con la API pública; si no, la API está incompleta. Dogfooding
  estructural (como pi con sus propias features).
- El usuario radical no hace fork: sustituye extensiones.
- `go:embed` preserva la experiencia batteries-included.

**Consecuencias.** La superficie de primitivas del kernel crece (HTTP/SSE,
spawn con streams, UI completa): el core conceptualmente mínimo necesita una
stdlib grande. La estabilidad de la API core se vuelve crítica desde v1: los
breaking changes nos rompen a nosotros primero y al ecosistema después.

---

## ADR-004 · Modelo de concurrencia híbrido ("modelo del navegador")

**Estado:** Aceptada · 2026-06

**Contexto.** Un agente es inherentemente concurrente (stream de tokens, tool
calls paralelas, input de UI simultáneos). gopher-lua no es thread-safe. El
modelo Neovim (todo en un hilo) produce los cuelgues con trabajo pesado que
queremos evitar. Alternativas evaluadas: (1) estado único + event loop, (2)
actores puros con paso de mensajes por extensión, (3) extensiones como
subprocesos, (4) cambiar de runtime (Starlark/WASM).

**Decisión.** Híbrido de tres patas:
1. Estado Lua principal single-threaded con event loop y async por coroutines
   (patrón Node/libuv/`vim.uv`) para UI, hooks y orquestación.
2. Workers explícitos (`worker.spawn()`): estados Lua adicionales en
   goroutines propias, sin memoria compartida, paso de mensajes.
3. Primitivas Go paralelas por dentro para todo lo universalmente pesado
   (búsqueda, diff, parsing, highlighting, markdown).

Regla de oro: **Lua decide, Go ejecuta**.

**Razonamiento.**
- Un harness no es un editor: no mantiene buffers gigantes resaltados a cada
  tecla. Sus tareas pesadas son delegables a primitivas paralelas.
- El monohilo en el estado principal es una feature (determinismo, cero data
  races) para el 95% de los plugins; el 5% restante tiene workers opt-in.
- Subprocesos como modelo principal: latencia inaceptable para hooks de UI y
  reintroduce fricción de distribución (queda como Capa 2).
- Es el modelo ya validado por la plataforma web y por Luau (actores de
  Roblox).

**Consecuencias.** Hay que construir el equivalente a "luv para Go" (event
loop + puente de coroutines): el mayor coste de ingeniería inicial del core.
Markdown/highlighting entran al kernel como builtins por rendimiento, violando
conscientemente la pureza del kernel mínimo. Queda abierta la granularidad de
aislamiento (ADR-008).

---

## ADR-005 · Providers de LLM: registro en TOML + adaptadores en Lua

**Estado:** Aceptada · 2026-06

**Contexto.** Los providers difieren en protocolo (SSE, tool calls, system
prompts, thinking blocks): eso es código. Pero endpoints, claves, modelos y
límites son datos. ¿Dónde vive cada cosa?

**Decisión.** TOML declara el registro (datos); los adaptadores de protocolo
son extensiones Lua oficiales (código). El kernel solo aporta la primitiva
HTTP/SSE.

**Razonamiento.**
- Coherente con ADR-003: implementar protocolos en el core contradiría el
  kernel mínimo.
- Parsear SSE en Lua es viable: texto a velocidad de lectura humana.
- Añadir un provider raro (Ollama, vLLM, proxy corporativo) pasa a ser un
  fichero Lua, sin recompilar ni esperar release.
- La configuración del usuario común sigue siendo declarativa y simple (TOML).

**Consecuencias.** El cliente HTTP del kernel debe exponer streaming de
respuesta de primera clase desde v1.

---

## ADR-006 · TUI: librería del kernel

**Estado:** Propuesta · 2026-06

**Contexto.** Candidatos en Go: Bubble Tea + Lipgloss (+ glamour para
markdown) o tview. La elección está acoplada a ADR-007 (qué API de UI se
expone a Lua): el kernel podría incluso usar primitivas de terminal propias.

**Decisión (provisional).** Bubble Tea + Lipgloss como punto de partida, a
revisar cuando se cierre ADR-007.

**Consecuencias.** Ninguna irreversible mientras la API Lua de UI no exponga
conceptos de Bubble Tea directamente (no debería: la API pública es nuestra,
la librería es detalle de implementación).

---

## ADR-007 · API de UI expuesta a Lua

**Estado:** Abierta

**Contexto.** Si la UI de chat es una extensión (ADR-003), la API de UI debe
ser lo bastante rica para construirla entera desde Lua. Opciones: buffers y
ventanas estilo Neovim, árbol de widgets retenido, o superficie de celdas
inmediata. Cada una condiciona qué extensiones serán fáciles o imposibles.

**Decisión.** Pendiente.

---

## ADR-008 · Granularidad de aislamiento: workers por tarea vs actores por plugin

**Estado:** Abierta

**Contexto.** Con ADR-004 decidido, queda la pregunta fina: ¿el aislamiento es
opt-in por tarea (todas las extensiones comparten el estado principal y lanzan
workers efímeros cuando lo necesitan) o por plugin (cada extensión vive
permanentemente en su propio actor)? Afecta a: composabilidad entre plugins
(que se requieran unos a otros), contención de fallos, latencia de hooks
síncronos de UI y complejidad de la API.

**Decisión.** Pendiente.
