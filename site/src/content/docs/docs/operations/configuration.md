---
title: Configuration
description: The config.yaml fields, environment variables, CLI flags, modes, and auth types for a self-hosted abacad hub.
---

The hub keeps its whole state in one YAML file. There is no database. The web console
edits the same file, so you can manage devices either way.

## Where it lives

- Default: `~/.config/abacad/config.yaml`
- `--config PATH` or `ABACAD_CONFIG` overrides it.
- Docker: `/config/config.yaml` on the mounted volume.

On first run the file is created with a generated `admin_token` and `cookie_secret`. It is
written `0600` because it contains device passwords.

## A full example

```yaml
listen: 0.0.0.0:8787
admin_token: 3f2c...        # 64 hex characters, generated
cookie_secret: 9a71...      # signs the browser session cookie
devices:
  - id: mac-mini
    name: Mac mini
    host: 192.168.1.92
    port: 5900
    auth: ard                # ard (macOS account) or vnc
    username: iloahz         # required for ard
    password: "the account password"
    platform: macos
    mode: gateway            # gateway, l4, or direct
    created_at: 2026-11-25T09:00:00Z

  - id: linux-box
    name: linux-box
    host: 192.168.1.30
    port: 5900
    auth: vnc
    password: "the VNC password"
    platform: linux
    mode: gateway
```

## Fields

| Field | Meaning |
| --- | --- |
| `listen` | Address the hub listens on. Default `0.0.0.0:8787`. |
| `admin_token` | The only secret that opens the hub. At least 16 characters; generated as 64 hex. |
| `cookie_secret` | Signs the HttpOnly browser session cookie. Rotating it signs every browser out. |
| `devices[].id` | URL segment in `/ws/vnc/<id>`. Generated from the name; must be unique. |
| `devices[].name` | Display name in the rail. |
| `devices[].host` | Address **the hub** uses to reach the device: LAN IP, VPN address, DNS name, or forwarded local port. |
| `devices[].port` | VNC port, usually 5900. |
| `devices[].auth` | `vnc` for a VNC password, `ard` for a macOS account (RFB type 30). |
| `devices[].username` | macOS short name. Required when `auth: ard`. |
| `devices[].password` | The VNC or account password. Omitted in `l4` mode if you do not want it stored. |
| `devices[].platform` | Free-form label used for the icon and status hints: `macos`, `windows`, `linux`, `android`, `ios`. |
| `devices[].mode` | `gateway` (default), `l4`, or `direct`. |
| `devices[].created_at` | Set when the device is added; informational. |

### Modes

| Mode | What the hub does | Client authentication |
| --- | --- | --- |
| `gateway` | Terminates the RFB handshake, signs in to the device, then pipes bytes. | None; the hub handles it. |
| `l4` | Blind TCP forwarding over the WebSocket. | The client answers the device's VNC password. |
| `direct` | Catalog only; no WebSocket. | The client connects to `host:port` itself. |

### Auth types

| Auth | RFB security | Use |
| --- | --- | --- |
| `vnc` | Type 2 (classic VNC password) | Windows, Linux, Android, iosvnc, legacy macOS. |
| `ard` | Type 30 (Apple DH) | macOS with a real account. Attaches to the console session. |

## Environment and flags

| Variable | Flag | Meaning |
| --- | --- | --- |
| `ABACAD_CONFIG` | `--config PATH` | Config file path. |
| `ABACAD_LISTEN` | `--listen ADDR` | Listen address. |
| `ABACAD_ADMIN_TOKEN` | none | Admin token. Runtime-only; never written to the file. Must be at least 16 characters. |
| none | `--tls-cert FILE` | Serve HTTPS/WSS with this certificate. |
| none | `--tls-key FILE` | Private key for the certificate. |
| none | `-v` | Verbose logs. |

Flags win over environment variables, which win over the file. `abacad` with no arguments
serves; `serve` is accepted for explicit compose files. `abacad version` prints the build
version.

## Editing and rotating

- **Hand edits are read at startup.** The running hub writes the file when you add, edit,
  or remove a device in the UI. If you edit it yourself, restart to apply.
- **Rotating `admin_token`** invalidates every browser session, agent connection, and saved
  skill immediately. Update the token in your MCP clients and regenerate any `SKILL.md`.
- **Rotating `cookie_secret`** signs every browser out but does not affect the admin token
  or MCP.
- **Back up the file.** It contains the token and every device password. Without it you can
  rebuild the device list, but not recover the secrets.

## Health and logs

The image's healthcheck is `GET /healthz` on the listen port. A TLS deployment using
`--tls-cert`/`--tls-key` should pass `--no-healthcheck` to Docker, because the probe is
plain HTTP. Use `-v` when diagnosing a connection; the hub logs the device, mode, and
handshake outcome.
