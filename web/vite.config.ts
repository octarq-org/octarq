import { defineConfig } from "vite";
import { fileURLToPath, URL } from "node:url";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

// Read the backend port from LED_PORT env var (default: 8680 to match .env LED_LISTEN=:8680).
// Override at the shell: LED_PORT=9000 make dev
const backendPort = process.env.LED_PORT ?? "8680";

// Edition switch for build-time frontend plugin composition. `VITE_LED_PLUGINS=pro`
// selects the commercial injection module (which composes the Pro UI plugins in);
// anything else selects the empty OSS module, so no Pro page is bundled.
const pluginsEntry =
  process.env.VITE_LED_PLUGINS === "pro" ? "./src/plugins/index.pro.ts" : "./src/plugins/index.ts";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      // The frontend plugin SDK. Plugins import by this name so the folder can
      // later be extracted to a published `@led/plugin-sdk` package without any
      // import churn in plugin code. Keep in sync with tsconfig.json paths.
      "@led/plugin-sdk": fileURLToPath(new URL("./src/plugin-sdk", import.meta.url)),
      // The plugin injection module — the OSS-vs-commercial composition seam.
      "#led-plugins": fileURLToPath(new URL(pluginsEntry, import.meta.url)),
    },
  },
  // The dashboard SPA is mounted under /admin so short-link slugs own the root.
  base: "/admin/",
  build: {
    outDir: "../webembed/dist",
    emptyOutDir: true,
  },
  server: {
    proxy: {
      "/api": `http://localhost:${backendPort}`,
    },
  },
});
