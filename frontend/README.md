# abacad frontend

Vite + React + TypeScript + React Router + Tailwind CSS + shadcn/ui.

```bash
npm ci
npm run dev     # http://127.0.0.1:5173, proxies /api, /novnc, /ws, /mcp to :8787
npm run build   # writes ../internal/web/dist, embedded by the Go binary
```

The Go server must be running on `127.0.0.1:8787` for the dev proxy. The
production build is embedded with `//go:embed`; the checked-in
`internal/web/dist/.gitkeep` lets `go build` succeed before the first build.
