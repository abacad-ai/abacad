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

```bash
abacad add --name windows-pc --host 192.168.1.40 \
  --password 'the-vnc-password' --platform windows --expose
```

## Notes

- If the machine is on Wi-Fi, reserve its DHCP address.
- TigerVNC's service mode and the user-session mode both work; service mode is
  better for a headless/always-on machine.
- Windows may sleep; disable sleep for the power profile used by the device.
