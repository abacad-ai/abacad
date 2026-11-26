---
title: Security
description: "The abacad threat model: one admin token opens the hub, the network is the boundary, device passwords live in config.yaml, and the hub is a single ingress to defend."
---

abacad is a gateway to machines you control. Its security model is deliberately small
enough to explain in one page.

## The shape of the risk

- **One admin token opens everything.** The web login, the API, MCP, and every device
  WebSocket use the same token. There are no per-device tokens in 3.0.
- **The hub can reach every device.** In `gateway` mode it holds the device passwords and
  signs in on the client's behalf.
- **The hub is a single ingress.** If it is compromised, the attacker is on the same
  network path as the hub, with its credentials. Treat it as a high-value host.

That is why the network boundary matters more than any single feature.

## The network is the boundary

- **Do not publish the hub to the internet directly.** Put it on a private network, a VPN,
  or at minimum behind TLS and a reverse proxy you control.
- **Bind to the interface you mean.** `--listen 127.0.0.1:8787` behind a local proxy, or a
  private LAN address. The default `0.0.0.0:8787` listens on every interface.
- **Restrict each device's VNC port to the hub's IP.** The hub is the only machine that
  needs to reach 5900 (or 5902 for TigerVNC's `:2` display, 5970 for the iOS VNC Bridge).
  Do not open those ports to the internet, and do not forward them to the hub host.
- **Use TLS for the browser.** Clipboard access in most browsers requires a secure context,
  and the admin token should not cross a network you do not control in clear text.
- **Keep the hub patched.** It is one binary with a small dependency surface; updating is
  `docker compose pull && docker compose up -d`.

See [TLS and the reverse proxy](/docs/install/#tls-and-the-reverse-proxy) for a Caddy and
Nginx example.

## Credentials

- **The admin token** is 32 random bytes (64 hex characters). It is generated on first run,
  printed once, and stored in `config.yaml` unless supplied as `ABACAD_ADMIN_TOKEN`.
  Rotate it by editing the file (or the environment) and restarting. Every browser session,
  agent, and saved skill stops working immediately.
- **Device passwords** live in `config.yaml`, which is written `0600`. In `gateway` mode the
  hub needs them; in `l4` mode it does not store a device password, because the client
  answers the device's own challenge.
- **The agent skill contains the admin token.** Treat the generated `SKILL.md` as a
  password. Keep it out of git and shared drives.
- **Browser sessions** use an HttpOnly, SameSite=Lax cookie derived from `cookie_secret`.
  Rotating the secret signs every browser out.

If storing a macOS account password in the hub is not acceptable, create a dedicated local
macOS account for remote control and use that. It is the same trust model as storing a VNC
password, with a more capable credential.

## What abacad does not do

- **No per-device tokens or roles.** One token opens all devices and the API. If you need
  scoped access, run separate hubs or put an authenticating proxy in front.
- **No human approval per action.** In `gateway` mode the hub signs in and then pipes bytes.
  It does not inspect or approve individual clicks or keystrokes.
- **No audit log or recording.** The hub does not decode the framebuffer or keep a session
  history.
- **No discovery or probing.** It never scans the network and never opens a connection to a
  device on its own. A device only goes online after a real authenticated session.
- **No NAT traversal or relay.** You own the path to the hub.

## A short checklist

1. Hub on a trusted network; not exposed directly to the internet.
2. TLS for anything that leaves the LAN.
3. VNC ports firewalled to the hub's IP only.
4. Admin token from a secret store, not a short human-chosen string.
5. A backup of `config.yaml` (it contains the token and device passwords).
6. A plan to rotate the token if a skill file or laptop is lost.

## Modes and their trade-off

| Mode | Credential stored in hub | Client sees a password | Use when |
| --- | --- | --- | --- |
| `gateway` | Yes | No | You want the browser and agents to work without handing them device passwords. |
| `l4` | No | Yes, the device's own | You do not want device passwords on the hub and your clients can answer the challenge. |
| `direct` | No | N/A (the client connects to the device) | The hub is only a catalog; you do not need the browser console. |

In every mode, the admin token still controls the hub itself.
