---
title: Windows
description: Run TigerVNC in service mode on Windows so abacad can reach the login screen as well as the desktop, with a firewall rule scoped to the hub.
---

Use **TigerVNC** in service mode. Service mode can show the Windows login screen, which a
user-session server cannot.

## Install

1. Download TigerVNC from <https://tigervnc.org/>.
2. Install it, and during setup enable **Register TigerVNC as a system service**.
3. Set a VNC password when prompted (or run `tvnserver` → **Configure** → **Security**).
4. The server listens on TCP **5900**.

TigerVNC passwords are limited to eight characters by the RFB security type. Choose a
strong eight-character password; the network boundary is what protects the port, not the
password length.

## Firewall

Create an inbound rule for TCP 5900 and scope it to the hub's IP:

```powershell
New-NetFirewallRule -DisplayName "abacad VNC" -Direction Inbound `
  -Protocol TCP -LocalPort 5900 -RemoteAddress 192.168.1.10 -Action Allow
```

Do not create an any-source rule; do not forward 5900 on the router.

## Add it to the hub

In the console, click **Add device**:

- **Name:** `windows-pc`
- **Host:** the machine's IP address, e.g. `192.168.1.40`
- **Port:** `5900`
- **Sign-in:** **VNC password**
- **Password:** the TigerVNC password
- **Platform:** Windows
- **Mode:** `gateway`

Save, then select the machine. The first successful session marks it **online**.

## Troubleshooting

- **Stays offline after opening.** Check the inbound firewall rule and that TigerVNC is
  running as a service, not only in the tray.
- **Login screen, then a black screen.** The service may be running in a session that is
  not the console. Restart the TigerVNC service and reconnect.
- **`Too many security failures`.** TigerVNC blacklists an IP after repeated failed
  connections, and the counter does not decay. Restart the service to clear it, then stop
  anything that probes the port. See [Troubleshooting](/docs/operations/troubleshooting/).
- **Keyboard layout is wrong.** noVNC sends keysyms; set the viewer's keyboard layout in
  the noVNC control bar to match Windows.

Read [Security](/docs/operations/security/) before exposing the hub beyond your LAN.
