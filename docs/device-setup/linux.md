# Linux

Use the VNC server that matches your session type.

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

## X11: TigerVNC (systemd)

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

TigerVNC counts every connection that closes before authenticating as a
security failure. Its failure counter does not decay, so a 30-second TCP
health probe from the hub eventually blacklists the hub's IP and rejects even
correct clients with `Too many security failures`. Do not probe the port with
`nc`, a load balancer, or a monitoring system.

For the hub, prefer an SSH health command (see below) over `health: vnc`.
To clear an existing lockout, restart the VNC server:

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

## Firewall

Only the hub should reach 5900:

```bash
sudo ufw allow from 192.168.1.10 to any port 5900 proto tcp
```

(Use the hub's LAN IP. Do not open 5900 to the internet.)

## Add it to the hub

In the hub web UI, click **Add device**:

- **Name:** `linux-box`
- **Host:** the machine's LAN address, e.g. `192.168.1.30`
- **Port:** `5900`
- **VNC password:** the password from the server above
- **Platform:** Linux
- **Also open a raw VNC port for agents:** optional; leave it off for
  browser-only.
- **Health check:** **Run a command on the hub**, for example
  `ssh -o BatchMode=yes debian 'systemctl is-active tigervncserver@:2'`.
  This avoids touching the VNC port at all.

The card should turn **online**. If it stays offline, check that the VNC server
is listening on the LAN interface, that the hub can run the health command
non-interactively, and that the firewall allows the hub's IP.
