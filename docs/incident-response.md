# Incident response (hosted service)

This is the runbook for a suspected or confirmed security incident on the hosted
abacad.ai service — an intrusion, a data exposure, a leaked credential, or abuse of
the relay. It exists so that when something goes wrong we act from a plan instead of
improvising, and so we meet our breach-notification duties. Self-hosters are their
own operators and should adapt this to their environment.

It is intentionally lightweight — sized for a small team — but every incident should
still leave a written trail.

## What counts as an incident

Any of:

- unauthorized access to the relay, database, blob store, or a privileged account;
- exposure of user data (screen contents, files, credentials, device inventory,
  connectivity metadata);
- a confirmed vulnerability being exploited in the wild;
- loss of the audit trail or kill controls;
- an abuse report indicating the platform is being used for covert surveillance or
  unauthorized device control at scale (see [`abuse.md`](abuse.md)).

When unsure whether something is an incident, treat it as one until triage says
otherwise.

## Severity

| Level | Meaning | Examples |
|---|---|---|
| **P0** | Active compromise or ongoing data exposure | attacker in the relay/DB; live exfiltration; secret-signing key leaked |
| **P1** | Confirmed vulnerability with real exposure, not yet weaponized | auth bypass found; a token class leakable; SSRF reaching new targets |
| **P2** | Limited or contained issue | single-account compromise from a phished password; a low-impact leak |
| **P3** | Minor / hardening | info leak with no practical impact |

## Roles

For a small team one person may wear several hats, but name them explicitly per
incident:

- **Incident lead** — owns the response, makes the calls, keeps the timeline.
- **Comms** — user/regulator notifications and any public statement.
- **Scribe** — timestamps every action and finding in the incident log.

## Phases

1. **Detect & declare.** Sources: audit-trail anomalies, error/latency spikes,
   a `security@` report, an `abuse@` report, or a provider alert. The first responder
   declares an incident, assigns severity, and opens an incident log.
2. **Triage.** Confirm it's real, scope it (what data/accounts/devices, over what
   window), and identify the entry point. Preserve evidence — **do not** wipe the
   audit trail or logs; snapshot them.
3. **Contain.** Stop the bleeding using the controls we have:
   - disconnect affected devices (relay kick) and revoke their credentials;
   - rotate or revoke leaked secrets (signing keys, tokens, sessions — "revoke all
     sessions" for an account);
   - suspend a compromised or abusive account;
   - if the relay itself is compromised, take it out of rotation; note that
     enrollment auto-expiry (24h) already caps standing device exposure.
4. **Eradicate.** Remove the attacker's access and close the root cause (patch,
   config fix, credential reset). Don't just paper over the symptom.
5. **Recover.** Restore normal operation from a known-good state, watch for
   recurrence, and confirm the fix holds.
6. **Notify.** See below — start the clock at *confirmation*, run this in parallel
   with recovery, not after.
7. **Post-incident review.** Within ~1 week, write a blameless post-mortem: timeline,
   root cause, what worked, what didn't, and concrete follow-ups (with owners). File
   the follow-ups as tracked work.

## Notification

Breach-notification duties depend on where affected users are and what data was
involved; **confirm specifics with counsel** — this section is the default posture,
not legal advice.

- **Affected users:** notify **without undue delay** once a breach involving their
  personal data is confirmed — plainly: what happened, what data, what they should
  do (e.g., unenroll a device, change a password on an account the device was signed
  into, revoke tokens).
- **Regulators:** where the law requires it, notify the relevant authority promptly —
  commonly **within 72 hours** of becoming aware (e.g., GDPR-style regimes), and
  "without undue delay" under China's PIPL and most US state laws. Timelines and
  thresholds vary by jurisdiction; when in doubt, notify.
- **Record** the decision either way — including a documented rationale if you
  conclude notification isn't required.

Keep the security contact (<security@abacad.ai>) and abuse contact
(<abuse@abacad.ai>) monitored so external parties can reach us during an incident.

## Preparation (do before an incident)

- Provision and monitor `security@` and `abuse@`.
- Keep the audit trail and infrastructure logs retained long enough to reconstruct an
  incident, and backed up out-of-band so an intruder can't erase them.
- Know where secrets live and how to rotate each (signing keys, OAuth secrets, DB
  credentials).
- Keep a current contact for legal/privacy counsel for the notification call.
