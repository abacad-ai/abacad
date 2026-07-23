# Shared responsibility

abacad can see a paired device's screen, read its on-screen text, inject input,
transfer files, and record the screen. Security is therefore a partnership: some of
it is ours, some of it is yours, and **which is which depends on how you run
abacad.** This document draws that line so neither side assumes the other has it
covered.

There are two deployment modes:

- **Self-hosted** — you run the relay (the server in this repo) on your own
  infrastructure. You are both the operator and the data controller.
- **Hosted (abacad.ai)** — we run the relay. We are the operator/processor; you are
  still the controller of your devices and what they do.

## The line

| Layer | Self-hosted | abacad.ai (hosted) |
|---|---|---|
| Relay / server infrastructure security & patching | **You** | **abacad** |
| Shipping timely security fixes & advisories (as the software vendor) | **abacad** | **abacad** |
| Data at rest (blobs, screenshots, DB) | **You** (your disk) | **abacad** (our infra) |
| Transport security (TLS to the relay) | **You** configure it | **abacad** |
| Breach detection & notification for the relay | **You** (you're the operator) | **abacad** |
| **Which devices you enroll, and what they're signed into** | **You** | **You** |
| **Who you authorize** (API keys, additional operators) | **You** | **You** |
| **Account credential hygiene** (password, MFA when available) | **You** | **You** |
| **Lawful, authorized use** and any third-party consent required | **You** | **You** |

The bottom four rows are **always yours**, in either mode. abacad relays and audits;
it does not decide whether an action is wise or permitted — that judgment lives with
you and your agent (see [`trust.md`](trust.md), "Where the judgment lives instead").

## What the hosted service provides

On abacad.ai we operate the relay and provide, today:

- **TLS everywhere** — `wss://` is mandatory; clients refuse cleartext off-loopback.
- **Account isolation** — every device, blob, and token is scoped to its account.
- **Secrets hashed at rest** — device tokens, API tokens, and session IDs are stored
  only as hashes; passwords are bcrypt.
- **An append-only audit trail** — every relayed command's method, source, outcome,
  and timing (never its parameters or screen contents).
- **Data minimization** — screenshots and transferred blobs are deleted on a
  retention window; on-screen text is relayed, never persisted.
- **Ephemeral enrollment** — devices auto-expire 24h after enrollment unless
  extended or explicitly made permanent, bounding the blast radius of a compromise.
- **Consent gates** — enrollment and "make permanent" require an explicit
  acknowledgement of what the device exposes.

**Honest limits.** Several deeper protections are on the roadmap, not yet shipped —
notably device↔server mutual authentication / server-key pinning, scoped/expiring
MCP tokens, a surfaced kill switch, and dashboard MFA. See the implementation-status
table in [`trust.md`](trust.md) for exactly what runs today. In particular, until
mutual endpoint authentication ships, a compromise of the relay could expose or
drive a device that is **actively connected** at that moment. Auto-expiry caps *how
many* devices are exposed; it does not yet reduce the per-device exposure of a live
session. We disclose this at the "make permanent" step rather than imply protection
we don't have.

## What is always yours

Regardless of who runs the relay, you are responsible for:

- **What a device is signed into.** An agent can do anything on the device that the
  logged-in user can. Don't leave an always-on, permanently-enrolled device signed
  into sensitive accounts (banking, email, primary identity). Prefer a clean profile
  for a drawer device.
- **Authorization.** Only enroll devices you own or are authorized to operate, and
  only issue API keys to agents/people you trust. Anyone who can enroll a device must
  physically possess it.
- **Consent of others.** If a device is used by, or displays the data of, other
  people, they must know it can be viewed and controlled remotely. Covert monitoring
  is prohibited (see [`abuse.md`](abuse.md) and the Terms).
- **Lawful use.** Comply with the Acceptable Use terms and applicable law, including
  any data-protection rules that apply to the personal information your devices
  process.
- **Your account.** Keep your password secret, enable MFA when available, and revoke
  credentials you no longer use.

## If you self-host

You inherit the operator responsibilities in the left column: run current code
(track `main` for security fixes), terminate TLS properly, secure your host and
database, and handle your own breach-detection and notification duties. The
enrollment 24h expiry and the retention windows are product behavior and apply to
you too; the dormant-row retention window is configurable
(`ABACAD_DEVICE_DORMANT_DELETE_DAYS`), as are the blob/screenshot/activity windows.
