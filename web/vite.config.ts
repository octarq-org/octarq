import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

// Read the backend port from LED_PORT env var (default: 8680 to match .env LED_LISTEN=:8680).
// Override at the shell: LED_PORT=9000 make dev
const backendPort = process.env.LED_PORT ?? "8680";

export default defineConfig({
  plugins: [react(), tailwindcss()],
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
