import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { resolve } from "path";

const backendPort = process.env.OCTARQ_PORT ?? "8680";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  base: "/portal/",
  build: {
    outDir: (process.env.OCTARQ_WEBEMBED_OUT || "../webembed/dist") + "/portal",
    emptyOutDir: true,
    rollupOptions: {
      input: {
        main: resolve(__dirname, "portal.html"),
      },
      output: {
        manualChunks: {
          'vendor-react': ['react', 'react-dom'],
          'vendor-motion': ['framer-motion'],
          'vendor-icons': ['lucide-react']
        }
      }
    },
  },
  server: {
    proxy: {
      "/api": `http://localhost:${backendPort}`,
    },
  },
});
