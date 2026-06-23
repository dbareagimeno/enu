# Web de documentación de `nu`

Manual de uso de `nu`: instalación, primeros pasos y la referencia función a
función de la API del core. Construido con [Astro](https://astro.build/) +
[Starlight](https://starlight.astro.build/).

> La **fuente de verdad** de la API es [`docs/api.md`](../docs/api.md) (la
> "superficie sagrada" v1). Este sitio la presenta de forma orientada a tareas y
> con ejemplos. Si algo discrepa, manda `docs/api.md`.

## Desarrollo

```sh
cd web
npm install
npm run dev      # servidor de desarrollo en http://localhost:4321/nu
npm run build    # genera el sitio estático en dist/
npm run preview  # sirve el build
```

## Estructura

```
web/
├── astro.config.mjs          # config de Starlight (sidebar, locale es, base /nu)
├── src/content/docs/
│   ├── index.mdx             # portada
│   ├── empezando/            # instalación y primeros pasos
│   └── referencia/           # una página por namespace de nu.*
└── public/                   # estáticos
```

## Ejemplos verificados

Los ejemplos `nu -e '...'` de la referencia están comprobados contra el binario
real (`go build -o nu . && nu -e '...'`). Recuerda que el chunk de `nu -e` corre
en el estado principal: las funciones suspendientes (⏸) van envueltas en
`nu.task.spawn(...)`.

## Despliegue

`.github/workflows/docs.yml` construye y publica el sitio en GitHub Pages al
hacer push a `main` cuando cambia algo bajo `web/`. El `base` del sitio es `/nu`
(project page); para un dominio propio, vacía `base` en `astro.config.mjs`.
