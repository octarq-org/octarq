import { defineConfig, searchForWorkspaceRoot } from "vite";
import { fileURLToPath, URL } from "node:url";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { octarqPlugins } from "./plugins-manifest";

// Read the backend port from OCTARQ_PORT env var (default: 8680 to match .env OCTARQ_LISTEN=:8680).
// Override at the shell: OCTARQ_PORT=9000 make dev
const backendPort = process.env.OCTARQ_PORT ?? "8680";

// Dev-only: an external edition (e.g. octarq-pro) can point the manifest at its
// plugins' LOCAL SOURCE for HMR instead of the published package (see a
// `{ "from": "../../octarq-pro/packages/plugin-x/src" }` manifest entry). Those
// files live outside this repo, so Vite's dev server must be allowed to read
// them: OCTARQ_DEV_ROOTS is a colon-separated list of absolute dirs to permit.
// The plugin source's own deps (api-client, xterm, …) resolve from that edition's
// node_modules; React and the SDK are deduped/aliased below so the out-of-root
// source shares the app's single instance (one React, one plugin registry).
const devRoots = (process.env.OCTARQ_DEV_ROOTS ?? "").split(":").map((p) => p.trim()).filter(Boolean);

// When composing plugins from external source (dev-from-source), force the UI
// libraries they share with the app to THIS repo's single copy — otherwise the
// out-of-root source resolves its own react/lucide from the edition's
// node_modules and you get duplicate-React hook crashes / mismatched context.
// Edition-specific plugin deps (@octarq-org/api-client, @xterm/*) still resolve
// from the edition. No-op for a normal build (devRoots empty).
const here = (p: string) => fileURLToPath(new URL(p, import.meta.url));
const devSharedAliases = devRoots.length
  ? {
      react: here("./node_modules/react"),
      "react-dom": here("./node_modules/react-dom"),
      "react/jsx-runtime": here("./node_modules/react/jsx-runtime"),
      "lucide-react": here("./node_modules/lucide-react"),
    }
  : {};

export default defineConfig({
  // octarqPlugins() serves the `#octarq-plugins` virtual module, composing the
  // UI plugins named in the active manifest (see plugins-manifest.ts). WHICH
  // plugins ship is chosen by manifest, not a code switch.
  plugins: [react(), tailwindcss(), octarqPlugins()],
  resolve: {
    // React must be a singleton so plugin source composed from OUTSIDE this repo
    // (dev-from-source) shares the app's React — otherwise hooks throw.
    dedupe: ["react", "react-dom"],
    alias: {
      // The frontend plugin SDK. The app (and plugins) import by this name; the
      // alias points at the app-side facade that re-exports the published
      // `@octarq-org/plugin-sdk`, giving the whole build one SDK instance (one
      // registry, one i18n/brand context). Keep in sync with tsconfig.json paths.
      "@octarq-org/plugin-sdk": fileURLToPath(new URL("./src/plugin-sdk", import.meta.url)),
      ...devSharedAliases,
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
    // Permit reading plugin source from external edition roots (dev-from-source).
    // Empty OCTARQ_DEV_ROOTS → just the workspace root, i.e. default behavior.
    fs: { allow: [searchForWorkspaceRoot(process.cwd()), ...devRoots] },
    proxy: {
      "/api": {
        target: `http://localhost:${backendPort}`,

        // 🔑 必须为 false（不设默认为 false，绝对不要写成 true）
        changeOrigin: false,
      },
    },
  },
});
