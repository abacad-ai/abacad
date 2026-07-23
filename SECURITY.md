# Security policy

abacad turns a real device into something a remote agent can see and control, so
we take reports seriously. Thank you for helping keep it safe.

## Reporting a vulnerability

**Email <security@abacad.ai>.** Please do **not** open a public GitHub issue for a
security problem — report privately first so we can fix it before it's disclosed.

Include what you'd want if you were triaging it:

- what the issue is and where (component, endpoint, file, or client),
- a proof of concept or the exact steps to reproduce,
- the impact you think it has, and
- any suggested fix.

If you need to encrypt, say so in a first (contentless) email and we'll exchange a
key.

## What to expect

- **Acknowledgement within 3 business days** that we received your report.
- An initial assessment (severity + whether we can reproduce) within about a week.
- Progress updates as we work a fix, and coordinated disclosure timing agreed with
  you. We aim to remediate critical issues within 30 days.
- Credit in the release notes / this file if you'd like it (or stay anonymous —
  your call).

We don't run a paid bounty yet. We do commit to treating good-faith reporters as
collaborators, not adversaries.

## Scope

In scope:

- the hosted service at **abacad.ai** (relay, dashboard API, `/mcp`, `/connect`,
  `/blobs`, the SSH jump), and
- the server and clients in this repository (Go server, Android / macOS / Windows /
  Linux / browser clients).

Generally **out of scope** (report anyway if you're unsure, but these usually
aren't actionable):

- volumetric denial of service / load testing against the hosted service,
- social engineering of abacad staff or users, physical attacks, or stolen devices,
- issues that require a already-compromised device or a malicious account operating
  only on its **own** devices,
- missing hardening that is already tracked as roadmap work — see the honest
  **implementation-status table in [`docs/trust.md`](docs/trust.md)** before
  reporting "you don't pin the server key" or "there's no MFA yet"; those are known
  and scheduled (P1), not undiscovered bugs.

## Safe harbor

We will not pursue or support legal action against research that is conducted in
good faith and:

- stays within accounts and devices **you own or are explicitly authorized to
  test**,
- does not access, modify, or exfiltrate other users' data,
- does not degrade the service for others (no DoS, no spam), and
- gives us reasonable time to remediate before public disclosure.

If in doubt about whether something is allowed, ask us first at
<security@abacad.ai>.

## Supported versions

abacad is developed on `main` and the hosted service tracks it. Fixes land on
`main` and are deployed forward; there is no separate LTS branch. Self-hosters
should track `main` for security fixes.

## Related

- [`docs/trust.md`](docs/trust.md) — the trust model and what is shipped vs planned.
- [`docs/shared-responsibility.md`](docs/shared-responsibility.md) — who secures what.
- [`docs/incident-response.md`](docs/incident-response.md) — how we handle an incident.
- [`docs/abuse.md`](docs/abuse.md) — reporting misuse (distinct from a vulnerability).
