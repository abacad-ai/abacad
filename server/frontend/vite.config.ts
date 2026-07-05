import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import path from "node:path";

// Dev: Go runs on :1213; proxy the API, MCP, and device WS to it so the SPA and
// backend share an origin (cookies, no CORS). Prod: Go serves the built dist.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: { alias: { "@": path.resolve(__dirname, "src") } },
  server: {
    host: true, // listen on 0.0.0.0 so the dev server is reachable via the LAN IP
    port: 1419,
    proxy: {
      "/api": "http://localhost:1213",
      "/mcp": "http://localhost:1213",
      "/device": { target: "ws://localhost:1213", ws: true },
    },
  },
  build: { outDir: "dist", emptyOutDir: true },
});
