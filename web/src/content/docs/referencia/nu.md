---
title: nu — raíz
description: Versión del runtime, nivel de API y detección de capacidades con nu.has.
---

El namespace raíz expone la versión del runtime y la detección de capacidades.
Es lo primero que toca cualquier plugin que quiera ser portable.

## `nu.version` [W]

```
nu.version -> { major, minor, patch, api: integer }
```

Versión del runtime y **nivel de API** del core. `api` es el número que crece con
cada adición a la superficie sagrada; úsalo para exigir un mínimo, pero prefiere
[`nu.has`](#nuhas) para detectar capacidades concretas.

```sh
nu -e 'return nu.json.encode(nu.version)'
```

```
{"api":2,"major":0,"minor":1,"patch":0}
```

```lua
-- Exigir un nivel mínimo de API.
assert(nu.version.api >= 2, "este plugin necesita api >= 2")
```

## `nu.has` [W]

```
nu.has(cap: string) -> boolean
```

Detección de capacidades para extensiones portables. Devuelve si una capacidad
está disponible en este runtime/entorno. Cubre tanto rasgos finos
(`"ui.images"`, `"net.tcp"`) como **módulos enteros**: en headless `nu.ui` no
existe, y `nu.has("ui")` es la forma correcta de saberlo —nunca probar y
capturar el error—.

```sh
nu -e 'return nu.has("ui")'
```

```
false
```

(En `nu -e` no hay TTY, así que `nu.ui` no existe y `nu.has("ui")` es `false`.)

```lua
-- Degradar con elegancia según el entorno.
if nu.has("ui") then
  -- pintar una región
else
  -- modo headless: solo texto a stdout/log
end
```

:::tip[Capacidades, no versiones]
`nu.has` es el mecanismo de detección recomendado por encima de comparar
`nu.version.api`. Una capacidad puede estar ausente por el *entorno* (headless,
terminal sin soporte de imágenes), no solo por el nivel de API.
:::
