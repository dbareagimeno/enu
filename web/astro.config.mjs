// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

// El sitio se publica bajo /nu/ en GitHub Pages (project page). Si se sirve en
// un dominio propio, basta con vaciar `base` y ajustar `site`.
export default defineConfig({
  site: 'https://dbareagimeno.github.io',
  base: '/nu',
  integrations: [
    starlight({
      title: 'nu',
      description:
        'Manual de nu: un runtime de Lua orientado a terminal cuya killer app es un coding harness.',
      defaultLocale: 'root',
      locales: {
        root: { label: 'Español', lang: 'es' },
      },
      social: [
        {
          icon: 'github',
          label: 'GitHub',
          href: 'https://github.com/dbareagimeno/nu',
        },
      ],
      editLink: {
        baseUrl: 'https://github.com/dbareagimeno/nu/edit/main/web/',
      },
      sidebar: [
        {
          label: 'Empezando',
          items: [
            { label: 'Qué es nu', slug: 'empezando/que-es-nu' },
            { label: 'Instalación', slug: 'empezando/instalacion' },
            { label: 'Tu primer script', slug: 'empezando/primer-script' },
            { label: 'Tu primer agente', slug: 'empezando/primer-agente' },
            { label: 'Conceptos clave', slug: 'empezando/conceptos' },
          ],
        },
        {
          label: 'Referencia de la API',
          items: [
            { label: 'Convenciones', slug: 'referencia/convenciones' },
            { label: 'nu — raíz', slug: 'referencia/nu' },
            { label: 'nu.task — concurrencia', slug: 'referencia/task' },
            { label: 'nu.events — eventos', slug: 'referencia/events' },
            { label: 'nu.fs — filesystem', slug: 'referencia/fs' },
            { label: 'nu.proc — subprocesos', slug: 'referencia/proc' },
            { label: 'nu.sys — entorno y reloj', slug: 'referencia/sys' },
            { label: 'nu.http / nu.ws — red', slug: 'referencia/red' },
            { label: 'nu.ui — terminal', slug: 'referencia/ui' },
            { label: 'nu.text / nu.re — texto', slug: 'referencia/text' },
            { label: 'nu.search — búsqueda', slug: 'referencia/search' },
            { label: 'nu.json / toml / yaml — codecs', slug: 'referencia/codecs' },
            { label: 'nu.worker — paralelismo', slug: 'referencia/worker' },
            { label: 'nu.plugin — plugins y loader', slug: 'referencia/plugin' },
            { label: 'nu.log — logging', slug: 'referencia/log' },
            { label: 'La CLI', slug: 'referencia/cli' },
          ],
        },
      ],
    }),
  ],
});
