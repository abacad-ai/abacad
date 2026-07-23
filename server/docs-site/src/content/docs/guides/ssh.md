---
title: SSH access
description: Reach a device's own sshd behind NAT with stock ssh and nothing installed on the connecting machine — ssh <device>.abacad.ai, via a ProxyJump jump host. Works from phones, CI runners, and locked-down networks.
---

How to reach a device's own `sshd` with **stock `ssh` and nothing installed on the
connecting machine** — `ssh <device>.abacad.ai`, just like any host.

## The idea

A device sits behind NAT and only ever **dials out** (the `/device` WebSocket). There is
no inbound route to it. The relay makes it reachable anyway: the device's `sshd` becomes
addressable as `<device>.<base-domain>`, and a normal SSH client reaches it through a
**jump host** the relay runs.

:::tip[The connecting end installs nothing]
No agent, no VPN, no `ProxyCommand` helper binary — just OpenSSH and one config block.
This is what makes SSH work from phones (Blink, Termius, Termux) and locked-down /
managed environments (CI runners, MDM'd laptops, cloud shells) where you can write
`~/.ssh/config` but can't install software.
:::

```sshconfig
# ~/.ssh/config — once, then `ssh <device>.abacad.ai` works forever
Host *.abacad.ai
    ProxyJump abacad.ai
```

## Why `ProxyJump` (and not a ProxyCommand or a password)

To route `<device>.abacad.ai` to the right device, the relay needs to learn *which*
device the client wants. A **bare SSH connection carries no hostname** — SSH has no SNI
and no `Host` header, so the target subdomain the client typed is resolved to an IP and
then discarded. A relay fronting many devices on one `:22` can't tell them apart.

`ProxyJump` (`ssh -J`, which expands to `ssh -W <host>:<port> <jump>`) is the escape
hatch: it asks the jump to open a `direct-tcpip` channel to `<device>.abacad.ai:22`, and
**that target host string travels in-band** (RFC 4254 §7.2). The jump reads it, maps it
to a device, and bridges. That's the hook a plain connection lacks.

Two alternatives were rejected:

- **`ProxyCommand <helper>`** — works, but needs a helper executable on every client.
  Dead on sandboxed mobile clients and anywhere you can't install binaries. `ProxyJump`
  needs no local executable — only config — so it works there.
- **Password auth** — OpenSSH has *no* way to put a password in `ssh_config`, and no way
  to supply one non-interactively without `sshpass`/askpass (an install). So a password
  always prompts. Public keys are presented automatically and never prompt, which is why
  they're the only method that meets "no prompt, nothing installed."

## How it works

```
  ssh <device>.abacad.ai
        │  (ProxyJump abacad.ai)
        ▼
  ┌───────────── abacad relay ─────────────┐
  │  SSH jump host                          │
  │   1. pubkey auth ──▶ account            │        WebSocket (/device)
  │   2. read direct-tcpip target           │◀───────────────────────────  device
  │      "<device>.abacad.ai:22"            │                               (dials out)
  │   3. authorize: account owns device?    │
  │   4. open stream to device 127.0.0.1:22 ┼──▶ device dials its own sshd ──▶ 127.0.0.1:22
  └─────────────────────────────────────────┘
        ▲
        └── the inner SSH session (client ⇄ device sshd) is end-to-end encrypted;
            the relay moves ciphertext and never holds the session keys.
```

1. **Authenticate** — the client's public key identifies the **account**. An unregistered
   key is rejected at the jump before any channel opens.
2. **Route** — the `direct-tcpip` target `<device>.abacad.ai` maps to a device id. The
   jump only routes to a device **that account owns and that is online**.
3. **Bridge** — the channel is spliced onto a relay stream opened to the device, which
   dials its **own `127.0.0.1:22`**. The port is pinned to sshd — the jump can never be
   used to reach arbitrary internal ports.

## Security model

- **Two independent checks, one key.** The jump authenticates you to your *account*
  (authorization: which devices may you reach). The device's own `sshd` does the real
  *login* (authentication). They can use the same SSH key; the jump never sees the login.
- **End-to-end encrypted, relay holds no keys.** The jump moves the inner SSH session as
  opaque bytes. It has a host key (so clients get a stable `known_hosts` entry for
  `abacad.ai`) but is *not* a man-in-the-middle of your device session.
- **Not an open relay.** Routing is scoped to devices the authenticated account owns, and
  the device-side target is fixed to `127.0.0.1:22`.
- **Public keys aren't secrets** — they're stored in full and indexed by SHA256
  fingerprint; there's no reveal-once flow.

## Server setup

The jump is **opt-in**. Enable it with a listen address (or several):

| Env / flag | Meaning | Example |
|---|---|---|
| `ABACAD_SSH_ADDR` / `-ssh-addr` | Listen address(es), comma-separated. Empty = disabled. | `:22,:443` |
| `ABACAD_SSH_HOST_KEY` / `-ssh-host-key` | Persistent host key path (created if absent). | `/data/ssh_host_ed25519_key` |
| `ABACAD_BASE_DOMAIN` / `-base-domain` | Domain devices hang off. | `abacad.ai` |

- **DNS:** point `abacad.ai` (the jump + dashboard) at the host. With `ProxyJump`, the
  `<device>.abacad.ai` names are sent to the jump *literally* and never resolved by the
  client, so they need no DNS records — the wildcard is optional.
- **Ports:** run on **`:22` and `:443`**. Many locked-down networks block outbound `:22`
  but allow `:443`; stock `ssh` speaks SSH-the-protocol over 443 fine
  (`ProxyJump abacad.ai:443`). In Docker the container is non-root and can't bind `:22`
  directly — bind `:2222` inside and publish it:
  `docker run -p 22:2222 -p 443:2222 -e ABACAD_SSH_ADDR=:2222 …`.
- **Host key:** keep it on the data volume so `known_hosts` stays valid across restarts.

## User setup

1. **Register a public key** — dashboard → Settings → *SSH access keys* (or
   `POST /api/ssh-keys {"name","public_key"}`). Paste the contents of your `.pub` file.
2. **Add one config block** (shown in the dashboard with your real values):
   ```sshconfig
   Host *.abacad.ai
       ProxyJump abacad.ai        # or abacad.ai:443 on restricted networks
   ```
3. **Connect** — `ssh <you>@<device>.abacad.ai`. `scp`, `rsync`, `git`, etc. work too;
   it's a raw TCP tunnel. The device's ssh hostname is shown per-device in the dashboard.

## Status

- **Implemented:** pubkey→account auth, account-scoped routing, target pinned to
  `127.0.0.1:22`, persistent ed25519 host key, multi-address listen, dashboard UI +
  `/api/ssh-keys` CRUD. Verified end-to-end with the system `ssh` CLI.
- **Deferred:** short-lived SSH certificates (vs. registered keys), per-key device scoping
  (today a key reaches all of the account's devices), and regional jump hosts for latency.
