# Windows

Use **TigerVNC** in service mode. Service mode can show the Windows login screen,
which a user-session server cannot.

## Install

1. Download TigerVNC from <https://tigervnc.org/>.
2. Install it, and during setup enable **Register TigerVNC as a system service**.
3. Set a VNC password when prompted (or run `tvnserver` → **Configure** →
   **Security**).
4. The server listens on TCP **5900**.

## Firewall

Create an inbound rule for TCP 5900 and scope it to the hub's IP:

```powershell
New-NetFirewallRule -DisplayName "abacad VNC" -Direction Inbound `
  -Protocol TCP -LocalPort 5900 -RemoteAddress 192.168.1.10 -Action Allow
```

Do not create an any-source rule; do not forward 5900 on the router.

## Add it to the hub

In the hub web UI, click **Add device**:

- **Name:** `windows-pc`
- **Host:** the machine's LAN address, e.g. `192.168.1.40`
- **Port:** `5900`
- **VNC password:** the TigerVNC password
- **Platform:** Windows
- **Also open a raw VNC port for agents:** optional; leave it off for
  browser-only.

The card should turn **online**. If it stays offline, check the inbound firewall
rule and that TigerVNC is running as a service.
