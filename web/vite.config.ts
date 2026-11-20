import { defineConfig, searchForWorkspaceRoot } from "vite";
import { fileURLToPath, URL } from "node:url";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { octarqPlugins } from "./plugins-manifest";

const backendPort = process.env.OCTARQ_PORT ?? "8680";

const devRoots = (process.env.OCTARQ_DEV_ROOTS ?? "").split(":").map((p) => p.trim()).filter(Boolean);

let devEditionAliases: Record<string, string> = {};
try {
  devEditionAliases = JSON.parse(process.env.OCTARQ_DEV_ALIASES ?? "{}");
} catch (e) {
  throw new Error(`OCTARQ_DEV_ALIASES is not valid JSON: ${(e as Error).message}`);
}

const here = (p: string) => fileURLToPath(new URL(p, import.meta.url));
const devSharedAliases = devRoots.length
  ? {
      react: here("./node_modules/react"),
      "react-dom": here("./node_modules/react-dom"),
      "react/jsx-runtime": here("./node_modules/react/jsx-runtime"),
      "lucide-react": here("./node_modules/lucide-react"),
      "@xterm/xterm": here("./node_modules/@xterm/xterm"),
      "@xterm/addon-fit": here("./node_modules/@xterm/addon-fit"),
    }
  : {};

export default defineConfig({
  plugins: [
    react(),
    tailwindcss(),
    octarqPlugins(),
    {
      name: "spa-basename-html",
      configureServer(server) {
        server.middlewares.use((req, _res, next) => {
          const url = req.url ?? "";
          if (url.startsWith("/instance") || url === "/admin" || url === "/status") {
            req.url = "/admin/";
          }
          next();
        });
      },
    },
  ],
  resolve: {
    dedupe: ["react", "react-dom"],
    alias: {
      "@octarq/plugin-sdk": fileURLToPath(new URL("./src/plugin-sdk", import.meta.url)),
      "@octarq-org/plugin-sdk": fileURLToPath(new URL("./src/plugin-sdk", import.meta.url)),
      ...devSharedAliases,
      ...devEditionAliases,
    },
  },
  base: "/admin/",
  build: {
    outDir: process.env.OCTARQ_WEBEMBED_OUT || "../server/webembed/dist",
    emptyOutDir: true,
    rollupOptions: {
      output: {
        manualChunks: {
          'vendor-react': ['react', 'react-dom', 'react-router-dom'],
          'vendor-motion': ['framer-motion'],
          'vendor-icons': ['lucide-react'],
          'vendor-ui': ['@base-ui/react']
        }
      }
    }
  },
  server: {
    fs: { allow: [searchForWorkspaceRoot(process.cwd()), ...devRoots, ...Object.values(devEditionAliases)] },
    proxy: {
      "/api": {
        target: `http://localhost:${backendPort}`,

        changeOrigin: false,
      },
    },
  },
});
