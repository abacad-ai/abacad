---
title: WebSocket and VNC
description: Every device is a websockify-style WebSocket at /ws/vnc/<id> carrying plain RFB. Use it from a browser, an agent, or any VNC client through a websocat bridge.
---

The hub has one way into a device: a WebSocket per device at **`/ws/vnc/<id>`** carrying
plain RFB. This is the websockify convention noVNC uses. Nothing about the bytes is
abacad-specific, so any VNC client that can reach a WebSocket — or a local TCP port — can
use it.

## The endpoint

```text
ws://<hub>/ws/vnc/<device id>
wss://<hub>/ws/vnc/<device id>
```

The device id is the one in the rail and in `list_devices`. Authenticate with the admin
token:

```text
Authorization: Bearer <admin token>
```

Clients that cannot set headers may use a query parameter instead:

```text
wss://<hub>/ws/vnc/mac-mini?token=<admin token>
```

Prefer the header. A token in a URL ends up in proxy logs, shell history, and browser
history.

## What the client sees

The handshake depends on the device's mode:

| Mode | The client's RFB handshake |
| --- | --- |
| `gateway` (default) | The hub signs in to the device upstream and presents a **None** handshake to the client. The client sees no VNC password. |
| `l4` | The hub is a blind pipe. The client answers the device's own VNC password. |
| `direct` | No WebSocket is served. The client connects to the device directly. |

In `gateway` mode the hub terminates only the RFB handshake and then copies bytes in both
directions. It never decodes a framebuffer and never synthesizes input.

## Bridge it to a local port

For a client that only speaks TCP, use `websocat`:

```bash
websocat --binary \
  -H="Authorization: Bearer <admin token>" \
  tcp-l:127.0.0.1:5900 \
  ws://192.168.1.10:8787/ws/vnc/mac-mini
```

Now point any VNC client at `127.0.0.1:5900`. If the device is in `l4` mode, the client
will ask for that device's VNC password; in `gateway` mode it will not ask for one.

## In a browser

The console opens the vendored noVNC application in an iframe. The same viewer is available
directly at:

```text
http://<hub>/vnc/<device id>
```

That route requires a signed-in browser session. The WebSocket itself accepts the admin
token, which is what an agent uses.

## Notes

- **One WebSocket is one session.** Open one per viewer. Some VNC servers accept multiple
  viewers; others do not.
- **No recording, no OCR, no input synthesis.** The hub is a pipe. If you need a screenshot
  or a click, the VNC client does that.
- **The online state is not this connection.** A successful authenticated session marks
  the device online for ten minutes; the hub does not probe to keep it fresh.
- **TLS matters.** On an `https` hub the WebSocket is `wss`, and the admin token does not
  cross the network in clear text. See [TLS and the reverse proxy](/docs/install/#tls-and-the-reverse-proxy).
