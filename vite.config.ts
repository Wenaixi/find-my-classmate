import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  build: {
    outDir: "server/web",
    emptyOutDir: true,
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (id.indexOf("node_modules/react") !== -1 || id.indexOf("node_modules/scheduler") !== -1) return "vendor-react";
          if (id.indexOf("node_modules/border-beam") !== -1) return "vendor-beam";
          return undefined;
        }
      }
    }
  },
  server: {
    port: 5173,
    proxy: { "/api": "http://localhost:3078" }
  },
  preview: { port: 4173 }
});
