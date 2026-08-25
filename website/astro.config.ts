import { defineConfig } from "astro/config";
import icon from "astro-icon";
import tailwindcss from "@tailwindcss/vite";
import nimbus, { defineConfig as defineNimbusConfig } from "@cloudflare/nimbus-docs";
import { tableScroll } from "@cloudflare/nimbus-docs/markdown";

const nimbusConfig = defineNimbusConfig({
  // One site, not two. This project already serves both halves — the landing
  // page at "/" (src/pages/index.astro) and the docs under it — so splitting
  // the marketing copy onto its own host would have meant a second deploy of
  // the same build. Canonical URLs, sitemap, and OG cards all derive from
  // this, so it has to be the host the pages are actually served on.
  site: "https://docs.octarq.org",
  title: "Octarq",
  description: "AI-native team backoffice & plugin framework",
  locale: "en",
  github: "https://github.com/octarq-org/octarq",
  socialImageAlt: "Octarq documentation preview",
  sidebar: {
    items: [
      {
        label: "Start",
        items: [
          // No "index" entry: Nimbus maps it to "/", which src/pages/index.astro
          // owns, so the sidebar link led to the landing page instead of a doc —
          // a "Start → Octarq" item that took you back to the front door. The
          // page's content now lives in the landing page itself.
          "what-is-octarq",
          "quickstart",
          "configuration",
          "deploy",
          // Both carry only frontmatter sidebar blocks today, which nothing
          // reads — without an entry here they were unreachable from the
          // sidebar (backup-restore since #323).
          "backup-restore",
          "pre-launch",
        ],
      },
      {
        label: "Core Features",
        autogenerate: { directory: "core" },
      },
      {
        label: "Build a Plugin",
        items: [
          "writing-a-plugin",
          "plugin-directory",
        ],
      },
      {
        label: "Architecture",
        autogenerate: { directory: "architecture" },
      },
      {
        label: "Guides",
        autogenerate: { directory: "guides" },
      },
    ],
  },
});

export default defineConfig({
  output: "static",
  // Tailwind v4 via its Vite plugin (the integration Astro recommends for
  // Tailwind v4 — replaces the PostCSS plugin, which doesn't build under
  // Astro 7's Vite 8 bundler).
  vite: {
    plugins: [tailwindcss()],
  },
  // Hover-prefetch link targets so full-page navigations feel instant without
  // a client-side router.
  prefetch: {
    prefetchAll: true,
    defaultStrategy: "hover",
  },
  integrations: [
    icon(),
    nimbus(nimbusConfig, {
      // Authoring rules are opt-in by design — your repo, your taste. The
      // two below are the load-bearing pair: frontmatter has to validate
      // against the content schema for the page to render properly, and
      // broken internal links are 404s for your readers. Add the others
      // (heading hierarchy, code-block language, style, etc.) when you're
      // ready to enforce them — see `nimbus-docs lint --help`.
      rules: {
        "nimbus/frontmatter-shape": "error",
        // Route truth comes from Astro's emitted pages, so anything served
        // straight out of public/ reads as a broken link. The API reference
        // links the spec file that lives there; ignore that one path rather
        // than downgrade the rule, which would stop catching real 404s.
        "nimbus/internal-link": ["error", { ignore: ["/openapi.json"] }],
      },
      // Wrap wide tables so they scroll instead of overflowing the page
      // (styled by `.nb-table-scroll` in src/styles/prose.css).
      markdown: {
        hastPlugins: [tableScroll()],
      },
    }),
  ],
});
