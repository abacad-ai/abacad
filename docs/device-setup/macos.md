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

Verify with:

```bash
abacad doctor
```

You want to see `security: [1 2]` (or `[2]`). If you see `[30 33 35 36]`, the
checkbox is off, or this macOS version no longer offers legacy VNC auth. In that
case install **TigerVNC** and use its server, or accept that the Mac is not
reachable by abacad.

> Why not support Apple's default authentication? It needs your macOS account
> password (or Apple's SSO), and storing that in the hub is worse than storing a
> separate, revocable VNC password. Apple DH is also not standard RFB. See
> decision D15 in the plan.

## Add it to the hub

```bash
abacad add --name mac-mini --host 192.168.1.20 \
  --password 'the-password-you-just-set' \
  --platform macos --expose
```

- `--expose` gives agents a raw VNC port on the hub. Omit it for browser-only.
- The hub uses the password to authenticate upstream; the browser and the agent
  never see it.

## Keep it reachable

- Prevent sleep: **System Settings → Energy Saver → Prevent automatic sleeping
  when the display is off**, or run `caffeinate -s` while the hub needs it.
- If the Mac is on Wi-Fi, give it a DHCP reservation so `--host` stays valid.

## Known issues

- Apple's server is an RFB variant. Standard clients work, but Apple-specific
  authentication and display modes can be picky. Phase 0 of the rewrite tests
  this explicitly; if your macOS version refuses the standard handshake, install
  **TigerVNC** and use its server instead.
- Screen Sharing may show the login window after a reboot; controlling the
  pre-login screen is not guaranteed.
- Only one or a few viewers may be accepted depending on the macOS version and
  the "shared" flag. If an agent and a browser conflict, use one at a time.
- Apple's own Screen Sharing app prefers Apple's protocol when talking to
  another Mac. From abacad, use the browser/noVNC path or a standard VNC client.
