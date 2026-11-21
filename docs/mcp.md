# MCP

abacad exposes exactly one MCP tool. That is not a limitation to work around; it
is the design. An agent that can drive VNC does not need a second screen API.

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
- `mode: direct` devices have no hub endpoint; `direct_host`/`direct_port` are
  returned instead, and the agent authenticates to the device itself.
- Devices without `vnc_port` are browser-only. Enable `--expose` (or the
  "raw VNC port" checkbox) to give an agent a direct endpoint.

## Wiring it up

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

`--host` is what goes into `vnc_host` and `browser_url`. Use the hub's LAN
address, not `localhost`, unless the agent runs on the hub itself.

## What the agent does next

1. Call `list_devices`.
2. Pick a device with `vnc_port` and `online: true`.
3. Connect a VNC client to `vnc_host:vnc_port`, password `token`.
4. Drive it with the client's normal screenshot/pointer/keyboard operations.

There is no `observe`, `frame_id`, or `act` tool. Those belong to the VNC client
the agent already has. abacad only routes and authenticates.

## If the agent has no VNC client

An agent with a shell can use any VNC CLI/library (for example `vncdotool` in
Python, or a Go VNC package). A future optional `abacad` subcommand may wrap one;
it is deliberately not part of the hub.

An agent that only has MCP tool calls, with no shell and no VNC client, would
need a separate adapter that exposes screenshot/click. That adapter would be an
RFB client, and it is the one thing this version intentionally does not put in
the core.
