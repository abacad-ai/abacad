import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

// Dev: Go runs on :1213; proxy the API, MCP, and device WS to it so the SPA and
// backend share an origin (cookies, no CORS). Prod: Go serves the built dist.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: { alias: { "@": new URL("./src", import.meta.url).pathname } },
  server: {
    host: true, // listen on 0.0.0.0 so the dev server is reachable via the LAN IP
    port: 1419,
    proxy: {
      // changeOrigin stays off: the backend derives absolute URLs (OAuth
      // redirect_uri, device WS) from the Host header, which must remain the
      // origin the browser actually used, not the proxy target.
      "/api": { target: "http://localhost:1213", changeOrigin: false },
      "/mcp": { target: "http://localhost:1213", changeOrigin: false },
      // Regex, not a prefix: a bare "/device" key also matches the "/devices"
      // SPA routes and would proxy those page loads to Go instead of serving
      // the dev index.html. Only the exact WS path (plus its ?token=) proxies.
      "^/device(\\?|$)": { target: "ws://localhost:1213", ws: true },
      // Release artifacts live on Go's disk. Regex again: only /downloads/<file>
      // proxies, so the bare /downloads SPA page still renders from dev.
      "^/downloads/.": { target: "http://localhost:1213", changeOrigin: false },
      // noVNC live-view WebSocket: the browser connects to /vnc/watch; proxy it
      // to Go so the dev SPA and backend share an origin (session cookie).
      "^/vnc/watch": { target: "ws://localhost:1213", ws: true },
    },
  },
  // es2022 so noVNC's top-level await (in its RFB module) transpiles; the
  // dashboard targets current evergreen browsers.
  build: { outDir: "dist", emptyOutDir: true, target: "es2022" },
});
