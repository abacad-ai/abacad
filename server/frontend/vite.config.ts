import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import path from "node:path";

// Dev: Go runs on :8848; proxy the API, MCP, and device WS to it so the SPA and
// backend share an origin (cookies, no CORS). Prod: Go serves the built dist.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: { alias: { "@": path.resolve(__dirname, "src") } },
  server: {
    port: 5173,
    proxy: {
      "/api": "http://localhost:8848",
      "/mcp": "http://localhost:8848",
      "/device": { target: "ws://localhost:8848", ws: true },
    },
  },
  build: { outDir: "dist", emptyOutDir: true },
});
