---
title: Security
description: How abacad secures the connection between an agent and your device — the two-plane trust model, the controls in place today, an honest account of what authentication can and can't defend, and how to report a vulnerability.
---

abacad hands an agent real control of a real device, so the connection between them
has to be trustworthy. This page explains how abacad thinks about that trust, the
protections in place today, and — just as important — the one thing authentication
cannot solve.

## Two planes, opposite treatment

abacad splits traffic into two planes and treats them deliberately differently:

- **Control plane** — commands, screenshot metadata, and the UI tree. This is
  **server-mediated**: it flows through the relay, which routes each command to the
  right device. Mediating this plane is what makes human oversight possible — you
  can't govern what you can't see.
- **Tunnel / data plane** — the `/connect` TCP tunnel, the SSH jump, and file bytes.
  This is **end-to-end opaque**: the relay authorizes the connection once, then moves
  ciphertext it cannot read. An SSH or TLS session stays private end to end; the
  relay holds no session keys.

So: mediate the plane you must be able to oversee; stay blind to the plane you only
need to carry.

## What's in place today

- **Encryption is mandatory.** Clients require `wss://` + TLS; cleartext to a
  non-loopback host is refused. There is no "downgrade to plaintext" path.
- **Secrets stay out of URLs.** Agent and device tokens travel in the
  `Authorization` header, not the query string — so they don't leak through proxy
  access logs, `Referer` headers, or browser history. On macOS the token is stored
  in the system Keychain.
- **Each principal proves a distinct identity.** A human signs in to the dashboard
  (password, rate-limited with lockout); an agent presents a scoped bearer token,
  individually revocable; each device authenticates with its own enrolled token and
  can be revoked instantly; an SSH client authenticates by public key.
- **The SSH jump is not an open relay.** It authenticates the client's key to an
  account, pins its own host key in your `known_hosts`, and pins the target to the
  device's own `127.0.0.1:22` — it can never be steered to an arbitrary internal
  port. The inner SSH session is end-to-end encrypted; the relay moves ciphertext.
- **The tunnel can't become a pivot.** `/connect` targets are policed on the server
  and again on the device: link-local and cloud-metadata addresses
  (`169.254.0.0/16`, including `169.254.169.254`), the unspecified address, and
  multicast are denied. Your device's own services and LAN stay reachable — that's
  the point of the tunnel — but the pipe can't be aimed at places that are never a
  legitimate target.

## An integrity layer, not an approval layer

abacad deliberately does **not** decide whether an action is *safe*. It doesn't gate
individual taps or judge intent — because judging whether an action is dangerous
needs the task's intent, which lives in the **agent**, not in abacad (which only sees
a UI tree). Most agents already gate their own sensitive tool calls with a human in
the loop. abacad's job is the layer beneath that: authenticate the parties, carry the
bytes with integrity, and keep the human in ultimate control of enrollment and
revocation.

## The honest limit: prompt injection

There is one risk authentication **cannot** close: **prompt injection through
authentic screen content.** If a page or app the agent is driving contains text
crafted to hijack the agent, those bytes arrive with perfect integrity — from
abacad's point of view there is nothing wrong to detect. No amount of channel
encryption or endpoint authentication solves this, and we won't pretend otherwise.

It's defended, in order, by: the **agent's own judgment** (the primary line — keep a
human in the loop for sensitive actions), **device hygiene** (don't leave an
always-on automation phone logged into your bank), and your ability to **revoke a
device's access at any time**.

## Self-hosting

Because abacad runs as a single self-hostable server, you can keep the entire control
path inside your own infrastructure and trust boundary. A self-signed certificate is
fine for a LAN deployment.

## Reporting a vulnerability

Found a security issue? Please report it privately to **[security@abacad.ai](mailto:security@abacad.ai)**
rather than opening a public issue, so we can fix it before it's disclosed. Include
what the issue is and where, steps to reproduce or a proof of concept, and the impact
you think it has. We aim to acknowledge reports within a few business days and work
coordinated disclosure with good-faith researchers.
