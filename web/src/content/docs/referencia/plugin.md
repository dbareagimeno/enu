---
title: nu.plugin — plugins y loader
description: El sistema de plugins de nu — estructura, loader, identidad por nombre, orden de arranque, reload y directorios de configuración.
---

`nu.plugin` y el loader son cómo `nu` carga el código que lo convierte en algo.
Recuerda: **todo** —el agente, el chat, los providers— es un plugin; las
extensiones oficiales no tienen privilegio. Solo estado principal (salvo
`nu.config.dir`/`data_dir`, que son **[W]**).

## Qué es un plugin

Un directorio con dos ficheros:

- `plugin.toml` — metadatos: `name`, `version`, `requires?: string[]`.
- `init.lua` — se ejecuta al cargar.

El subdirectorio `lua/` del plugin se añade a las rutas de `require`, así los
plugins se requieren entre sí (composabilidad).

```toml
# plugin.toml
name = "mi-plugin"
version = "0.1.0"
requires = ["agent"]   # se carga después de 'agent'
```

```lua
-- init.lua
local agent = require("agent")
-- registra tools, comandos, keymaps... usando solo la API pública
```

## Identidad por nombre

El **nombre es la identidad** del plugin, y el loader la mantiene **única**: el
directorio de usuario *sustituye* a la extensión embebida del mismo nombre (no
coexisten), y dos plugins con el mismo nombre son un error de carga accionable.
Esa unicidad es lo que deja que los namespaces de eventos y demás registros sean
libres de colisión por simple convención (namespace = nombre del plugin), sin
que el core reserve nombre alguno.

## Orden de arranque canónico

```
core → plugins activados (topológico por requires) → init.lua del usuario → core:ready
```

El `init.lua` del **usuario va el último** a propósito: como en la pila de input
el registro más reciente gana, el usuario tiene la última palabra (keymaps,
theme, overrides) por construcción, sin sistema de prioridades.

Las extensiones oficiales embebidas (`go:embed`) se cargan primero pero solo si
`plugins.enabled` (en `nu.toml`) las nombra —**inactivas por defecto**, ADR-010—.

## API

### `nu.plugin.current`

```
nu.plugin.current() -> { name, version, dir }
```

El plugin en cuyo contexto corre el código. El core lo usa para etiquetar los
handles por dueño (lo que hace posible el `reload`).

### `nu.plugin.list`

```
nu.plugin.list() -> { name, version, source: "builtin"|"user", enabled }[]
```

```lua
for _, p in ipairs(nu.plugin.list()) do
  nu.log.info("%s %s (%s) %s", p.name, p.version, p.source,
    p.enabled and "activo" or "inactivo")
end
```

### `nu.plugin.reload` ⏸

```
nu.plugin.reload(name)
```

Herramienta de **desarrollo**, *best-effort*: suelta los handles del plugin,
emite `core:plugin.unload` (las extensiones limpian sus registros), vacía la
caché de `require` del plugin y recarga su `init.lua`. Un plugin con efectos
globales exóticos puede no descargarse limpio: es para **iterar, no para
producción**.

## Directorios

```
nu.config.dir() -> string       [W]   -- ~/.config/nu (o equivalente)
nu.config.data_dir() -> string  [W]   -- ~/.local/share/nu (o equivalente)
```

`config.dir()` es donde viven `nu.toml`, `providers.toml` y la config de los
plugins; `data_dir()` es para datos (logs, sesiones).

```sh
nu -e 'return nu.config.dir() ~= nil, nu.config.data_dir() ~= nil'
```

```
true
true
```

:::note[Configuración del runtime]
`config.dir()/nu.toml` gobierna al propio core: qué plugins se activan, rutas
extra de plugins y el presupuesto del watchdog. Un `nu.toml` roto o un
`plugins.enabled` que nombra algo inexistente es un error de arranque accionable
que apunta a la línea a corregir.
:::
