# macOS

macOS ships a VNC-compatible server with Screen Sharing. No extra software is
needed.

## Enable it

1. **System Settings → General → Sharing → Screen Sharing** (or **Remote
   Management**), turn it on.
2. Open the **ⓘ** next to Screen Sharing → **Computer Settings**.
3. Enable **VNC viewers may control screen with password** and set a password.
   **This checkbox is required.** Without it, modern macOS advertises only
   Apple-specific security types (`30 33 35 36`), which the hub does not speak.
4. Make sure the Mac's firewall allows Screen Sharing (System Settings →
   Network → Firewall → Options).

The server listens on TCP **5900**.

## Add it to the hub

In the hub web UI, click **Add device**:

- **Name:** `Mac mini`
- **Host:** the Mac's LAN address, e.g. `192.168.1.20`
- **Port:** `5900`
- **VNC password:** the password you just set
- **Platform:** macOS
- **Also open a raw VNC port for agents:** check it if an agent should connect
  with a standard VNC client; leave it off for browser-only.

The card should turn **online**. If it shows a warning about security types
`[30 33 35 36]`, the legacy VNC password checkbox is off, or this macOS version
no longer offers legacy VNC auth. In that case install **TigerVNC** and use its
server, or accept that the Mac is not reachable by abacad.

> Why not support Apple's default authentication? It needs your macOS account
> password (or Apple's SSO), and storing that in the hub is worse than storing a
> separate, revocable VNC password. Apple DH is also not standard RFB. See
> decision D15 in the plan.

## Keep it reachable

- Prevent sleep: **System Settings → Energy Saver → Prevent automatic sleeping
  when the display is off**, or run `caffeinate -s` while the hub needs it.
- If the Mac is on Wi-Fi, give it a DHCP reservation so the host field stays valid.

## Known issues

- Apple's server is an RFB variant. Standard clients work, but Apple-specific
  authentication and display modes can be picky. Tested against a real macOS Screen Sharing server; if your macOS version refuses the standard handshake, install
  **TigerVNC** and use its server instead.
- Screen Sharing may show the login window after a reboot; controlling the
  pre-login screen is not guaranteed.
- Only one or a few viewers may be accepted depending on the macOS version and
  the "shared" flag. If an agent and a browser conflict, use one at a time.
- Apple's own Screen Sharing app prefers Apple's protocol when talking to
  another Mac. From abacad, use the browser/noVNC path or a standard VNC client.
