# Trust model: who proves what, to whom

The chain runs **human → server → device**, with agents hanging off the server and
the channel itself in the middle. This doc names every identity in that chain and
the exact thing each one proves before it's trusted. It's the companion to
[`transport.md`](transport.md) (how bytes move) and [`ssh.md`](ssh.md) (the jump
host) — those describe the pipes; this describes who's allowed on them.

Everything here follows from two rules. Read those first; the rest is bookkeeping.

---

## Two principles

### 1. Mediate what you govern; blind what you don't

The system has two planes (see [`transport.md`](transport.md)), and they get
**opposite** trust treatment on purpose:

- **Control plane** (commands, screenshot metadata, the UI tree) is
  **server-mediated and authenticated, but not end-to-end encrypted.** The server
  can see every control frame — deliberately, because the audit log and the kill
  switch *need* it to. You cannot govern what you cannot see.
- **Tunnel / data plane** (`/connect`, the SSH jump, file bytes) is
  **end-to-end opaque.** The server authorizes once, at connect time, then moves
  ciphertext it can't read. An SSH or TLS session stays private end to end; the
  relay holds no session keys.

So the answer to "does the relay see my traffic?" is *yes for control, no for
tunnels* — and that split is a feature, not an inconsistency. Mediate the plane you
must be able to log and cut; blind the plane you only need to carry.

### 2. Non-escalation

> Any credential a **client** holds — a device key, an MCP token, an SSH key — can
> only **exercise** access within limits set by the account owner through an
> **authenticated dashboard session.** No client credential can widen its own scope
> or change another's. Scope changes require the human session, never a token.

This is the property that makes a **leaked or prompt-injected token bounded**
instead of fatal. The token holder can act; it cannot promote itself.

---

## Principals

| Principal | Identity | Proves it by |
|---|---|---|
| **Account owner** (human) | account | password (bcrypt) + session cookie; MFA recommended |
| **Dashboard session** | derived from the account | opaque session id cookie — `HttpOnly`, `Secure`, `SameSite` |
| **Agent** (MCP client) | scoped capability token | bearer token in the `Authorization` header (never in a URL) |
| **Device** (phone / Mac) | hardware-backed keypair | mutual TLS / signed challenge; its public key is enrolled to the account |
| **SSH client** (a terminal) | SSH keypair | public key → account; pins the jump's host key |
| **Server** | server identity (cert + pinned public key) | TLS — a public CA for browsers, a **pin** for device clients |

**Trust anchors.** The human account is the root of everything *mutable* (arming a
credential's scope, minting tokens, enrolling or revoking a device). The pinned
server identity is the root of *channel* trust for a device. The hardware keypair
is the root of *device* identity. The first is bootstrapped by password + MFA; the
other two are bootstrapped together at enrollment.

---

## The auth chain

```
   human ──①──▶ dashboard ──▶ server ◀── agent ──②── (MCP / connect)
                                  ║
                                  ║ ③  MUTUAL: device pins the server's identity,
                                  ║     server verifies the device's key
                                  ▼
                               device
                                  ▲
   ssh client ──④── jump host ───╜   (pubkey → account; host key pinned;
                                       target pinned to the device's 127.0.0.1:22)
```

### ① Human ⇄ server — the dashboard (the crown jewel)

This session is the root of every *mutable* thing, so it earns the strongest human
auth.

- **Human → server:** password (bcrypt), rate-limited with lockout, **TOTP MFA**
  (optional but available). Compromise here lets an attacker mint tokens and enroll
  devices, so MFA buys the most here of anywhere.
- **Server → human:** TLS server cert, validated by a public CA in the browser.
- **The cookie:** `HttpOnly`, `Secure` (unconditionally — not "if we detect
  HTTPS"), `SameSite`, rotated on privilege change, with an idle timeout under the
  long absolute TTL, and a "revoke all sessions" control.

### ② Agent ⇄ server — MCP and `/connect`

An MCP token is a **capability grant, not a master key.**

- `{ which devices, which capabilities, expiry }`, with **multiple named tokens**
  per account, each independently revocable.
- **Header-only.** Send it as `Authorization: Bearer …`. `/connect` must stop
  accepting `?token=` — a secret in a URL leaks through reverse-proxy access logs,
  `Referer` headers, and history. (The app itself already logs path-only, but it
  can't control a fronting proxy.)
- A token *uses* its scope; it can never *change* it (principle 2).

### ③ Device ⇄ server — the mutual-auth core

This is the edge that carries the whole product: the device is an **actuator** — it
executes whatever its socket peer sends. So authenticating *the peer* is not
optional politeness, it's the main event. A device that proves itself but never
checks who's giving it orders will faithfully take orders from an impostor.

- **wss + TLS 1.3, mandatory.** No cleartext, no `ws://` to a non-loopback host.
- **Server → device by pinning.** The device pins the server's public key (stronger
  than plain CA validation) and refuses anything else. This defeats a rogue-AP MITM
  *and* a mis-issued or rogue-CA certificate — which matters precisely because a
  drawer phone is unattended and has no human to notice a warning. A robot can't be
  socially engineered, but it also can't be suspicious, so the server's identity
  has to be baked in.
- **Device → server by hardware keypair.** At enrollment the device generates a
  keypair in the Android Keystore / macOS Secure Enclave; the private key never
  leaves hardware. It authenticates by proving possession (mutual-TLS client cert
  or a signed challenge). **Nothing secret is ever transmitted or stored in
  prefs** — so there is nothing to harvest from the wire, from `logcat`, or from a
  stolen backup. Revocation is deleting the public key.

> This is the half that a stock SSH setup already gets right (edge ④ below pins the
> jump's host key). The fix for edge ③ is to carry that same idea — a pinned server
> identity — to the device socket.

### ④ SSH client ⇄ jump ⇄ device

Already correct; keep it. See [`ssh.md`](ssh.md).

- **Client → jump:** public key → account (`AccountBySSHKeyFingerprint`). An
  unregistered key is rejected before any channel opens.
- **Jump → client:** the jump's host key is pinned in the client's `known_hosts` —
  real server-to-client authentication.
- **Jump → device:** rides edge ③'s mutually-authenticated channel, with the target
  pinned to the device's own `127.0.0.1:22`.
- The inner SSH session is end-to-end; the relay moves ciphertext and holds no keys.

### ⑤ The two planes, restated as trust

- **Control plane:** server-mediated, per-hop mutually authenticated and encrypted.
  *Not* end-to-end — so the audit log and kill switch work.
- **Tunnel plane:** end-to-end opaque, authorized once at connect time. The tunnel
  **target** is policed as a channel-integrity matter (see below), not because the
  server reads the bytes — it can't.

---

## Enrollment: bootstrapping trust

Edge ③ has a chicken-and-egg: the device must know the server's identity *before*
it can trust the connection. Solve it by delivering the pin out-of-band, through the
already-authenticated dashboard.

```
1. Human signs in to the dashboard         (password + MFA, over CA-validated TLS)
2. "Add device" → server mints a one-time  (short TTL, single-use)
   enrollment code and renders a QR:
       { wss endpoint, server pubkey pin, enrollment code }
3. Phone scans the QR (in person)          learns the server's pinned identity + code
4. Phone generates a keypair in hardware
5. Phone connects over wss, VALIDATES the  submits { code, device pubkey, (attestation?) }
   server against the pin, then:
6. Server checks the code, binds           code is consumed
   device pubkey → account
7. Thereafter the device authenticates by its key. No shared token ever exists.
```

Why this is a strong bootstrap: the human is authenticated over trusted TLS, the QR
crosses an **in-person, out-of-band** channel, and the code binds *this* key to
*this* account exactly once. Two consequences worth stating:

- **A malicious QR can't hurt you.** A foreign QR can't present *your* server's
  pinned identity, and it carries no valid enrollment code. There is no
  trust-on-first-use gap — the pin is *delivered*, not assumed.
- **Self-hosting on a LAN needs no public CA.** Because the pin travels in the QR, a
  self-signed server cert works fine: the phone learns to trust exactly that server.
  The only thing lost versus today is plaintext `ws://` to a bare IP — which was the
  vulnerability, not a feature.

**Reboot self-heal is preserved.** The device key lives in the hardware keystore and
the pin persists on disk, so a power-cut reboot reconnects with zero user
interaction, exactly as before. Mutual auth doesn't touch the zero-click story.

---

## Credential lifecycle & revocation

| Credential | Storage | Rotation | Revocation |
|---|---|---|---|
| Device key | hardware keystore, non-exportable | re-enroll (fresh keypair) | delete the public key → instant lockout |
| MCP token | hashed server-side; shown once | rotate per token | revoke one without touching the others |
| SSH key | public key stored (not a secret) | add / remove | delete → the jump rejects it |
| Session | server-side, hashed | rotate on privilege change | logout / revoke-all |
| **Server identity** | cert + pinned public key | pin a **self-managed CA or a backup pin-set** so a leaf rotation doesn't brick devices; push a new pin signed by the current key over the already-authenticated channel | rotate the CA |

The server-identity row is the subtle one: naive leaf pinning makes rotation brick
every device at once. Pin a CA (or a small set of backup keys) instead, and let an
already-mutually-authenticated device receive a new pin signed by the current key.

---

## Observability & revocation

abacad owns a **thin, non-semantic** sliver of responsibility here. It is
deliberately *not* an approval or policy layer — abacad does **not** judge whether an
action is safe, does not gate individual actions, and has no "arm the device" toggle.
Enrollment *is* the authorization; the kill switch is the off. What remains is only:

- **Scope** — which devices a credential reaches. Part of the auth chain, not a gate;
  defaults to full-account, changeable only via the human session (principle 2).
- **Audit** — an append-only record of every command: source, method, outcome,
  duration, and every tunnel target. Automatic, no configuration, nothing to judge.
- **Kill switch** — a human emergency stop that disconnects (and optionally revokes
  the device key), propagating over the live channel immediately. It decides
  nothing; a person hits it.

### Where the judgment lives instead

Deciding whether an action is *dangerous* needs the task's **intent**, which lives in
the **agent**, not in abacad — abacad sees a UI tree and can't know what the user
asked for. So semantic judgment is the agent's job, and most agents already gate
their own tool calls with a human in the loop.

This leaves exactly one gap that authentication cannot close: **prompt injection
through authentic screen content** — a poisoned page or UI tree, returned by
`screenshot`, that steers the agent. From abacad's integrity lens those bytes are
*perfectly authentic*; there is nothing to detect. That residual is defended not by
the auth chain but by, in order: the **agent's** own judgment (primary), **device
hygiene** (don't leave an always-on automation phone logged into your bank), and
abacad's **audit + kill switch** as the backstop. abacad observes and can cut the
cord; it does not pre-judge the action.

### The one channel-integrity control

The `/connect` tunnel **target** is policed server-side (and, defense-in-depth, on
the device): deny loopback, link-local (`169.254.0.0/16`, including the cloud
metadata endpoint), and RFC-1918 / ULA ranges by default, opt-in per device. This is
framed as *channel integrity* — not letting the pipe be turned into a network pivot —
not as action policy. It's transparent: an agent reaching a normal host never
notices. (The SSH jump already does the strict version of this by pinning its target
to `127.0.0.1:22`.)

---

## What this defends — and what it doesn't

| Threat | Defended by |
|---|---|
| LAN MITM / rogue AP / ARP-DNS spoof | mandatory pinned wss + mutual TLS (edge ③) |
| Rogue or mis-issued CA certificate | **public-key pinning** on the device (edge ③) |
| Credential stolen from a device, logs, or a backup | hardware key; nothing secret transmitted or stored (edge ③) |
| Token harvested from proxy / `Referer` / history | no secrets in URLs (edge ②) |
| Stolen MCP token | scoped, non-escalating, audited, individually revocable (edge ②, principle 2) |
| Malicious QR aiming a device at an attacker | pin + code originate from *your* authenticated dashboard (enrollment) |
| Tunnel SSRF / network pivot | connect-time authz + server & device target policy (edge ⑤) |
| Account takeover | MFA + rate-limit + session hygiene (edge ①) |
| **Prompt injection via authentic screen content** | **not an auth problem** — agent judgment + device hygiene + audit/kill backstop |

The last row is the honest one: the auth chain **cannot** close injection, because
the malicious bytes arrive with perfect integrity. Say so plainly rather than imply
crypto solves it.

---

## Cost to users and agents

Almost all of this is transparent or one-off — the model was factored that way on
purpose (semantic judgment offloaded to the agent, the pin delivered in the QR you
already scan, keys in hardware so reconnect self-heals):

- **Transparent (zero ongoing cost):** channel encryption + pinning, device
  keypair / mutual TLS, server key rotation, no-token-in-URL, token scope (from the
  agent's view — it just sends a token), the audit log, the kill switch until used.
- **One-off setup:** device enrollment (the *same* visible step as today — scan a
  QR).
- **Recurring, by design:** MFA at login — infrequent under a long session TTL, and
  the one non-transparent cost worth keeping, because the dashboard is the crown
  jewel.

Agents notice nothing new: same token, same header, same tools. The only behavioral
change is that an out-of-scope action returns an error — indistinguishable from
today's "you don't own that device."

---

## Rollout order

- **P0 — restore channel integrity** (the currently-broken half): mandatory wss and
  remove cleartext, pin the server identity on the device, move all tokens out of
  URLs, secure token storage. Mostly client + config; closes the
  highest-consequence gaps.
- **P1 — identity upgrade:** device keypair + mutual-TLS enrollment replacing the
  shared device token; scoped, expiring MCP tokens; tunnel target policy; surfaced
  audit + kill switch; dashboard MFA + rate-limit.
- **P2 — hardening:** platform attestation, pin rotation via signed update, quotas
  and handshake deadlines.

Once P0 ships you can honestly say the channel and both endpoints can't be
hijacked. Once P1 ships you can say a credential can't escalate itself and
everything is logged and revocable — which is the whole advertisable claim, every
word of it true.

---

## Relationship to `transport.md` and `ssh.md`

- [`transport.md`](transport.md) — the control-plane / data-plane split this doc's
  principle 1 assigns trust to.
- [`ssh.md`](ssh.md) — the jump host (edge ④), the one place server-to-client
  authentication (host-key pinning) is already done right, and the model edge ③
  should copy.
