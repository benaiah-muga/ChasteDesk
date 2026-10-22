// SPDX-License-Identifier: AGPL-3.0-only
import { fileURLToPath, URL } from "node:url";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

export default defineConfig({
  base: "/static/console/",
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: { "@": fileURLToPath(new URL("./src", import.meta.url)) },
  },
  server: {
    port: 5174,
    proxy: {
      "/api": "http://127.0.0.1:3100",
      "/static/css": "http://127.0.0.1:3100",
      "/static/js": "http://127.0.0.1:3100",
      "/static/fonts": "http://127.0.0.1:3100",
      "/static/mark.svg": "http://127.0.0.1:3100",
      "/static/favicon-32.png": "http://127.0.0.1:3100",
      "/static/favicon-192.png": "http://127.0.0.1:3100",
      "/static/apple-touch-icon.png": "http://127.0.0.1:3100",
    },
  },
  build: {
    outDir: fileURLToPath(new URL("../internal/web/static/console", import.meta.url)),
    emptyOutDir: true,
    sourcemap: false,
    cssCodeSplit: false,
    chunkSizeWarningLimit: 650,
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (id.includes("node_modules")) return "vendor";
          return undefined;
        },
      },
    },
  },
});
