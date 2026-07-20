# abacad server

A public, multi-tenant relay that lets a remote AI agent (over MCP) see and drive
a device (an Android phone today; Mac/Linux later). Users sign up, pair their
devices, and point their agent at one endpoint — `https://abacad.ai/mcp` — to reach
**their** devices.

```
                              ┌──────────────────────────────┐
  agent ──MCP (POST /mcp)────▶│   abacad server (Go)         │◀──WS (/device?token=)── device
  Bearer <account mcp token>  │   relay · accounts · MCP     │   per-device token
                              │   dashboard (SPA + /api)     │
  human ──browser────────────▶│                              │
                              └──────────────────────────────┘
```

- **`backend/`** — Go, `coder/websocket`, stdlib `net/http` (Go 1.22 `ServeMux`), SQLite
  (`modernc.org/sqlite`, no CGO). One binary serves the MCP endpoint, the device
  WebSocket, the dashboard API, and the embedded dashboard SPA.
- **`frontend/`** — Vite + React + React Router + Tailwind v4 (shadcn-style UI). Built
  and embedded into the Go binary for production.

## Architecture

- **Accounts** — email + password → `httpOnly` session cookie (dashboard). Auth is
  deliberately minimal for now; it will be hardened later.
- **MCP endpoint** (`POST /mcp`) — stateless JSON-RPC (Streamable HTTP). Authenticated
  by a scoped **API key** as `Authorization: Bearer <token>`. Each key is restricted to
  a set of devices and methods (and, optionally, the `/connect` tunnel); create and
  manage keys on the dashboard's **Access** page. Tools:
  `list_devices`, `screenshot`, `tap`, `long_press`, `swipe`, `input_text`, `back`,
  `home`, `recents`. Every action tool takes an optional `device_id`; omit it to use
  your only / most-recently-active device.
- **Device WebSocket** (`/device?token=<device-token>`) — the device dials out
  (NAT-friendly) and holds the connection open. The per-device token maps the socket
  to an account + a unique `device_id`. The wire protocol (`{id,method,params}` /
  `{id,ok,result|error}`) is unchanged from v0, so the Android app connects with no
  code change — just paste the `wss://…/device?token=…` URL. The channel carries
  **control frames only** — commands, replies, metadata; binary blobs (screenshots,
  files) go over a generic HTTP `/blobs` pair. See [`../docs/transport.md`](../docs/transport.md).
- **Tokens** are stored **hashed** (sha-256); the plaintext is shown once, on
  create/rotate.
- **SSH jump host** (opt-in, `-ssh-addr`) — makes each device reachable as
  `ssh <device>.<base-domain>` for a **stock `ssh` client with nothing installed** (one
  `ProxyJump` line in `~/.ssh/config`). The client's public key (registered under
  Settings) identifies the account; the jump routes only to that account's online
  devices and bridges into the device tunnel to its own `127.0.0.1:22`. The inner SSH
  session stays end-to-end encrypted — the relay holds no keys. See
  [`../docs/ssh.md`](../docs/ssh.md).

## Build & run

Requires Go and Node.

```bash
./build.sh                 # builds the SPA, embeds it, compiles backend/abacad
./backend/abacad           # listens on :8848 (override with -addr / ABACAD_ADDR)
```

Flags: `-addr :8848`, `-db abacad.db`, `-dev-cors` (local dev), `-seed` (mint a dev
account/device/API key and print them). SSH jump host (opt-in): `-ssh-addr :22,:443`,
`-ssh-host-key <path>`, `-base-domain abacad.ai` — see [`../docs/ssh.md`](../docs/ssh.md).

Register the endpoint with your agent:

```bash
claude mcp add --transport http --header "Authorization: Bearer <api-key>" \
  abacad http://localhost:8848/mcp
```

## Develop

Two processes, shared origin via the Vite proxy:

```bash
cd backend && go run ./cmd/abacad -dev-cors    # :8848
cd frontend && npm install && npm run dev       # :5173  (proxies /api /mcp /device → :8848)
```

Open http://localhost:5173, register, add a device (shows a `wss://…?token=…` URL +
QR), and create an API key under Access.

## Verify without a phone

`mock-device.mjs` stands in for the device; `smoke.mjs` / `test-multi.mjs` act as the
agent (using the real MCP SDK client).

```bash
npm install                              # harness deps (ws + MCP SDK)

# Single-device loop against a seeded server:
./backend/abacad -db /tmp/abacad.db -seed &     # prints device_token / api_key
SERVER_URL="ws://localhost:8848/device?token=<device_token>" node mock-device.mjs &
MCP_TOKEN="<api_key>" node smoke.mjs             # -> SMOKE OK

# Multi-tenant isolation/routing (provisions two accounts via the API):
./backend/abacad -db /tmp/abacad-multi.db &
BASE=http://localhost:8848 node test-multi.mjs   # -> MULTI OK
```

## Deploy

This repo builds no deployment. CI pushes `ghcr.io/abacad-ai/abacad:latest` on every
push to main; the production deploy — compose file, config, the macOS client dmg,
restart, health check — lives outside this repo, in the infra repo under
`deployment/xyz-sg-1/abacad.ai/` (`./deploy.sh` there, see its README).

Production terminates TLS/`wss` at Caddy and proxies to the Go server. The proxy must
not cap WebSocket frame size below screenshot payloads and needs long/absent idle
timeouts on `/device`. The server strips query strings from its logs (device tokens
ride in the query); redact them at the proxy too.

`GET /downloads/<file>` serves public release artifacts (e.g.
`abacad-macos-latest.dmg`) from a plain directory (`-downloads` /
`ABACAD_DOWNLOADS`, `/data/abacad-downloads` in Docker) — publishing a build is just
a file copy into the data volume, no restart.

## Not yet (deliberately)

Hardened auth (rate limits, email verification, CSRF, token scopes), pairing-code flow
(vs. paste-the-URL), approval gating, desktop backends. Each is additive behind the same
device wire protocol and MCP tool contract.
