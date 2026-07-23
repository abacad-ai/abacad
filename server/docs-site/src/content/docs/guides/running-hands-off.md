---
title: Running a phone hands-off
description: Can an Android phone run as an abacad device long-term, hands-off, and only get busy when the agent calls it? Yes — the connection idles cheaply and the screen auto-wakes on demand. Setup, the support matrix, and the one hard constraint (no secure lock).
---

Can a phone run as an abacad device long-term, hands-off, and only "get busy" when the
agent calls it? **Short answer: yes** — the connection idles cheaply and the agent wakes
the screen on demand — with **one hard constraint (no secure lock)** and **one
OEM-dependent risk (background-launch)**.

The behavior below is delivered by **automatic wake** (run before every command) plus the
phone's own display timeout for sleep, over a **held idle WebSocket**. To keep that socket
alive through screen-off, the app runs as a **foreground service** (ongoing notification),
requests a **battery-optimization exemption**, holds a **CPU wakelock while unplugged**,
and force-reconnects on **screen-on / network-regained** — so a screen-off device stays
*reachable* ("sleeping"), not offline. There is no agent-facing wake/sleep tool — power is
transparent to the agent.

## The three dimensions

Every deployment is a point in this space:

| Dimension | Options |
|---|---|
| **Power** | on charger · on battery |
| **Screen at idle** | stays on · turns off (to save energy / screen lifespan) |
| **Lock** | None · Swipe (non-secure) · PIN/pattern/biometric (**secure**) |

Two hard facts constrain the whole space:

1. **The primitives need a live, unlocked screen.** With the screen off, `screenshot`
   returns black, its UI tree shows the lockscreen (not the target app), and `tap`/`swipe`
   land on the lockscreen. So work can only happen after the screen is on **and** past the
   keyguard — which is why wake is automatic.
2. **A secure keyguard cannot be dismissed programmatically.** A *swipe* lock can be
   cleared; PIN / pattern / password / fingerprint means a human (or biometric) must
   unlock, and again after every reboot. Nothing in software climbs that wall.

## How idle & wake work

- **Idle connection.** The app dials out and holds one long-lived WebSocket (20s pings, no
  work until a command arrives). A **foreground service** keeps the process (and this
  socket) off the OEM idle/kill path; on a charger it stays alive indefinitely at near-zero
  cost.
- **Doze (battery only).** Unplugged + screen-off + stationary triggers Doze, which
  suspends network for non-exempt apps. The app counters it with a **battery-optimization
  exemption** and a **partial wakelock while unplugged**, so the pings keep firing and the
  socket stays healthy. **On a charger there is no Doze**, so the wakelock is skipped.
- **Reconnect triggers.** If the socket does die (deep Doze, a network blip, an OEM
  freeze), the client force-reconnects immediately on **screen-on**, **unlock**, and
  **network-regained** — so the device is reachable again the moment it's touched.
- **Automatic wake-on-command.** When a command arrives on a dark or locked phone, the
  service briefly powers the display on, shows over the keyguard, and dismisses a
  *non-secure* keyguard — then the command runs. This is invisible to the agent: it just
  issues `screenshot`/`tap`/etc. and the screen is brought up first. A *secure* keyguard
  can't be dismissed, so the command returns a clear error instead.
- **Sleep is the device's own timeout.** There is no sleep command; after the system
  display-timeout the screen turns off on its own, and the next command auto-wakes it. For
  long uninterrupted sessions, enable *Stay awake while charging* so the screen never
  sleeps.

## What you'll see on the device

- **A permanent notification — by design.** While connected, an ongoing, low-priority
  notification sits in the shade: *"abacad — Keeping this device reachable for the agent."*
  It can't be swiped away (that's the foreground service staying alive) and won't make
  sound. For a drawer phone this is **intended, not a cost** — it's the honest "on duty"
  signal, the same pattern remote-control / VPN / recorder apps use.
- **The screen still turns off on its own.** The app does not keep the screen on while
  idle — after the system display timeout it goes dark normally. (It only holds the screen
  awake *during* an active session, for ~3 min after the last command, so it doesn't
  re-wake on every command.)
- **Screen off ≠ offline.** With the screen dark the socket stays held, so the dashboard
  keeps showing the device **online** and an agent can reach it.
- **The abacad app is never brought to the front.** Commands run over the accessibility
  service on **whatever app is already foreground** — the agent drives that app in place;
  abacad stays in the background.
- **A secure lock stops here.** If a PIN/pattern/biometric is set, waking turns the screen
  on but can't get past the keyguard; the command returns a clear error. Hands-off use
  needs None/Swipe.
- **Battery.** On a charger this is all effectively free. Off-charger the app holds a CPU
  wakelock to keep the socket healthy through Doze, which adds meaningful drain — the
  plugged-in drawer phone is the intended posture.

## Support matrix

✅ supported · ⚠️ supported with a caveat · ❌ not supported

| Power | Screen idle | Lock | Verdict | Notes |
|---|---|---|---|---|
| Charger | **On** (Stay Awake) | None / Swipe | ✅ **recommended** | No Doze, no lock wall, no wake step. The simplest, most reliable posture. |
| Charger | On (Stay Awake) | Secure | ⚠️ | Works only after a human unlocks once; re-locks on reboot/power-button. |
| Charger | **Off** (idle dark) | None / Swipe | ✅ | Auto-wakes on the next command. Subject to the OEM background-launch risk below. |
| Charger | Off | Secure | ❌ | Auto-wake turns the screen on but can't unlock; the command errors at the keyguard. |
| Battery | On | None / Swipe | ⚠️ | Fine while awake, but battery drains fast and Doze never engages (screen on). Charger is better. |
| Battery | **Off** | None / Swipe | ⚠️ | Survives Doze with the battery-optimization exemption + off-charger wakelock; the wakelock costs battery, and the most aggressive OEMs may still freeze the app. Best-effort. |
| Battery | Off | Secure | ❌ | Same wall as charger + off + secure. |

**Aggressive OEMs (Samsung / Xiaomi / Huawei / Oppo).** A foreground service +
battery-opt exemption is the standard mitigation, but some ROMs sleep apps anyway.
**Samsung One UI:** add the app to **Settings → Battery → Background usage limits → Never
sleeping apps** (Device Care), or it will be deep-slept and the socket will drop. See
[dontkillmyapp.com](https://dontkillmyapp.com) per brand.

## What we support, and why

1. **Blessed configuration: charger + no secure lock.** Everything else is a variation on
   this. Screen may be left **on** (Stay Awake) or **off** (auto-woken on the next
   command) — both supported on a charger.
2. **No secure lock for hands-off.** A PIN/pattern/biometric is a hard wall we won't
   pretend to solve. Secure-lock devices are supported only with a human-in-the-loop
   unlock, and we say so plainly.
3. **Screen-off idle just works** for battery life and screen longevity: the phone sleeps
   on its own display timeout and the next command auto-wakes it.
4. **On-battery is "same features, shorter life + looser latency,"** not a different
   capability set. Features aren't gated on power source; the Doze latency caveat is
   documented.
5. **OEM background-launch is a known risk.** Launching the waker from the background can
   be blocked on some aggressive ROMs. Mitigation: grant "Display over other apps"
   (SYSTEM_ALERT_WINDOW), which exempts background activity starts. If a ROM still blocks
   it, that device falls back to *screen stays on* (Stay Awake).
6. **The foreground service (and its notification) stays on purpose.** The product is an
   always-on drawer phone, so maximizing uptime beats hiding a notification. The permanent
   low-priority notification is an accepted, expected "on duty" signal, not friction.

## Setup checklist (hands-off, screen-off idle)

1. Plug into a **charger** (kills Doze).
2. Screen lock → **None** or **Swipe** (never a secure lock).
3. Install the app, set the server URL, enable **Accessibility**.
4. Tap **Allow Display Over Other Apps** → grant (makes auto-wake reliable on strict ROMs).
5. Tap **Ignore Battery Optimization** → grant (keeps the held socket alive through Doze).
6. **Samsung only:** Settings → Battery → Background usage limits → **Never sleeping apps**
   → add abacad.
7. Verify: let the screen time out (or press the power button), then from the agent issue a
   `screenshot` → the screen auto-wakes & unlocks and the UI tree shows the real foreground
   app (not the lockscreen).

If you don't need screen-off idle, skip step 4 and instead enable Developer Options →
**Stay awake while charging**; the screen never sleeps and the lock question is moot.

## Known limits

- **Secure keyguard** — unlock requires a human; re-locks on reboot. By design,
  unsolvable in software.
- **OEM background-activity-launch** — per-device; mitigated by the overlay permission,
  else fall back to Stay Awake.
- **On-battery Doze latency** — countered by the foreground service + battery-opt
  exemption + off-charger wakelock. Fully deep-sleeping the phone and reviving it is a
  future step (server-side queue + push-wake).
- **Aggressive OEM app-sleep** — Samsung/Xiaomi/etc. may freeze even a foreground service;
  needs a per-brand "never sleeping" allowlisting by the user.
