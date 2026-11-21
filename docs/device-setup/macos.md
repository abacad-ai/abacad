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

## If the Mac is locked when you connect

The VNC password gets you to the Mac; the **lock screen wants the macOS account
password**, not the VNC password.

1. Click once inside the viewer.
2. Press **Space** or click the user avatar to reveal the password field.
3. Type the macOS account password and press Return.

If the keyboard does nothing, click **Keyboard** in the viewer's top bar to use
noVNC's on-screen keyboard.

### If the clock in the viewer is frozen

A frozen lock-screen clock (for example it still says 19:43 hours later) means
the Mac's **display is asleep**, not that the password field is missing. A
third-party VNC session may keep the last frame and may not be able to wake the
display. The Mac can still answer ping and accept TCP 5900 while this is true.

Wake it physically or with Apple's Screen Sharing app, then reconnect. To keep a
headless Mac awake, configure it properly:

```bash
# Never sleep the system or display; wake on magic packet.
sudo pmset -a sleep 0 displaysleep 0 womp 1
```

Or run `caffeinate -d -s` while the Mac should remain available. On a desktop,
System Settings → **Energy** also has **Prevent automatic sleeping when the
display is off**.

For a lock screen you never want to fight, System Settings → **Lock Screen** →
**Require password after screen saver begins or display is turned off** → Never.
That is a security trade-off; only do it on a machine you already trust on the
LAN.

### Screen Sharing vs Remote Management

If **Remote Management** is enabled, it owns the login window and may not accept
third-party VNC input there. For abacad, prefer:

- **Screen Sharing** on, **Remote Management** off;
- **Computer Settings… → VNC viewers may control screen with password** on.

Switching from Remote Management to Screen Sharing is a configuration change,
not a workaround: it removes Apple's managed login session from the path.

If macOS still refuses input at the login/lock screen, unlock once with Apple's
Screen Sharing app or physically, then use abacad for the logged-in session. We
do not recommend auto-login or disabling FileVault just to make this work.

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
