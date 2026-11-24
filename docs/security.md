# Security

abacad is self-hosted. This document is deliberately short: it names the trust
boundary, the weak points, and the operations that keep the weak points from
mattering.

## The boundary

```
device VNC server  ──RFB──▶  hub  ──HTTPS/WSS──▶ browser
                               └──RFB/TCP────▶ agent
```

- The hub is the only host that needs to reach every device. Scope each device's
  `5900/tcp` to the hub's IP at the firewall.
- The hub is the only ingress. abacad does not implement public forwarding,
  relaying or NAT traversal. How the hub is reached — a VPN, an authenticated
  tunnel, a reverse proxy — is your network decision, and it should be
  deliberate.
- The hub connects to any address you add. Making that path fast and private is
  the network's job; that direct path is also what keeps a relay out of the
  middle.

## What is weak

| Weak point | Why | Mitigation |
|---|---|---|
| Classic VNC auth | 8-character password, DES challenge; not strong by modern standards | Trusted path; firewall to the hub IP; a VPN for untrusted links; prefer VeNCrypt where the device supports it |
| 8-character device token | RFB VNC auth carries only 8 bytes | Random alphabet, rotatable; gateway rate-limits failed handshakes; treat it as a capability for your network path, not an internet secret |
| No TLS on hub → device | Many VNC servers do not speak TLS | Keep that leg on a trusted path; use VeNCrypt/TLS where available; use a VPN for untrusted links |
| Password at rest | v0.1 stores device passwords in `~/.config/abacad/config.yaml` | File is `0600`; full-disk encryption; a later version can use the OS keychain |
| Hub compromise | The hub can reach every device and holds their passwords | Patch the host, run as a non-root user, don't co-host unrelated services, back up the config securely |
| VNC blacklist | Some servers (notably TigerVNC) blacklist a source IP after repeated unauthenticated connections and never decay the counter | The hub never connects on its own; only a real browser or agent session touches the device. Do not point external probes or load balancers at port 5900. |

## Browser sessions

- Login exchanges the admin token for an HttpOnly, SameSite=Lax cookie. The
  cookie value is an HMAC of a random cookie secret, not the admin token.
- The admin token is 32 random bytes (64 hex characters). It is printed once at
  startup and stored in the config. Rotate it by editing `admin_token` in the
  config and restarting the hub.
- There is no CSRF token because the API is JSON-only and the cookie is
  SameSite=Lax. If you put the hub behind a reverse proxy, keep the same origin.

## Device tokens

Each device gets an 8-character token. It is the VNC password an agent uses when
connecting to the hub's raw port. The real device password never leaves the hub
in `gateway` mode.

On macOS, the hub can authenticate with the **macOS account** (Apple DH, RFB
type 30) instead of a legacy VNC password. That stores a more capable
credential: the account password, not an 8-character VNC password. Prefer a
dedicated local account for remote control if you do not want to store your
main account's password. The config file remains `0600`; there is no OS
keychain yet.

To rotate a token, edit `token:` in the config and restart the hub (or update the
device through the API in a later version). Removing the device closes its
listener immediately.

## Operational checklist

- [ ] Hub listens on the address you chose, and that address sits behind the
      network controls you intend (VPN, firewall, reverse proxy).
- [ ] Device VNC ports accept connections only from the hub IP.
- [ ] Hub config is `0600` and on an encrypted disk.
- [ ] Admin token is stored in a password manager, not a shell history.
- [ ] Raw VNC ports are opt-in per device; most devices can be browser-only.
- [ ] No process port-scans 5900; the hub connects only for a real browser or agent session.
- [ ] Hub is updated; no unrelated services share the host.
- [ ] You have decided how remote access works (a VPN or an authenticated
      tunnel), and it is not a raw VNC port on the internet.

## If something leaks

1. Rotate the admin token by editing `admin_token` in the config and restarting
   the hub.
2. Rotate the affected device token(s).
3. Change the device's VNC password, or the macOS account password if the
   device uses `auth: ard`.
4. Remove the device from the hub if it is no longer trusted.
5. Check the hub's logs for connections you do not recognize.
