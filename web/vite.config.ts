import { defineConfig } from "vite";
import { fileURLToPath, URL } from "node:url";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

// Read the backend port from OCTARQ_PORT env var (default: 8680 to match .env OCTARQ_LISTEN=:8680).
// Override at the shell: OCTARQ_PORT=9000 make dev
const backendPort = process.env.OCTARQ_PORT ?? "8680";

// Edition switch for build-time frontend plugin composition. `VITE_OCTARQ_PLUGINS=pro`
// selects the commercial injection module (which composes the Pro UI plugins in);
// anything else selects the empty OSS module, so no Pro page is bundled.
const pluginsEntry =
  process.env.VITE_OCTARQ_PLUGINS === "pro" ? "./src/plugins/index.pro.ts" : "./src/plugins/index.ts";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      // The frontend plugin SDK. Plugins import by this name so the folder can
      // later be extracted to a published `@octarq-org/plugin-sdk` package without any
      // import churn in plugin code. Keep in sync with tsconfig.json paths.
      "@octarq-org/plugin-sdk": fileURLToPath(new URL("./src/plugin-sdk", import.meta.url)),
      // The plugin injection module — the OSS-vs-commercial composition seam.
      "#octarq-plugins": fileURLToPath(new URL(pluginsEntry, import.meta.url)),
    },
  },
  // The dashboard SPA is mounted under /admin so short-link slugs own the root.
  base: "/admin/",
  build: {
    // OCTARQ_WEBEMBED_OUT lets a commercial build (octarq-pro) redirect the
    // output elsewhere while reusing this exact build; defaults to the core's
    // embedded dist.
    outDir: process.env.OCTARQ_WEBEMBED_OUT || "../webembed/dist",
    emptyOutDir: true,
  },
  server: {
    proxy: {
      "/api": `http://localhost:${backendPort}`,
    },
  },
});
