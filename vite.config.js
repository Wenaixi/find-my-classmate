import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
export default defineConfig({
    plugins: [react()],
    build: { outDir: "server/web", emptyOutDir: true },
    server: {
        port: 5173,
        proxy: { "/api": "http://localhost:3078" }
    },
    preview: { port: 4173 }
});
