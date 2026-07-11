import { defineConfig } from "vite";
import { fileURLToPath, URL } from "node:url";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { octarqPlugins } from "./plugins-manifest";

// Read the backend port from OCTARQ_PORT env var (default: 8680 to match .env OCTARQ_LISTEN=:8680).
// Override at the shell: OCTARQ_PORT=9000 make dev
const backendPort = process.env.OCTARQ_PORT ?? "8680";

export default defineConfig({
  // octarqPlugins() serves the `#octarq-plugins` virtual module, composing the
  // UI plugins named in the active manifest (see plugins-manifest.ts). WHICH
  // plugins ship is chosen by manifest, not a code switch.
  plugins: [react(), tailwindcss(), octarqPlugins()],
  resolve: {
    alias: {
      // The frontend plugin SDK. The app (and plugins) import by this name; the
      // alias points at the app-side facade that re-exports the published
      // `@octarq-org/plugin-sdk`, giving the whole build one SDK instance (one
      // registry, one i18n/brand context). Keep in sync with tsconfig.json paths.
      "@octarq-org/plugin-sdk": fileURLToPath(new URL("./src/plugin-sdk", import.meta.url)),
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
      "/api": {
        target: `http://localhost:${backendPort}`,

        // 🔑 必须为 false（不设默认为 false，绝对不要写成 true）
        changeOrigin: false,
      },
    },
  },
});
