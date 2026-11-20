import { defineConfig } from "astro/config";
import icon from "astro-icon";
import tailwindcss from "@tailwindcss/vite";
import nimbus, { defineConfig as defineNimbusConfig } from "@cloudflare/nimbus-docs";
import { tableScroll } from "@cloudflare/nimbus-docs/markdown";

const nimbusConfig = defineNimbusConfig({
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
          "what-is-octarq",
          "quickstart",
          "configuration",
          "deploy",
          "backup-restore",
          "pre-launch",
        ],
      },
      {
        label: "Core Features",
        autogenerate: { directory: "core" },
      },
      {
        label: "Plugins",
        autogenerate: { directory: "plugins" },
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
        label: "Developers",
        items: [
          "developers/conventions",
        ],
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
  vite: {
    plugins: [tailwindcss()],
  },
  prefetch: {
    prefetchAll: true,
    defaultStrategy: "hover",
  },
  integrations: [
    icon(),
    nimbus(nimbusConfig, {
      rules: {
        "nimbus/frontmatter-shape": "error",
        "nimbus/internal-link": ["error", { ignore: ["/openapi.json"] }],
      },
      markdown: {
        hastPlugins: [tableScroll()],
      },
    }),
  ],
});
