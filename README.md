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
  enable **VNC viewers may control screen with password**; without it, Screen
  Sharing only offers Apple-specific auth that the hub does not speak.
- **One binary.** Go + embedded noVNC + a few static files. No database, no Node build at runtime.
- **LAN only.** Remote access is your network layer's job (VPN/firewall); abacad does not implement relays or port forwarding.
- **Standard AI interface.** An MCP tool lists devices and returns `vnc_host`, `vnc_port`, `token`. The agent connects with any VNC client.

Design and decision log: [docs/v2-rewrite-plan.md](docs/v2-rewrite-plan.md).

## Quick start

Requirements: Go 1.27+ and a machine that stays on your LAN (a Mac mini, a NAS, a Raspberry Pi).

```bash
go build -o abacad ./cmd/abacad
./abacad serve
```

On first run it creates `~/.config/abacad/config.yaml` and prints the URL and an admin token:

```
  abacad is running
  → http://192.168.1.10:8787
  admin token: 3f2c...
```

Open the URL in a browser, paste the token, and add a device:

```bash
# or from the CLI
./abacad add --name mac-mini --host 192.168.1.20 --password 'your-vnc-password' \
  --platform macos --expose
```

`--expose` opens a raw VNC port on the hub for agents. Without it the device is browser-only.

Then:

- **Human:** click **Open** on the device card. noVNC runs in the tab.
- **Agent:** run `abacad mcp` (stdio) and call `list_devices`; or copy the `vnc://…` endpoint from the card.

## CLI

```text
abacad serve   [--config PATH] [--listen ADDR] [--no-mdns] [-v]
abacad add     --name NAME --host HOST [--port 5900] [--password PW]
               [--platform macos|linux|windows|android]
               [--mode gateway|l4|direct] [--expose]
abacad list    [--probe] [--json]
abacad remove  ID
abacad probe   [ID]
abacad mcp     [--host HOST] [--base-url URL]
abacad doctor
abacad token   [--rotate]
abacad version
```

`list` and `doctor` do not require the hub to be running; they read the same config.

For HTTPS/WSS (needed for browser clipboard access in most browsers), provide a
certificate:

```bash
./abacad serve --tls-cert cert.pem --tls-key key.pem
```

Use a certificate your browser trusts (a local CA, or a reverse proxy in front).


The CLI edits the config file. A running hub keeps its in-memory inventory and
listeners until restart, so use the web UI for live add/remove/probe, or restart
`abacad serve` after CLI changes. `abacad token --rotate` also takes effect only
after a restart.

## MCP

`abacad mcp` speaks MCP over stdio and exposes one tool:

```json
{
  "name": "list_devices",
  "description": "List VNC devices on this LAN and the standard VNC endpoint to use."
}
```

Example output:

```json
{
  "devices": [
    {
      "id": "mac-mini",
      "name": "Mac mini",
      "online": true,
      "mode": "gateway",
      "vnc_host": "192.168.1.10",
      "vnc_port": 5901,
      "token": "Abcd1234",
      "browser_url": "http://192.168.1.10:8787/vnc/mac-mini"
    }
  ]
}
```

Register it with an MCP client:

```json
{
  "mcpServers": {
    "abacad": {
      "command": "/absolute/path/to/abacad",
      "args": ["mcp", "--host", "192.168.1.10"]
    }
  }
}
```

The agent then connects its own VNC client to `vnc_host:vnc_port` with `token` as the VNC password. No new screen API to learn.

## Modes

| Mode | Browser | Raw VNC for agents | Credential handling |
|---|---|---|---|
| `gateway` (default) | yes, via hub | optional, via hub port | hub stores the device password; agents get a token |
| `l4` | yes (noVNC asks for the password) | optional | hub is a blind TCP pipe; no password storage needed |
| `direct` | no | no | catalog only; the agent connects to the device directly |

Raw VNC ports are opt-in per device. The browser path only needs one HTTP port.

## Docker

`--network host` is strongly recommended: it keeps mDNS discovery, source IPs, and dynamically allocated VNC ports working. Bridge networking can publish a fixed range, but mDNS will not cross it and new ports cannot appear without recreating the container.

```yaml
services:
  abacad:
    build: .
    network_mode: host
    volumes:
      - ./config:/config
    command: ["serve", "--config", "/config/config.yaml", "--listen", "0.0.0.0:8787"]
```

On macOS, run the native binary instead; Docker Desktop's host networking is limited.

## Security model

- **LAN only.** Bind to a LAN interface with `--listen`. Do not expose the raw VNC ports to the internet; VNC's classic 8-character DES password is not internet-grade.
- **Device passwords stay in the hub** in `gateway` mode. The config file is written `0600`. A later version can move secrets to the OS keychain without changing the protocol.
- **Per-device tokens** are 8 characters because RFB's VNC authentication only carries 8 bytes. They are rotatable by editing the config, and the gateway can rate-limit failed attempts.
- **Browser sessions** use an HttpOnly, SameSite=Lax cookie derived from a random cookie secret.
- **The hub is a single ingress.** Restrict each device's VNC port to the hub's IP at the firewall. The hub itself becomes a high-value host: keep it patched, don't expose it, and back up the config.

More: [docs/security.md](docs/security.md).

## Status

v0.1 implements the thin path:

- L7 RFB handshake gateway (browser `None`, agent token, upstream device password)
- raw VNC listeners per device (`gateway` and `l4`)
- noVNC browser viewer with vendored assets
- mDNS discovery and advertising
- MCP `list_devices` over stdio
- CLI (`serve`, `add`, `list`, `remove`, `probe`, `mcp`, `doctor`, `token`)
- tests for RFB handshake, gateway WebSocket path, config, inventory

Not in v0.1: automatic self-signed cert generation, OS keychain, view-only enforcement, thumbnails, recording, OCR, multi-agent leases.

Verified against a real **x11vnc** server (handshake, token, ServerInit, Raw
framebuffer). macOS Screen Sharing was probed over the LAN: it advertises
`RFB 003.889` and, without the legacy VNC password checkbox, only security types
`[30 33 35 36]`. Enable the checkbox (or use TigerVNC) before adding a Mac. See
[docs/device-setup/macos.md](docs/device-setup/macos.md) and decision D15.

## Development

```bash
go test ./...
go vet ./...
go run ./cmd/abacad serve --listen 127.0.0.1:8787 --no-mdns
```

The RFB handshake tests use in-memory mock servers. An opt-in live test can point at a real VNC server (or a hub port):

```bash
ABACAD_LIVE_VNC=127.0.0.1:5901 ABACAD_LIVE_TOKEN=Abcd1234 \
  go test ./internal/rfb -run TestLiveServer -v
```

noVNC is vendored under `internal/web/assets/novnc` (v1.6.0, MPL 2.0; see its `LICENSE.txt`).
