# Providers de LLM: registro TOML y contrato del adaptador

Estado: **borrador para discusión**. Este documento define el contrato de la
**extensión oficial de providers** — no es API sagrada del core
([api.md](api.md)); se versiona aparte y puede evolucionar más rápido.
Materializa la ADR-005: *TOML declara los datos, Lua implementa el protocolo*.

Dos audiencias:

1. **Usuario que añade un modelo** (el caso `models.json` de pi): edita
   `providers.toml`. Cero código.
2. **Autor de un adaptador** (protocolo nuevo o dialecto raro): escribe un
   módulo Lua que cumple el contrato de §3.

---

## 1. El registro: `providers.toml`

Vive en `nu.config.dir()`. Declara *datos*, nunca lógica.

```toml
# Provider con adaptador oficial: solo datos.
[providers.anthropic]
adapter     = "anthropic"                  # qué adaptador habla su protocolo
base_url    = "https://api.anthropic.com"
api_key_env = "ANTHROPIC_API_KEY"          # nunca la clave en el fichero

[[providers.anthropic.models]]
id         = "claude-opus-4-8"
context    = 200000
max_output = 32000
cost       = { input = 5.0, output = 25.0 }   # USD por Mtok (informativo)
aliases    = ["opus"]

# El caso models.json: endpoint compatible-OpenAI, p. ej. Ollama local.
[providers.local]
adapter  = "openai-compat"
base_url = "http://localhost:11434/v1"

[[providers.local.models]]
id      = "qwen3:32b"
context = 32768

# Provider con protocolo exótico: el adaptador es de un plugin de terceros.
[providers.corp]
adapter  = "mi-plugin/corp-gateway"        # resoluble por require()
base_url = "https://llm.internal.corp"
extra    = { tenant = "equipo-7" }         # tabla opaca, pasa al adaptador
```

Resolución de un modelo: `"proveedor/id-o-alias"` (`"anthropic/opus"`,
`"local/qwen3:32b"`). La extensión de providers resuelve el TOML, lee la API
key del entorno y entrega al adaptador una `ProviderConfig` ya cocinada.

---

## 2. El modelo canónico

El agente habla siempre esta representación; el adaptador traduce de/hacia el
dialecto del provider. Es deliberadamente un superconjunto pequeño de lo que
hoy ofrecen Anthropic/OpenAI/Gemini.

### 2.1 Request

```
Request = {
  model:       string,            -- id tal y como lo espera el provider
  system?:     string,
  messages:    Message[],
  tools?:      ToolDef[],         -- { name, description, schema (JSON Schema, tabla) }
  max_tokens?: integer,
  temperature?: number,
  thinking?:   { budget?: integer },
}

Message = { role: "user"|"assistant", content: Block[] }
```

### 2.2 Bloques de contenido

```
Block =
  | { type = "text",        text }
  | { type = "image",       media_type, data_base64 }
  | { type = "thinking",    text }
  | { type = "tool_call",   id, name, args }            -- args: tabla
  | { type = "tool_result", id, content: Block[], is_error? }
```

**Regla `meta`**: cualquier bloque puede llevar `meta?: tabla` — un campo
**opaco propiedad del adaptador**. El agente lo preserva intacto y lo devuelve
en turnos siguientes sin mirarlo. Es la válvula para los caprichos de cada
protocolo (firmas de thinking de Anthropic, `cache_control`, ids internos...)
sin contaminar el modelo canónico.

### 2.3 Eventos de streaming (lo que el adaptador emite)

```
Event =
  | { type = "text",            text }                  -- delta de texto
  | { type = "thinking",        text }                  -- delta de razonamiento
  | { type = "tool_call.begin", id, name }
  | { type = "tool_call.delta", id, args_json }         -- fragmento del JSON de args
  | { type = "tool_call.end",   id }
  | { type = "usage",           input_tokens?, output_tokens?, cache_read_tokens? }
  | { type = "done",            stop_reason: "end"|"tool_calls"|"max_tokens"|"refusal",
                                message: Message }      -- el mensaje completo ensamblado
```

`done` cierra siempre el stream e incluye el `Message` canónico completo (con
sus `meta`), listo para anexar a la conversación. Así el agente no tiene que
re-ensamblar deltas, y los deltas quedan solo para pintar en vivo.

---

## 3. El contrato del adaptador

Un adaptador es un módulo Lua que devuelve:

```
{
  name: string,
  caps: { tools?: boolean, images?: boolean, thinking?: boolean,
          system?: boolean, usage?: boolean },
  stream: function(req: Request, provider: ProviderConfig) -> iterator<Event>,  ⏸
}
```

donde `ProviderConfig = { base_url, api_key?, extra?, model: ModelInfo }` ya
resuelta desde el TOML.

Obligaciones del adaptador:

1. **`stream` es una función suspendiente** que devuelve un iterador de
   `Event`s (típicamente envolviendo `nu.http.stream` + `Stream:events()`).
   Se ejecuta dentro de la task del agente: la cancelación de esa task
   cancela la petición (el runtime cierra el `Stream` subyacente).
2. **Errores**: lanza errores estructurados (ADR-009) con código
   `EPROVIDER` y `detail = { status?, provider_code?, retryable: boolean }`.
   Marcar `retryable` correctamente (429, 5xx, cortes de red) es la única
   inteligencia de fallos que se le pide.
3. **Sin política**: el adaptador no reintenta, no hace backoff, no trunca
   contexto, no decide nada. Eso es del loop del agente (que sí ve
   `retryable`). Un adaptador es un traductor puro.
4. **Round-trip fiel**: lo que llegue en `meta` de bloques previos debe
   reinyectarse en el wire format como el provider lo exige.
5. **Degradación declarada**: si `caps.tools = false` y el request trae
   tools, lanza `EINVAL` — no simula silenciosamente.

Esqueleto ilustrativo (no normativo):

```lua
-- adapters/openai_compat.lua
return {
  name = "openai-compat",
  caps = { tools = true, images = true, system = true, usage = true },
  stream = function(req, provider)
    local body = to_wire(req)                       -- canónico → dialecto
    local s = nu.http.stream{
      url = provider.base_url .. "/chat/completions",
      method = "POST",
      headers = auth_headers(provider),
      body = nu.json.encode(body),
    }
    if s.status >= 400 then
      error({ code = "EPROVIDER", message = read_error(s),
              detail = { status = s.status, retryable = s.status == 429 or s.status >= 500 } })
    end
    return events_from(s)                           -- SSE del dialecto → Event[]
  end,
}
```

---

## 4. Registro y descubrimiento

- Los adaptadores oficiales (`anthropic`, `openai-compat`, `gemini`) van
  embebidos como parte de la extensión de providers.
- Un plugin aporta el suyo registrándolo:
  `providers.register_adapter("corp-gateway", adapter)` — o por convención de
  nombre resoluble con `require` desde el TOML (`"mi-plugin/corp-gateway"`).
- API de consumo para el agente (y cualquier extensión):
  `providers.resolve("anthropic/opus") -> { adapter, config }` y
  `providers.list() -> ModelInfo[]` (alimenta el selector de modelos de la UI).

---

## 5. Cuestiones abiertas

1. **Prompt caching**: ¿se expresa en el modelo canónico (marcas de
   breakpoint) o queda enteramente en `meta`/heurística del adaptador?
2. **Embeddings y otros endpoints no-chat**: fuera de este contrato v1;
   ¿segundo contrato o extensión de este?
3. **Imágenes/archivos de salida** del modelo: el vocabulario de `Event` no
   los contempla aún.
4. **Token counting** previo (para compactación de contexto): ¿obligación
   opcional del adaptador (`count_tokens?`) o estimación genérica del agente?
