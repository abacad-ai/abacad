# MCP

abacad exposes exactly one MCP tool. That is not a limitation to work around; it
is the design. An agent that can drive VNC does not need a second screen API.

The hub serves MCP at `POST /mcp` using **Streamable HTTP** with JSON responses.
Sessions expire after 30 minutes of inactivity. It uses the same admin token as the web UI, passed as a Bearer token:

```
Authorization: Bearer <admin token>
```

## Connect a client

```bash
claude mcp add --transport http \
  --header "Authorization: Bearer <admin token>" \
  abacad http://192.168.1.10:8787/mcp
```

JSON client config:

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

If the hub is behind TLS, use `https://…/mcp`.

## `list_devices`

Input: none.

Output: a JSON object with a `devices` array:

```json
{
  "devices": [
    {
      "id": "mac-mini",
      "name": "Mac mini",
      "platform": "macos",
      "online": true,
      "status": "online",
      "mode": "gateway",
      "ws_url": "wss://abacad.example.com/ws/vnc/mac-mini",
      "auth": "none",
      "sign_in": "ard",
      "username": "li",
      "browser_url": "https://abacad.example.com/vnc/mac-mini"
    }
  ]
}
```

- `ws_url` is the device: a WebSocket carrying plain RFB, the websockify
  convention noVNC uses. It goes through the same HTTP port (and the same
  reverse proxy) as the web UI. Authenticate with the admin token you already
  use for MCP: `Authorization: Bearer <admin token>`, or `?token=<admin token>`
  for a client that only takes a URL.
- `auth` is what the VNC handshake *inside* the WebSocket asks for. `none`
  (gateway mode): the hub signs in to the device, the client sees no password.
  `device` (l4 mode, or direct): answer with the device's own VNC password.
- `direct` devices are catalog entries: no `ws_url`; `direct_host` and
  `direct_port` say where the VNC server is.
- `sign_in: "ard"` means the hub signs in to the device with a macOS account
  (Apple DH, type 30); `username` is that account. `sign_in: "vnc"` or an
  absent field means the hub uses the legacy VNC password. This is the device
  sign-in, separate from `auth`, which describes what the agent answers.
- `browser_url` is the same device in the hub's noVNC page, for a human.

## What the agent does next

1. Call `list_devices`.
2. Pick a device with a `ws_url`, preferably `online: true`.
3. Open the WebSocket with a WebSocket VNC client, or bridge it to a local TCP
   port and use any VNC client:

   ```bash
   websocat --binary -H="Authorization: Bearer <admin token>" \
     tcp-l:127.0.0.1:5900 wss://abacad.example.com/ws/vnc/mac-mini
   vncdotool -s 127.0.0.1::5900 capture screen.png
   ```

4. Drive it with the client's normal screenshot/pointer/keyboard operations.

There is no `observe`, `frame_id`, or `act` tool. Those belong to the VNC client
the agent already has. abacad only routes and authenticates.

## Clients

- **websocat + any VNC client** (vncdotool, TigerVNC, macOS Screen Sharing on
  `127.0.0.1`). Most robust: the client is a standard one.
- **noVNC** speaks WebSocket natively; it is what the hub's own page runs.
- **py-wsvnc** is a Python VNC client that dials `ws(s)://` directly. It assumes
  one RFB message per WebSocket frame and can lose sync on large updates.

Common desktop viewers (TigerVNC, RealVNC, Remmina) do not speak WebSocket; use
the bridge.

An agent that only has MCP tool calls, with no shell and no VNC client, would
need a separate adapter that exposes screenshot/click. That adapter would be an
RFB client, and it is the one thing this version intentionally does not put in
the core.

## Without MCP: the `/skill` page

Not every agent speaks MCP. Open **Skill for agents** in the hub (or go to
`/skill`) and the page generates a `SKILL.md`: the admin token, the machine table
with each `ws_url`, how to fetch the current list with
`GET /api/devices` and a Bearer header, and how to connect. The file is a
secret — it holds the admin token, which controls every machine.

The hub address field defaults to the page's own origin; set it if the agent
reaches the hub under a different name. Save the file as
`<agent>/skills/abacad/SKILL.md` (for example `~/.claude/skills/abacad/` or
`~/.dsh/skills/abacad/`).

## One secret

The hub has one credential, the admin token. The web UI exchanges it for a
session cookie at `POST /api/login`; everything else — the REST API, `/mcp` and
the device WebSockets — accepts it directly as `Authorization: Bearer`. The
WebSockets also accept `?token=`. `GET /api/token` returns it to a signed-in
browser so the `/skill` page can bake it into the file.
