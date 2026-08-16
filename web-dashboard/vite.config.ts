import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import path from "node:path";
import { defineConfig } from "vite";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      "@": path.resolve(import.meta.dirname, "client", "src"),
    },
  },
  envDir: path.resolve(import.meta.dirname),
  root: path.resolve(import.meta.dirname, "client"),
  build: {
    outDir: path.resolve(import.meta.dirname, "dist/public"),
    emptyOutDir: true,
  },
  server: {
    port: 10002,
    strictPort: true,
    host: true,
    // Local engine API (ainovel-cli web) — proxy sang Go backend khi dev.
    proxy: {
      "/api": {
        target: process.env.VITE_ENGINE_PROXY_TARGET || "http://127.0.0.1:10001",
        changeOrigin: true,
        ws: true,
      },
    },
  },
});
