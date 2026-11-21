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
      "mode": "gateway",
      "vnc_host": "192.168.1.10",
      "vnc_port": 5901,
      "auth": "hub-token",
      "token": "Abcd1234",
      "browser_url": "http://192.168.1.10:8787/vnc/mac-mini",
      "banner": "RFB 003.008"
    }
  ]
}
```

- `vnc_host:vnc_port` + `token` is a **standard VNC endpoint**. Connect with any
  VNC client or library; use `token` as the VNC password.
- `auth: "hub-token"` means use `token`. `auth: "device"` means the hub is not
  terminating authentication (L4 or direct mode); use the device's own VNC
  password.
- `browser_url` is for a human. It is present even when `vnc_port` is absent.
- `warning` is present when the hub knows the device's authentication is not
  usable (for example, macOS Screen Sharing with only Apple-specific types).
- Devices without `vnc_port` are browser-only. Enable the raw VNC port in the
  device's edit dialog to give an agent a direct endpoint.

## What the agent does next

1. Call `list_devices`.
2. Pick a device with `vnc_port` and `online: true`.
3. Connect a VNC client to `vnc_host:vnc_port`, using `token` or the device
   password as indicated by `auth`.
4. Drive it with the client's normal screenshot/pointer/keyboard operations.

There is no `observe`, `frame_id`, or `act` tool. Those belong to the VNC client
the agent already has. abacad only routes and authenticates.

## If the agent has no VNC client

An agent with a shell can use any VNC CLI/library (for example `vncdotool` in
Python, or a Go VNC package). A future optional adapter may wrap one; it is
deliberately not part of the hub.

An agent that only has MCP tool calls, with no shell and no VNC client, would
need a separate adapter that exposes screenshot/click. That adapter would be an
RFB client, and it is the one thing this version intentionally does not put in
the core.
