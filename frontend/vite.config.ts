import { mkdirSync, writeFileSync } from "node:fs"
import { fileURLToPath, URL } from "node:url"
import tailwindcss from "@tailwindcss/vite"
import react from "@vitejs/plugin-react"
import { defineConfig } from "vite"

const distDir = fileURLToPath(new URL("../internal/web/dist", import.meta.url))

// The hub owns the API, so `npm run dev` points the proxy at a running one.
// ABACAD_DEV_API lets the design work sit against a fixture instead.
const apiTarget = process.env.ABACAD_DEV_API ?? "http://127.0.0.1:8787"

export default defineConfig({
  plugins: [
    react(),
    tailwindcss(),
    {
      // emptyOutDir removes the tracked placeholder on every build. Put it back
      // so a plain `npm run build` leaves the git tree clean as well.
      name: "abacad-dist-placeholder",
      closeBundle() {
        mkdirSync(distDir, { recursive: true })
        writeFileSync(`${distDir}/.gitkeep`, "")
      },
    },
  ],
  resolve: {
    alias: {
      "@": fileURLToPath(new URL("./src", import.meta.url)),
    },
  },
  build: {
    outDir: distDir,
    emptyOutDir: true,
  },
  server: {
    port: 5173,
    proxy: {
      "/api": apiTarget,
      "/mcp": apiTarget,
      "/ws": {
        target: apiTarget.replace(/^http/, "ws"),
        ws: true,
      },
    },
  },
})
