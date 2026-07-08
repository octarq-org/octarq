import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { resolve } from "path";

const backendPort = process.env.OCTARQ_PORT ?? "8680";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  base: "/portal/",
  build: {
    outDir: "../webembed/dist/portal",
    emptyOutDir: true,
    rollupOptions: {
      input: {
        main: resolve(__dirname, "portal.html"),
      },
    },
  },
  server: {
    proxy: {
      "/api": `http://localhost:${backendPort}`,
    },
  },
});
