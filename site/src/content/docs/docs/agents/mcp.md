---
title: MCP
description: Connect an agent to abacad over Streamable HTTP at POST /mcp. The single list_devices tool returns each machine's WebSocket and auth requirements.
---

abacad exposes exactly one MCP tool. That is not a limitation to work around; it is the
design. An agent that can drive VNC does not need a second screen API.

The hub serves MCP at **`POST /mcp`** using **Streamable HTTP** with JSON responses.
Sessions expire after 30 minutes of inactivity. It uses the same admin token as the web
console, passed as a Bearer token:

```text
Authorization: Bearer <admin token>
```

## Connect a client

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

If the hub is behind TLS, use `https://…/mcp`.

## `list_devices`

Input: none.

```json
{
  "devices": [
    {
      "id": "mac-mini",
      "name": "Mac mini",
      "platform": "macos",
      "online": true,
      "known": true,
      "status": "online",
      "mode": "gateway",
      "ws_url": "ws://192.168.1.10:8787/ws/vnc/mac-mini",
      "auth": "none",
      "sign_in": "ard",
      "username": "iloahz",
      "browser_url": "http://192.168.1.10:8787/vnc/mac-mini"
    }
  ]
}
```

Fields worth knowing:

- `ws_url` — the WebSocket carrying plain RFB. Empty for a `direct` device.
- `auth` — `none` when the hub signs in (gateway), `device` when the client must answer
  the device's own VNC password (l4), and `device` plus `direct_host`/`direct_port` for a
  catalog-only direct device.
- `sign_in` — `vnc` or `ard` (macOS account). This is what the hub used upstream.
- `status` — `online`, `offline`, or `unknown` (never seen). The state goes stale after
  ten minutes; it is not a live probe.
- `last_error` — the last failure, when there is one.

## Open the stream

The agent opens `ws_url` with the same admin token, either as an `Authorization: Bearer`
header or as `?token=` for clients that take only a URL:

```text
ws://192.168.1.10:8787/ws/vnc/mac-mini
Authorization: Bearer <admin token>
```

With a WebSocket VNC client, that is the whole integration. With a client that only speaks
TCP, bridge it to a local port first:

```bash
websocat --binary -H="Authorization: Bearer <admin token>" \
  tcp-l:127.0.0.1:5900 ws://192.168.1.10:8787/ws/vnc/mac-mini &

vncdotool -s 127.0.0.1::5900 capture screen.png
```

Then use any VNC client on `127.0.0.1:5900` — `vncdotool`, TigerVNC, noVNC, a screenshot
tool, or the agent's own VNC implementation.

## No MCP? Use the skill

An agent that cannot speak MCP can still use the hub. The **Skill** page generates a
ready-made `SKILL.md` with the hub address, the admin token, and the machine list baked
in. It is a plain file; there is no server-side integration. See
[the agent skill](/docs/agents/skill/).

## Notes

- **One token opens every machine.** There are no per-device tokens in 3.0. Treat the
  token as the password to the whole hub.
- **The hub is not an approval layer.** In `gateway` mode it signs in to the device; it
  does not ask a human to approve each action. If you need that, put the agent behind your
  own policy.
- **One session per viewer.** Some VNC servers accept multiple viewers, some do not. If a
  browser and an agent conflict, use one at a time.
- **The WebSocket is RFB, not a new protocol.** The same stream works with noVNC and with
  any standard VNC client. See [WebSocket and VNC](/docs/agents/websocket/).
