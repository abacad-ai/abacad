# abacad

A small, self-hosted **VNC hub**.

Public site and docs: <https://abacad.ai> (source: [`site/`](site/)).

Your machines already have VNC servers. abacad gives them one browser tab: a page for you, the same VNC stream for an agent, and one entry point for any VNC server the hub can reach. No client to install on each device, no cloud, no relay, no public forwarding.

```
  macOS Screen Sharing ─┐
  Linux x11vnc/wayvnc  ─┤
  Windows TigerVNC     ─┼── RFB ──▶  abacad hub  ── HTTPS/WSS ──▶ browser (noVNC)
  Android droidVNC-NG  ─┤              │
  iOS/iPadOS via the Mac app─┘         └── same WSS ──▶ agent's VNC client
```

In the default `gateway` mode the hub terminates only the RFB handshake (so neither the browser nor an agent ever sees a device password). After that it is a blind byte pipe. It never decodes a framebuffer and never synthesizes input.

- **Zero abacad software on devices.** Use each platform's existing VNC server (on Android, droidVNC-NG is the standard choice). On macOS,
  choose **macOS account** sign-in: the hub speaks Apple DH (type 30), which
  attaches to the logged-in user's console session. The legacy VNC password has
  no user identity and can land on the login window.
- **iPhone and iPad** have no VNC server of their own, so they reach the hub
  through [iOS VNC Bridge](https://github.com/abacad-ai/ios-vnc-bridge): a
  macOS menu-bar app for the Mac the device is paired to, which speaks plain
  RFB to the hub. No jailbreak. See
  [docs/device-setup/ios.md](docs/device-setup/ios.md).
- **One binary.** Go + an embedded Vite/React frontend. Node is a build-time dependency only; the running hub needs no Node, database, or CDN.
- **No relay, no public forwarding.** abacad is not a hosted service. You run the hub and own the network path — a VPN, a tunnel, your own firewall. The hub connects directly to the addresses you add, which is faster and safer than forwarding traffic through a relay.
- **One port, one secret.** Browser, API, MCP and every device's VNC stream go
  through the hub's single HTTP port, so one reverse proxy covers all of it. Each
  device is a websockify-style WebSocket at `/ws/vnc/<id>` carrying plain RFB, and
  one admin token opens everything.
- **Standard AI interface.** An MCP tool lists devices and returns each `ws_url`. The agent uses a WebSocket VNC client, or bridges it to a local port with `websocat` and uses any VNC client.
- **No MCP? A skill instead.** The **Skill** page at `/skill` generates a ready-made
  `SKILL.md` for an agent: the admin token and machine list baked in, plus the
  rules that keep the hub healthy. No server-side integration.

Design and decision log: [docs/v2-rewrite-plan.md](docs/v2-rewrite-plan.md) ·
visual system: [brand/theme.md](brand/theme.md).

## Quick start

Requirements: Go 1.27+ and a machine you control (a Mac mini, a NAS, a Raspberry Pi). Node 24 is needed only to build the frontend, not to run the hub.

```bash
make build          # npm ci + vite build + go build
./bin/abacad serve
```

Or skip the toolchain entirely and run the container: `docker compose up -d`
(see [Docker](#docker)).

Or build the two halves by hand:

```bash
cd frontend && npm ci && npm run build && cd ..
go build -o abacad ./cmd/abacad
./abacad serve
```

On first run it creates `~/.config/abacad/config.yaml` and prints the URL and an admin token:

```
  abacad is running
  → http://192.168.1.10:8787
  admin token: 3f2c...
```

Open the URL in a browser, paste the token, and click **Add device**. Fill in
the device's name, host, VNC port (usually 5900), and either the VNC password or
a macOS account. The host can be any address the hub can reach: a private IP, a
VPN address, a DNS name, or a forwarded local port. The same device is then
available to agents at `/ws/vnc/<id>` with the admin token; there is nothing
extra to enable.

Everything after that is in the web UI: add, edit, remove, open, copy the VNC
endpoint. There are no management commands to remember.

- **Human:** select a machine in the rail and its screen opens in the pane next to
  it. The `…` menu on a row is where a machine is edited or removed.
- **Agent:** point it at the hub's MCP endpoint (below); the device list it returns
  carries the `vnc://…` endpoint and token.

## Running

```text
abacad [serve] [--config PATH] [--listen ADDR]
               [--tls-cert FILE --tls-key FILE] [-v]
abacad version
```

`ABACAD_CONFIG`, `ABACAD_LISTEN` and `ABACAD_ADMIN_TOKEN` set the same things
from the environment, for containers; flags win. See [Docker](#docker).

With no arguments, `abacad` serves. `serve` is accepted for explicit compose
files. For HTTPS/WSS (needed for browser clipboard access in most browsers),
provide a certificate:

```bash
./abacad serve --tls-cert cert.pem --tls-key key.pem
```

Use a certificate your browser trusts (a local CA, or a reverse proxy in front).

## MCP

MCP is served by the hub at **`POST /mcp`** (Streamable HTTP). It uses short-lived sessions and
returns JSON. Authenticate with the admin token:

```bash
claude mcp add --transport http \
  --header "Authorization: Bearer <admin token>" \
  abacad http://192.168.1.10:8787/mcp
```

Or in a JSON client config:

```json
{
  "mcpServers": {
    "abacad": {
      "type": "http",
      "url": "http://192.168.1.10:8787/mcp",
      "headers": { "Authorization": "Bearer <admin token>" }
    }
  }
}
```

It exposes one tool, `list_devices`:

```json
{
  "devices": [
    {
      "id": "mac-mini",
      "name": "Mac mini",
      "online": true,
      "known": true,
      "status": "online",
      "mode": "gateway",
      "ws_url": "ws://192.168.1.10:8787/ws/vnc/mac-mini",
      "auth": "none",
      "browser_url": "http://192.168.1.10:8787/vnc/mac-mini"
    }
  ]
}
```

The agent opens `ws_url` with the same admin token (`Authorization: Bearer`, or
`?token=` for clients that take only a URL). With no WebSocket support in its VNC
client, it bridges first:

```bash
websocat --binary -H="Authorization: Bearer <admin token>" \
  tcp-l:127.0.0.1:5900 ws://192.168.1.10:8787/ws/vnc/mac-mini &
vncdotool -s 127.0.0.1::5900 capture screen.png
```

No new screen API to learn.

## Modes

| Mode | Browser and agents | Credential handling |
|---|---|---|
| `gateway` (default) | yes, via the hub's WebSocket | hub stores the device password; clients see no VNC password |
| `l4` | yes (the client answers the device's password) | hub is a blind pipe; no password storage needed |
| `direct` | no | catalog only; the agent connects to the device directly |

Everything goes through one HTTP port.

## Online status

abacad never opens a connection to a device on its own. There is no scheduled
check, no health command and no **Check** button: a device is marked **online**
only after a real browser or agent session authenticates to it, and that state
goes stale after 10 minutes. A device you have never opened shows **not seen**
until you do.

This is deliberate. TigerVNC counts every connection that closes before
authenticating as a security failure, and its counter does not decay: even a
well-behaved probe is still a client connection that can eventually blacklist
the hub. The only connection abacad makes is the one you or an agent asked for.

If a server already answers `RFB 003.003` + `Too many security failures` because
of something else on the network, restart the VNC server to clear its in-memory
counter. Raising `BlacklistThreshold` alone does not clear an existing lockout.
See [docs/incidents/2026-11-22-vnc-blacklist.md](docs/incidents/2026-11-22-vnc-blacklist.md)
for the incident history.

## No discovery

abacad does not browse, scan or advertise over mDNS. There is no `_rfb._tcp`
query, no `_abacad._tcp` announcement, and no discovered-devices list. The rail
only lists devices you added by host:port. If you want the hub discoverable, put
its URL wherever your tools already look — abacad will not announce itself.

## Docker

A prebuilt image is published on every tagged release for **linux/amd64** and
**linux/arm64**:

```bash
docker run -d --name abacad --network host \
  -v abacad-config:/config \
  ghcr.io/abacad-ai/abacad:latest
docker logs abacad    # prints the admin token
```

Then open `http://<host>:8787`. `--network host` is strongly recommended on
Linux: it keeps real client IPs in the hub's logs. The compose file in this repo is the same setup:

```bash
docker compose up -d
```

### Ports

One port in. The console, the API, MCP and every device's VNC stream are served
from `ABACAD_LISTEN` (`8787` by default), so there is no port pool and no port
range to publish — device two and device fifty ride the same port, at
`/ws/vnc/<id>`:

```yaml
# compose, bridge networking: the whole mapping. there is no 5901-5999 to add
ports:
  - "8787:8787"
```

The hub dials each device's own VNC port **outbound** — `5900` by default,
`5902` for TigerVNC's `:2` display, `5970` for the iOS VNC Bridge. Those ports
must be reachable from the hub and should be firewalled to the hub's IP; they
are never published on the hub host. A `direct` device is the exception: the hub
carries no traffic for it and the client connects to the device itself.

### Configuration

The image keeps state in `/config` on a volume and takes deployment settings
from the environment. Flags still win over both.

| Variable | Default | Meaning |
|---|---|---|
| `ABACAD_CONFIG` | `/config/config.yaml` | config file path in the container |
| `ABACAD_LISTEN` | `0.0.0.0:8787` | listen address (the healthcheck follows its port) |
| `ABACAD_ADMIN_TOKEN` | generated | admin token, at least 16 characters, never written to `config.yaml` |

The generated token is printed to `docker logs`. To pin one from your secret
store instead:

```bash
docker run -d --name abacad --network host \
  -e ABACAD_ADMIN_TOKEN="$(openssl rand -hex 32)" \
  -v abacad-config:/config \
  ghcr.io/abacad-ai/abacad:latest
```

Devices live in `config.yaml` on the volume: `docker compose down` keeps them,
`docker compose down -v` deletes them. To keep the file on the host, bind-mount
`./config:/config` and `chown -R 10001 ./config` (the image runs as uid 10001).

### Upgrading

```bash
docker compose pull && docker compose up -d
docker run --rm ghcr.io/abacad-ai/abacad:latest version
```

The `/config` volume is left alone, so devices survive. Tags are `latest`, `3`,
`3.0`, `3.0.0`; pin a version if you would rather upgrade on purpose.

3.0 dropped raw VNC ports and per-device tokens: every device is reached over
the hub's WebSocket, and the admin token is the only secret. An old config file
still loads; the dropped fields go away on the next save.

### Bridge networking

Where host networking is unavailable (macOS/Windows Docker Desktop, or a host
namespace you do not control), use `compose.bridge.yaml`. It publishes 8787,
which carries everything.

### Healthcheck

The image calls `GET /healthz` on `ABACAD_LISTEN`'s port every 30 seconds.
That probe is plain HTTP, so a TLS deployment (`--tls-cert`/`--tls-key`) should
run with `--no-healthcheck`.

On macOS, run the native binary instead; Docker Desktop's host networking is
limited.

## Security model

- **No public forwarding.** Bind the hub to the interface you choose and put it behind the network controls you trust. If the hub must be reachable from outside, put it behind TLS (a reverse proxy or `--tls-cert`) and a VPN; the admin token opens every machine.
- **Device passwords stay in the hub** in `gateway` mode. The config file is written `0600`. A later version can move secrets to the OS keychain without changing the protocol.
- **One admin token** (32 random bytes) authenticates the web login, the API, MCP and the device WebSockets. Rotate it by editing `admin_token` in the config (or `ABACAD_ADMIN_TOKEN`) and restarting.
- **Browser sessions** use an HttpOnly, SameSite=Lax cookie derived from a random cookie secret.
- **The hub is a single ingress.** Restrict each device's VNC port to the hub's IP at the firewall. The hub itself becomes a high-value host: keep it patched, don't expose it, and back up the config.

More: [docs/security.md](docs/security.md).

## Status

The thin path is implemented:

- L7 RFB handshake gateway (client `None`, upstream VNC password or macOS account via Apple DH type 30)
- one WebSocket per device at `/ws/vnc/<id>` for browsers and agents (`gateway` and `l4`)
- Vite + React + React Router + Tailwind + shadcn frontend, built into `internal/web/dist` and embedded
- noVNC viewer: the upstream application (1.7.0) vendored under
  `frontend/public/novnc`, served at `/novnc` and loaded in an iframe
- One `serve` command (also the default); device management is entirely in the web UI
- MCP `list_devices` over Streamable HTTP at `/mcp`
- `/skill` page that hands an agent a ready-made `SKILL.md` (admin token and machine list baked in)
- multi-arch container image on GHCR with environment-only configuration and a healthcheck
- tests for RFB handshake, gateway WebSocket path, config, inventory

Not yet: automatic self-signed cert generation, OS keychain, view-only enforcement, thumbnails, recording, OCR, multi-agent leases.

Verified against a real **x11vnc** server (handshake, token, ServerInit, Raw
framebuffer) and the real macOS Screen Sharing banner/type list. macOS offers
`RFB 003.889` with Apple DH (type 30), which the hub now performs; choose
**macOS account** sign-in rather than the legacy VNC password. See
[docs/device-setup/macos.md](docs/device-setup/macos.md) and decision D15.

## Design

The interface is **porcelain**: paper, ink and three greys, with the logo's two
colours used almost nowhere else.

| Token | Value | Where |
| --- | --- | --- |
| Paper / surface | `#F6F7F8` / `#FFFFFF` | page, card, selected row |
| Ink | `#15171A` | text, headings, **every primary button** |
| Line / secondary / faint | `#E5E7EA` / `#63686F` / `#9AA0A7` | hairlines, captions, placeholders |
| Accent — "you are here" | `#13A7F5` (text `#0B70AC`) | a live dot, the focus ring, the next step, links |
| Signal — bad news | `#FD605E` (text `#BF3A37`) | a failed session, an error, Remove |

Blue appears once per screen at most; coral never decorates; the only place the
two logo colours meet is the mark itself. Type is **Geist** for the UI and the
wordmark and **Geist Mono** for addresses, versions and all-caps micro labels —
there is no second display face. Every separation is a 1px hairline, depth is two
shadow levels, and there are no gradients or glass.

Tokens live only in [`frontend/src/index.css`](frontend/src/index.css);
components use `bg-card`, `border-border`, `text-muted-foreground`, `bg-brand`
and friends. `npm run lint` fails on a raw colour in a component — the mark
([`brand-mark.tsx`](frontend/src/components/brand-mark.tsx)) is the one
exception, because its two fills are the logo, not the theme.

The full system — roles, type scale, states, the do/don't list — is
[brand/theme.md](brand/theme.md), with the sheet at
[brand/theme.png](brand/theme.png). The vendored noVNC viewer deliberately keeps
its own UI: its control bar is a third-party application, not our chrome
([docs/vendored-novnc.md](docs/vendored-novnc.md)).

## Development

```bash
# terminal 1: the Go API/WebSocket server
go run ./cmd/abacad serve --listen 127.0.0.1:8787

# terminal 2: Vite with HMR, proxying /api, /ws and /mcp to :8787
cd frontend
npm ci
npm run dev
```

Open http://127.0.0.1:5173. `npm run build` writes the production bundle to
`internal/web/dist`; `go build` embeds that directory. The checked-in
`.gitkeep` keeps `go build` working on a tree where the frontend has not been
built yet.

```bash
go test ./...
go vet ./...
```

The RFB handshake tests use in-memory mock servers. An opt-in live test can point at a real VNC server:

```bash
ABACAD_LIVE_VNC=127.0.0.1:5900 ABACAD_LIVE_TOKEN=<vnc password> \
  go test ./internal/rfb -run TestLiveServer -v
```

The frontend lives in `frontend/` (Vite, React, React Router, Tailwind, shadcn/ui).
The screen is noVNC's own application, vendored at `frontend/public/novnc` (1.7.0,
MPL-2.0 core + BSD app UI) and loaded in an iframe — the same split Proxmox VE
uses. npm cannot supply that half: `@novnc/novnc` publishes only `core/`, so the
control bar and its stylesheet have to travel as files. Keys, clipboard,
fullscreen, scaling and reconnect are noVNC's; the rail, login and device API are
ours. See [docs/vendored-novnc.md](docs/vendored-novnc.md) for the file list and
the upgrade steps.
