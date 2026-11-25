---
title: Troubleshooting
description: "Fix the common abacad failures: a device that stays not seen, authentication errors, TigerVNC blacklists, proxy and WebSocket problems, and stale iOS sessions."
---

Start with the hub's logs. Run the container with `-v` (or the binary with `-v`) and watch
what happens when you open the device. Most failures are one of five things: the hub
cannot reach the host, the port is closed, the credential is wrong, the VNC server chose a
different session, or a proxy is not upgrading the WebSocket.

## The device stays "not seen"

**not seen** means no authenticated session has ever reached the device. abacad does not
probe, so this is not a health check result. Check, in order:

1. **Is the host reachable from the hub?** Not from your laptop — from the hub. The host
   field is resolved by the hub.
2. **Is the VNC server listening on an interface the hub can reach?** On the device:
   `ss -ltnp | grep 5900` (Linux) or `lsof -nP -iTCP:5900` (macOS). `127.0.0.1` is not
   reachable from the hub.
3. **Is the firewall scoped correctly?** Allow the hub's IP, not "any".
4. **Is the mode right?** A `direct` device is catalog-only and has no browser session.
5. **Did the session authenticate?** Open it once and watch the logs; the device only turns
   online after a successful handshake.

Do not test a TigerVNC port with `nc`, a load balancer, or a monitoring probe. A TCP
connection that closes before authenticating counts as a security failure.

## Authentication failed

- **VNC password devices:** re-enter the password. Classic VNC passwords are limited to
  eight significant characters; a longer password is truncated, not rejected, so a typo in
  the first eight is the usual cause.
- **macOS account:** use the account's short name (`whoami`), not the display name. Use the
  login password, not the legacy VNC password. If the Mac has **Remote Management** on, the
  account must be in its **Allow access for** list with Observe/Control.
- **`Authentication or authorization failure` on macOS:** the DH exchange succeeded and the
  Mac refused the account. Check Remote Management's access list, not the password.
- **`macOS: switch Sign-in to "macOS account"`:** the device is still configured for the
  legacy VNC password path. Edit it and switch.

## `Too many security failures`

TigerVNC blacklists a client IP after repeated connections that close before
authenticating, and the counter does not decay. The fix is to restart the VNC server, which
clears the in-memory counter:

```bash
sudo systemctl restart tigervncserver@:2.service   # Linux
# or restart the TigerVNC service on Windows
```

Raising `BlacklistThreshold` alone does not clear an existing lockout. After the restart,
stop whatever is probing the port — including monitoring, a load balancer, or a browser
tab that reconnects in a loop.

## The console will not load

- **401 / redirected to the login page:** the admin token is wrong or the session cookie
  expired. Sign in again.
- **The page loads but sessions fail:** check the browser console for a WebSocket error.
  A proxy that does not forward `Upgrade`/`Connection` will break `/ws`.
- **Clipboard does not work:** most browsers require HTTPS for clipboard access. Put the
  hub behind TLS.
- **The hub is unreachable:** check `--listen` and the port mapping. With Docker, host
  networking is recommended on Linux; with bridge networking, publish `8787`.

## A proxy in front

The whole hub is one HTTP port, so one reverse proxy is enough — but it must upgrade
WebSockets and not buffer the stream.

- Caddy does this automatically with `reverse_proxy`.
- Nginx needs `proxy_http_version 1.1`, `Upgrade`, and `Connection "upgrade"`.
- Do not put a buffering body limit in front of `/ws` or `/mcp`.
- If TLS ends at the proxy, the hub can stay on plain HTTP on loopback.

See [TLS and the reverse proxy](/docs/install/#tls-and-the-reverse-proxy).

## A black screen or a frozen screen

- **macOS:** a frozen picture is usually display sleep or the `loginwindow` session. Confirm
  the device uses **macOS account** sign-in, then wake the Mac or keep it awake with
  `caffeinate -d -s`. See [macOS](/docs/devices/macos/).
- **Linux:** the server may be attached to a different display. For x11vnc, `-display :0`
  must match the session; a headless box needs an X server or a virtual display.
- **Wayland:** the compositor may not implement remote input; try its own VNC backend.
- **iOS:** a frozen frame means the bridge session ended. It is never shown as live; restart
  the bridge and wait 15–30 seconds for a keyframe. See [iOS](/docs/devices/ios/).

## An agent cannot connect

- Use the exact `ws_url` from `list_devices`; the device id is case-sensitive.
- Send the admin token as `Authorization: Bearer`, or `?token=` if the client cannot set
  headers. Prefer the header.
- In `l4` mode the client must answer the device's own VNC password; in `gateway` mode it
  must not expect a password challenge.
- If the client only speaks TCP, bridge the WebSocket with `websocat` first. See
  [WebSocket and VNC](/docs/agents/websocket/).

## Docker and TLS

- The image healthcheck calls plain HTTP `/healthz`. With `--tls-cert`/`--tls-key`, run
  with `--no-healthcheck`.
- On macOS or Windows Docker Desktop, host networking is limited; use bridge networking and
  publish 8787.
- The image runs as uid 10001. A bind-mounted config directory must be writable by that
  uid: `chown -R 10001 ./config`.

## Reset without losing the devices

Stop the hub, edit `config.yaml`, and restart. To rotate the admin token, change
`admin_token` (or set `ABACAD_ADMIN_TOKEN`) and restart; every browser session, MCP client,
and saved skill stops working. To keep devices but start fresh browser sessions, change
`cookie_secret` only.

If you have lost the token, read it from the file:

```bash
# native
sed -n 's/^admin_token: //p' ~/.config/abacad/config.yaml

# Docker
docker exec abacad cat /config/config.yaml
```
