# abacad frontend

Vite + React + TypeScript + React Router + Tailwind CSS + shadcn/ui.

```bash
npm ci
npm run dev     # http://127.0.0.1:5173, proxies /api, /ws, /mcp to :8787
npm run build   # writes ../internal/web/dist, embedded by the Go binary
```

The Go server must be running on `127.0.0.1:8787` for the dev proxy — or set
`ABACAD_DEV_API` to point it at another hub (a fixture, a remote box). The
production build is embedded with `//go:embed`; the checked-in
`internal/web/dist/.gitkeep` lets `go build` succeed before the first build.

Colour and type are decided once, in [brand/theme.md](../brand/theme.md), and
implemented as tokens in [`src/index.css`](src/index.css). Components do not
carry their own hex values: `npm run lint` runs `scripts/check-theme.mjs` and
fails on a raw colour, with the brand mark as the one exception.
