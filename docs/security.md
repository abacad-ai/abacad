# Security

abacad is self-hosted. This document is deliberately short: it names the trust
boundary, the weak points, and the operations that keep the weak points from
mattering.

## The boundary

```
device VNC server  ──RFB──▶  hub  ──HTTPS/WSS──▶ browser
                               └──HTTPS/WSS──▶ agent
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
| One admin token for everything | Web login, API, MCP and every device WebSocket accept the same token, so whoever holds it controls every machine | 32 random bytes; keep generated `SKILL.md` files out of git; rotate it if a file leaks. `?token=` puts it in URLs, so keep proxy access logs private or use the `Authorization` header |
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

## One token

The admin token is the only credential the hub issues. The web UI trades it for
a session cookie; agents send it as `Authorization: Bearer` on the API, `/mcp`
and the device WebSockets at `/ws/vnc/<id>`, or as `?token=` for a WebSocket
client that takes only a URL. Inside the WebSocket, a `gateway` device asks for
no VNC password: the hub has already authenticated the caller and signs in to
the device itself. The real device password never leaves the hub.

The `/skill` page bakes the token into a `SKILL.md` for an agent (it reads it
from `GET /api/token`, which needs a signed-in session). That file is as
sensitive as the config: keep it out of git, and regenerate it after rotating
the token.

On macOS, the hub can authenticate with the **macOS account** (Apple DH, RFB
type 30) instead of a legacy VNC password. That stores a more capable
credential: the account password, not an 8-character VNC password. Prefer a
dedicated local account for remote control if you do not want to store your
main account's password. The config file remains `0600`; there is no OS
keychain yet.

## Operational checklist

- [ ] Hub listens on the address you chose, and that address sits behind the
      network controls you intend (VPN, firewall, reverse proxy).
- [ ] Device VNC ports accept connections only from the hub IP.
- [ ] Hub config is `0600` and on an encrypted disk.
- [ ] Admin token is stored in a password manager, not a shell history.
- [ ] The hub publishes one HTTP port; nothing else is forwarded to it.
- [ ] No process port-scans 5900; the hub connects only for a real browser or agent session.
- [ ] Hub is updated; no unrelated services share the host.
- [ ] You have decided how remote access works (a VPN or an authenticated
      tunnel), and it is not a VNC port on the internet.

## If something leaks

1. Rotate the admin token by editing `admin_token` in the config and restarting
   the hub.
2. Regenerate every `SKILL.md` you handed out.
3. Change the device's VNC password, or the macOS account password if the
   device uses `auth: ard`.
4. Remove the device from the hub if it is no longer trusted.
5. Check the hub's logs for connections you do not recognize.
