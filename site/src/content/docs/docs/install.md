---
title: Install
description: Run abacad with Docker or build the single Go binary. Configure TLS, a reverse proxy, the config file, upgrades, and the healthcheck.
---

The hub is one binary with the frontend embedded. Node is a build-time dependency only;
the running hub needs no Node, database, or CDN.

## Docker

A prebuilt image is published on every tagged release for **linux/amd64** and
**linux/arm64**:

```bash
docker run -d --name abacad --network host \
  -v abacad-config:/config \
  ghcr.io/abacad-ai/abacad:latest

docker logs abacad
```

Then open `http://<host>:8787`.

On Linux, `--network host` is strongly recommended: it keeps real client IPs in the hub's
logs, which matters when you are deciding what to allow through a firewall. The compose
file in the repository is the same setup:

```bash
docker compose up -d
```

### Ports

The hub publishes one port. Every device is a path on it — `/ws/vnc/<id>` — so adding
devices never adds ports, and no compose file needs a `5901-5999` range.

- **Inbound, on the hub host:** `8787`, or `443` if a reverse proxy terminates TLS. Nothing
  else. In bridge networking that is the whole mapping: `-p 8787:8787`.
- **Outbound, from the hub:** each device's own VNC port — `5900` by default, `5902` for
  TigerVNC's `:2` display, `5970` for the iOS VNC Bridge. They must be reachable from the
  hub and should be allowed only from the hub's IP. They are never published on the hub.
- **`direct` mode** is the one exception: the hub carries no traffic for that device and
  the client dials its `host:port` itself.

An agent whose VNC client speaks only TCP bridges on its own machine (`websocat`), not on
the hub; the hub is still one port.

### Environment

The image keeps state in `/config` on a volume and takes deployment settings from the
environment. Flags still win over both.

| Variable | Default | Meaning |
| --- | --- | --- |
| `ABACAD_CONFIG` | `/config/config.yaml` | Config file path in the container |
| `ABACAD_LISTEN` | `0.0.0.0:8787` | Listen address; the healthcheck follows its port |
| `ABACAD_ADMIN_TOKEN` | generated | Admin token, at least 16 characters, never written to `config.yaml` |

The generated token is printed to `docker logs`. To pin one from your secret store
instead:

```bash
docker run -d --name abacad --network host \
  -e ABACAD_ADMIN_TOKEN="$(openssl rand -hex 32)" \
  -v abacad-config:/config \
  ghcr.io/abacad-ai/abacad:latest
```

Devices live in `config.yaml` on the volume: `docker compose down` keeps them,
`docker compose down -v` deletes them. To keep the file on the host, bind-mount
`./config:/config` and `chown -R 10001 ./config` (the image runs as uid 10001).

### Bridge networking instead of host

Where host networking is unavailable — macOS or Windows Docker Desktop, or a host
namespace you do not control — publish 8787 instead. That one port carries the console,
the API, MCP, and every device WebSocket:

```bash
docker run -d --name abacad \
  -p 8787:8787 \
  -v abacad-config:/config \
  ghcr.io/abacad-ai/abacad:latest
```

See the repository's `compose.bridge.yaml` for the compose equivalent.

### Healthcheck

The image calls `GET /healthz` on `ABACAD_LISTEN`'s port every 30 seconds. That probe is
plain HTTP, so a TLS deployment (`--tls-cert`/`--tls-key`) should run with
`--no-healthcheck`.

### Upgrading

```bash
docker compose pull && docker compose up -d
docker run --rm ghcr.io/abacad-ai/abacad:latest version
```

The `/config` volume is left alone, so devices survive. Tags are `latest`, `3`, `3.0`,
`3.0.0`; pin a version if you would rather upgrade on purpose.

3.0 dropped raw VNC ports and per-device tokens: every device is reached over the hub's
WebSocket, and the admin token is the only secret. An old config file still loads; the
dropped fields go away on the next save.

## The binary

Requirements: Go 1.27+ and Node 24 only if you build the frontend from source.

```bash
git clone https://github.com/abacad-ai/abacad.git
cd abacad
make build          # npm ci + vite build + go build
./bin/abacad serve
```

Or build the two halves by hand:

```bash
cd frontend && npm ci && npm run build && cd ..
go build -o abacad ./cmd/abacad
./abacad serve
```

On first run it creates `~/.config/abacad/config.yaml` and prints the URL and an admin
token.

```text
abacad [serve] [--config PATH] [--listen ADDR]
               [--tls-cert FILE --tls-key FILE] [-v]
abacad version
```

With no arguments, `abacad` serves. `serve` is accepted for explicit compose files.
`ABACAD_CONFIG`, `ABACAD_LISTEN`, and `ABACAD_ADMIN_TOKEN` set the same things from the
environment; flags win.

## TLS and the reverse proxy

Use HTTPS/WSS. Browser clipboard access in most browsers requires a secure context, and
the admin token should not cross a plaintext network you do not control.

The binary can terminate TLS itself:

```bash
./abacad serve --tls-cert cert.pem --tls-key key.pem
```

Use a certificate your browser trusts. Alternatively, terminate TLS at a reverse proxy.
Everything goes through the hub's single HTTP port, so one proxy covers the console, the
API, MCP, noVNC, and every device WebSocket.

Caddy does WebSocket upgrades automatically:

```text
abacad.example.com {
  reverse_proxy 127.0.0.1:8787
}
```

Nginx needs the upgrade headers:

```nginx
server {
  listen 443 ssl;
  server_name abacad.example.com;

  location / {
    proxy_pass http://127.0.0.1:8787;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
  }
}
```

If TLS ends at the proxy, leave the hub on plain HTTP and bind it to loopback or the
private interface. Do not publish port 8787 to the internet directly.

## The config file

The same file the web UI writes. It is YAML, mode `0600`, and the device passwords are in
it.

```yaml
listen: 0.0.0.0:8787
admin_token: 3f2c...
cookie_secret: 9a71...
devices:
  - id: mac-mini
    name: Mac mini
    host: 192.168.1.92
    port: 5900
    auth: ard              # ard = macOS account, vnc = VNC password
    username: iloahz
    password: "the account password"
    platform: macos
    mode: gateway          # gateway, l4, or direct
```

- `id` is generated from the name when you add a device in the UI. It is the path segment
  in `/ws/vnc/<id>`; keep it stable if an agent has saved the URL.
- `cookie_secret` signs the browser session cookie. Rotating it signs every browser out.
- `admin_token` is the only secret that opens the hub. Rotate it by editing the file (or
  `ABACAD_ADMIN_TOKEN`) and restarting.
- The generated token is 64 hex characters. A hand-written one must be at least 16
  characters.
- Hand edits are read at startup. The running hub writes the file when you change devices
  in the UI; restart after editing it yourself.

See [Configuration](/docs/operations/configuration/) for every field and mode.
