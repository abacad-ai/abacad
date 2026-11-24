# abacad

A small, self-hosted **LAN VNC hub**.

Your machines already have VNC servers. abacad gives them one quiet place: a browser page for you, a standard VNC endpoint for an agent, and a single entry point for your LAN. No client to install on each device, no cloud, no relay, no public exposure.

```
  macOS Screen Sharing ─┐
  Linux x11vnc/wayvnc  ─┼── RFB ──▶  abacad hub  ── HTTPS/WSS ──▶ browser (noVNC)
  Windows TigerVNC     ─┤              │
  Android droidVNC-NG  ─┘              └── RFB/TCP :5901.. ──▶ agent's VNC client
```

In the default `gateway` mode the hub terminates only the RFB handshake (so the browser never sees a device password, and an agent can use a revocable token). After that it is a blind byte pipe. It never decodes a framebuffer and never synthesizes input.

- **Zero abacad software on devices.** Use each platform's existing VNC server (on Android, droidVNC-NG is the standard choice). On macOS,
  choose **macOS account** sign-in: the hub speaks Apple DH (type 30), which
  attaches to the logged-in user's console session. The legacy VNC password has
  no user identity and can land on the login window.
- **One binary.** Go + an embedded Vite/React frontend. Node is a build-time dependency only; the running hub needs no Node, database, or CDN.
- **LAN only.** Remote access is your network layer's job (VPN/firewall); abacad does not implement relays or port forwarding.
- **Standard AI interface.** An MCP tool lists devices and returns `vnc_host`, `vnc_port`, `token`. The agent connects with any VNC client.

Design and decision log: [docs/v2-rewrite-plan.md](docs/v2-rewrite-plan.md).

## Quick start

Requirements: Go 1.27+ and a machine that stays on your LAN (a Mac mini, a NAS, a Raspberry Pi). Node 24 is needed only to build the frontend, not to run the hub.

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
a macOS account. Leave
**Also open a raw VNC port for agents** unchecked for browser-only access, or
check it to give an agent a standard VNC endpoint.

Everything after that is in the web UI: add, edit, remove, probe, open, copy the
VNC endpoint. There are no management commands to remember.

- **Human:** click **Open** on the device card. noVNC runs in the tab.
- **Agent:** point it at the hub's MCP endpoint (below), or copy the `vnc://…`
  endpoint from the card.

## Running

```text
abacad [serve] [--config PATH] [--listen ADDR] [--no-mdns]
               [--tls-cert FILE --tls-key FILE] [-v]
abacad version
```

`ABACAD_CONFIG`, `ABACAD_LISTEN`, `ABACAD_ADMIN_TOKEN` and `ABACAD_MDNS` set
the same things from the environment, for containers; flags win. See
[Docker](#docker).

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
      "vnc_host": "192.168.1.10",
      "vnc_port": 5901,
      "auth": "hub-token",
      "token": "Abcd1234",
      "browser_url": "http://192.168.1.10:8787/vnc/mac-mini"
    }
  ]
}
```

The agent connects its own VNC client to `vnc_host:vnc_port` with `token` as the
VNC password. No new screen API to learn.

## Modes

| Mode | Browser | Raw VNC for agents | Credential handling |
|---|---|---|---|
| `gateway` (default) | yes, via hub | optional, via hub port | hub stores the device password; agents get a token |
| `l4` | yes (noVNC asks for the password) | optional | hub is a blind TCP pipe; no password storage needed |
| `direct` | no | no | catalog only; the agent connects to the device directly |

Raw VNC ports are opt-in per device. The browser path only needs one HTTP port.

## Health checks

The hub does **not** port-scan devices. TigerVNC counts every connection that
closes before authenticating as a security failure, and that counter does not
decay: a 30-second TCP probe eventually blacklists the hub's IP permanently.
There is no side-effect-free way to probe a VNC port.

Each device chooses one health mode:

| `health` | What it does |
|---|---|
| `session` (default) | Opens no connection. A device becomes online after a browser or agent session authenticates; the state goes stale after 10 minutes. |
| `command` | Runs `health_command` on the hub every 60 seconds, with exponential backoff. Exit 0 means online. Prefer SSH/systemd or an agent report. |
| `vnc` | One complete authenticated RFB handshake, at most every 5 minutes, with bounded failures. |

The command mode is the recommended way to watch a server that is already
reachable over SSH. It runs on the hub host, so `ssh` and the key must be
available there (and inside the container if you run abacad in Docker):

```yaml
health: command
health_command: ssh -o BatchMode=yes debian 'systemctl is-active tigervncserver@:2'
```

In `vnc` mode the hub sends `RFB 003.008`, selects VncAuth or Apple DH,
authenticates with the stored credential, reads ServerInit, and disconnects. It
never opens a connection without starting the handshake. After two
authentication failures it stops automatic checks for an hour; the manual
**Check** button is also limited to one VNC probe every five minutes.

If a server already answers `RFB 003.003` + `Too many security failures`,
restart the VNC server to clear its in-memory counter. Raising
`BlacklistThreshold` alone does not clear an existing lockout. See
[docs/incidents/2026-11-22-vnc-blacklist.md](docs/incidents/2026-11-22-vnc-blacklist.md)
for the full incident and verification.

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
Linux: it keeps mDNS discovery, source IPs, and the dynamically allocated raw
VNC ports (5901-5999) working. The compose file in this repo is the same setup:

```bash
docker compose up -d
```

### Configuration

The image keeps state in `/config` on a volume and takes deployment settings
from the environment. Flags still win over both.

| Variable | Default | Meaning |
|---|---|---|
| `ABACAD_CONFIG` | `/config/config.yaml` | config file path in the container |
| `ABACAD_LISTEN` | `0.0.0.0:8787` | listen address (the healthcheck follows its port) |
| `ABACAD_ADMIN_TOKEN` | generated | admin token, at least 16 characters, never written to `config.yaml` |
| `ABACAD_MDNS` | `true` | `false` disables discovery and advertising |

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

The `/config` volume is left alone, so devices and tokens survive. Tags are
`latest`, `2`, `2.0`, `2.0.0`; pin a version if you would rather upgrade on
purpose.

### Bridge networking

Where host networking is unavailable (macOS/Windows Docker Desktop, or a host
namespace you do not control), use `compose.bridge.yaml`. It publishes 8787 and
the raw VNC range 5901-5999. mDNS does not cross a bridge, so discovery is off
and devices are added by hand; a device that needs a raw port allocated after
container creation requires `docker compose up -d --force-recreate`.

### Healthcheck

The image probes `GET /healthz` on `ABACAD_LISTEN`'s port every 30 seconds.
That probe is plain HTTP, so a TLS deployment (`--tls-cert`/`--tls-key`) should
run with `--no-healthcheck`.

On macOS, run the native binary instead; Docker Desktop's host networking is
limited.

## Security model

- **LAN only.** Bind to a LAN interface with `--listen`. Do not expose the raw VNC ports to the internet; VNC's classic 8-character DES password is not internet-grade.
- **Device passwords stay in the hub** in `gateway` mode. The config file is written `0600`. A later version can move secrets to the OS keychain without changing the protocol.
- **Per-device tokens** are 8 characters because RFB's VNC authentication only carries 8 bytes. They are rotatable by editing the config, and the gateway can rate-limit failed attempts.
- **Browser sessions** use an HttpOnly, SameSite=Lax cookie derived from a random cookie secret.
- **The hub is a single ingress.** Restrict each device's VNC port to the hub's IP at the firewall. The hub itself becomes a high-value host: keep it patched, don't expose it, and back up the config.

More: [docs/security.md](docs/security.md).

## Status

The thin path is implemented:

- L7 RFB handshake gateway (browser `None`, agent token, upstream VNC password or macOS account via Apple DH type 30)
- raw VNC listeners per device (`gateway` and `l4`)
- Vite + React + React Router + Tailwind + shadcn frontend, built into `internal/web/dist` and embedded
- noVNC browser viewer built on the upstream JavaScript library (`@novnc/novnc`)
- mDNS discovery and advertising
- One `serve` command (also the default); device management is entirely in the web UI
- MCP `list_devices` over Streamable HTTP at `/mcp`
- multi-arch container image on GHCR with environment-only configuration and a healthcheck
- tests for RFB handshake, gateway WebSocket path, config, inventory

Not yet: automatic self-signed cert generation, OS keychain, view-only enforcement, thumbnails, recording, OCR, multi-agent leases.

Verified against a real **x11vnc** server (handshake, token, ServerInit, Raw
framebuffer) and the real macOS Screen Sharing banner/type list. macOS offers
`RFB 003.889` with Apple DH (type 30), which the hub now performs; choose
**macOS account** sign-in rather than the legacy VNC password. See
[docs/device-setup/macos.md](docs/device-setup/macos.md) and decision D15.

## Development

```bash
# terminal 1: the Go API/WebSocket server
go run ./cmd/abacad serve --listen 127.0.0.1:8787 --no-mdns

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

The RFB handshake tests use in-memory mock servers. An opt-in live test can point at a real VNC server (or a hub port):

```bash
ABACAD_LIVE_VNC=127.0.0.1:5901 ABACAD_LIVE_TOKEN=Abcd1234 \
  go test ./internal/rfb -run TestLiveServer -v
```

The frontend lives in `frontend/` (Vite, React, React Router, Tailwind, shadcn/ui).
The viewer uses the noVNC JavaScript library straight from npm (`@novnc/novnc`,
MPL 2.0) rather than the vendored noVNC application: `frontend/src/lib/vnc.ts`
wraps its `RFB` object, the page owns the canvas and the toolbar, and `npm run build`
bundles it into `internal/web/dist`. Bump the dependency like any other.
