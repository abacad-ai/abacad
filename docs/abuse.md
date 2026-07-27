# Reporting abuse

abacad gives an agent real eyes and hands on a device. That power can be misused, and
we want to hear about it when it is. This page is for reporting **misuse of the
service** — which is different from a security vulnerability (for that, see
[`SECURITY.md`](../SECURITY.md)).

## What counts as abuse

Using abacad to do any of the following violates our Terms and is abuse:

- **Controlling a device or account you don't own or aren't authorized to operate.**
- **Covert surveillance** — monitoring a person through a device without their
  knowledge and consent (stalkerware-style use). Anyone using a paired device must be
  aware it can be viewed and controlled remotely.
- **Circumventing another platform's protections** — defeating bot detection,
  CAPTCHAs, rate limits, or other anti-automation measures, or otherwise breaking a
  third party's terms.
- **Unlawful, fraudulent, or infringing use**, or processing someone's personal data
  without a lawful basis.

## How to report

Email **<abuse@abacad.ai>** with as much of the following as you can:

- what's happening and who/what is affected,
- any identifiers you have (a device, account, URL, or the behavior you observed),
- and how you came to know about it.

If you believe an **account of yours** is being misused, or a device you own was
enrolled without your consent, tell us — we can help you unenroll it and cut its
access.

## What we do about it

We are the integrity and audit layer, not a court, but we act on credible reports:

- **Investigate** using the audit trail (which records the commands run, not their
  contents) and the reported details.
- **Contain** — disconnect a device, revoke its credentials, and/or suspend the
  account behind the abuse, using the same controls described in
  [`incident-response.md`](incident-response.md).
- **Preserve evidence** for the investigation and for any lawful request.
- **Report to authorities** where the law requires it or where someone is in danger.

Design choices that make covert misuse harder in the first place: enrolling a device
requires **physical possession** of it, enrollment **discloses** that the device can
be controlled remotely, and enrollments **auto-expire after 24 hours** unless
explicitly renewed — so a device that's abandoned or forgotten stops being reachable
on its own.

All three still hold when a device enrols itself (see
[`trust.md`](trust.md#two-enrollment-directions)), but two of them work differently:

- **Physical possession is stronger, not weaker.** Claiming a self-registered device
  requires reading its id *and* a rotating claim code off the device's own screen.
  Neither appears anywhere else.
- **Disclosure moves to the device.** It used to live only in the dashboard's approval
  dialog. The human with physical possession is now standing at the *device*, so the
  device states what claiming will permit, before the claim — and the dashboard keeps
  its acknowledgement too. Both, not either.
- **The 24-hour clock starts at the claim**, not at install, so it still measures how
  long an *account* has been responsible for the device.

**One risk this adds:** anyone who can see the screen can claim the device to their
own account. If someone claimed a device of yours this way, the device itself shows
which account holds it and offers a one-action disconnect — and you can tell us at
<abuse@abacad.ai>. If you are setting up a device in a public place, treat the claim
code like a password on screen: claim it before you walk away.

## If you are in danger

If you think a device is being used to monitor or control you and you may be at risk,
prioritize your safety. Disconnect the device from the internet or power it off if
you can do so safely, and contact local support services or law enforcement. Then, if
it's safe to, let us know at <abuse@abacad.ai> so we can revoke the device's access
from our side.
