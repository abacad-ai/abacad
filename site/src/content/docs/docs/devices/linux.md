---
title: Linux
description: Set up x11vnc, TigerVNC, or a Wayland VNC server for abacad, with a systemd unit, firewall rule, and the blacklist caveat.
---

Use the VNC server that matches your session type. X11 and Wayland are different
problems; `x11vnc` does not work on a Wayland session.

## X11: x11vnc

```bash
sudo apt install x11vnc
x11vnc -storepasswd                 # writes ~/.vnc/passwd
x11vnc -display :0 -rfbauth ~/.vnc/passwd \
  -forever -shared -rfbport 5900 -listen 0.0.0.0
```

A small systemd user unit:

```ini
# ~/.config/systemd/user/x11vnc.service
[Unit]
Description=x11vnc for abacad
After=graphical-session.target

[Service]
ExecStart=/usr/bin/x11vnc -display :0 -rfbauth %h/.vnc/passwd -forever -shared -rfbport 5900 -listen 0.0.0.0
Restart=on-failure

[Install]
WantedBy=default.target
```

```bash
systemctl --user enable --now x11vnc.service
```

`-shared` lets a second viewer in; without it, a second browser or agent connection may be
refused. If you want a single viewer, leave it off and connect one client at a time.

## X11: TigerVNC

TigerVNC's per-user config lives at `~/.config/tigervnc/config`:

```ini
localhost = no
SecurityTypes = VncAuth
geometry = 1600x900
depth = 24

# Optional hardening: raise the bar for source-IP blacklisting. This does not
# clear an existing lockout; restart the service after changing it.
BlacklistThreshold = 1000000
```

Run it as a system service:

```bash
sudo systemctl enable --now tigervncserver@:2.service   # RFB port 5902
```

TigerVNC counts every connection that closes before authenticating as a security failure.
Its failure counter does not decay, so repeated probes from any tool eventually blacklist
that IP and reject even correct clients with `Too many security failures`. Do not probe
the port with `nc`, a load balancer, a monitoring system, or a health check.

abacad itself never probes: it only connects when you open a browser session or an agent
opens the device WebSocket. To clear an existing lockout, restart the VNC server:

```bash
sudo systemctl restart tigervncserver@:2.service
```

## Wayland

- **wlroots (sway, etc.)**: `wayvnc 0.0.0.0 5900`, configured in
  `~/.config/wayvnc/config`.
- **KDE Plasma**: `krfb`, configure the port and password in its settings.
- **GNOME**: `gnome-remote-desktop` has a VNC backend:

  ```bash
  grdctl vnc set-auth-method password
  grdctl vnc set-password
  grdctl vnc disable-view-only
  grdctl vnc enable
  systemctl --user enable --now gnome-remote-desktop.service
  ```

  Some distributions package or enable the VNC backend differently; verify with
  `grdctl vnc --help`.

Wayland compositors differ in input support. `wayvnc` supports pointer and keyboard input
on wlroots compositors; `krfb` and `gnome-remote-desktop` have their own limitations.
Test clicking and typing before you rely on the machine for agent work.

## Firewall

Only the hub should reach the VNC port:

```bash
sudo ufw allow from 192.168.1.10 to any port 5900 proto tcp
```

Use the hub's IP. Do not open 5900 to the internet.

## Add it to the hub

In the console, click **Add device**:

- **Name:** `linux-box`
- **Host:** the machine's IP address, e.g. `192.168.1.30`
- **Port:** `5900` (or `5902` for the TigerVNC `:2` display)
- **Sign-in:** **VNC password**
- **Password:** the password from the server above
- **Platform:** Linux
- **Mode:** `gateway`

There is no health check to configure: abacad never probes the port. After saving, select
the machine once. The rail shows **online** when that session authenticates and stays
**not seen** until then.

## Troubleshooting

- **Connection refused.** Check the server is listening on an interface the hub can reach:
  `ss -ltnp | grep 5900`. `-listen 127.0.0.1` is only reachable from the same machine.
- **Black screen after connecting.** The server may be attached to the wrong display. For
  x11vnc, check `-display :0` matches the session; for a headless box, start an X server
  or use a virtual display.
- **Input does nothing.** On Wayland, the compositor may not implement remote input. Try
  the compositor's own VNC backend or switch the session to X11.
- **`Too many security failures`.** Restart the VNC server; the blacklist is in memory and
  does not decay.

Read [Security](/docs/operations/security/) before exposing the hub beyond your LAN.
