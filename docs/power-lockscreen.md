# Power, screen, and lock screen — cases & support decisions

The question this answers: **can a phone run as an Abacad device long-term, hands-off, and only
"get busy" when the agent calls it?** Short answer: **yes** — the connection idles cheaply and
the agent wakes the screen on demand — with **one hard constraint (no secure lock)** and **one
OEM-dependent risk (background-launch)**.

This doc is the source of truth for what we support and why. The behavior it describes is
implemented by **automatic wake** (`WakerActivity`, run before every command) plus the phone's
own display timeout for sleep, over the existing idle WebSocket. There is no agent-facing
wake/sleep tool — power is transparent to the agent.

---

## The three dimensions

Every deployment is a point in this space:

| Dimension | Options |
|---|---|
| **Power** | on charger · on battery |
| **Screen at idle** | stays on · turns off (to save energy / screen lifespan) |
| **Lock** | None · Swipe (non-secure) · PIN/pattern/biometric (**secure**) |

Two hard facts constrain the whole space:

1. **The primitives need a live, unlocked screen.** With the screen off, `screenshot` returns
   black, its UI tree shows the lockscreen (not the target app), and `tap`/`swipe` land on the
   lockscreen. So work can only happen after the screen is on **and** past the keyguard — which
   is exactly why wake is automatic (below).
2. **A secure keyguard cannot be dismissed programmatically.** `requestDismissKeyguard` only
   clears a *swipe* lock. PIN / pattern / password / fingerprint = a human (or biometric) must
   unlock, and again after every reboot. Nothing in software climbs that wall.

---

## How idle & wake work

- **Idle connection.** The app dials out and holds one long-lived WebSocket (20s pings, no work
  until a command arrives). On a charger this stays alive indefinitely at near-zero cost.
- **Doze (battery only).** Unplugged + screen-off + stationary triggers Doze, which suspends
  network for non-exempt apps. The idle socket typically goes half-open and only reconnects at a
  *maintenance window* — frequent early, stretching toward ~an hour as Doze deepens. Nothing is
  lost (the client reconnects with backoff), but delivery is delayed and quantized. **Charger =
  no Doze**, so this whole problem disappears when plugged in.
- **Automatic wake-on-command.** When a command arrives on a dark or locked phone, the service
  holds a brief CPU wakelock and launches `WakerActivity`, which powers the display on, shows
  over the keyguard, and dismisses a *non-secure* keyguard — then the command runs. This is
  invisible to the agent: it just issues `screenshot`/`tap`/etc. and the screen is brought up
  first. A *secure* keyguard can't be dismissed, so the command returns a clear error instead.
- **Sleep is the device's own timeout.** There is no sleep command; after the system
  display-timeout the screen turns off on its own, and the next command auto-wakes it again. For
  long uninterrupted sessions, enable *Stay awake while charging* so the screen never sleeps.

---

## Support matrix

✅ supported · ⚠️ supported with a caveat · ❌ not supported

| Power | Screen idle | Lock | Verdict | Notes |
|---|---|---|---|---|
| Charger | **On** (Stay Awake) | None / Swipe | ✅ **recommended** | No Doze, no lock wall, no wake step. The simplest, most reliable posture. |
| Charger | On (Stay Awake) | Secure | ⚠️ | Works only after a human unlocks once; re-locks on reboot/power-button. |
| Charger | **Off** (idle dark) | None / Swipe | ✅ | Auto-wakes on the next command. Subject to the OEM background-launch risk below. |
| Charger | Off | Secure | ❌ | Auto-wake turns the screen on but can't unlock; the command errors at the keyguard. |
| Battery | On | None / Swipe | ⚠️ | Fine while awake, but battery drains fast and Doze never engages (screen on). Charger is better. |
| Battery | **Off** | None / Swipe | ⚠️ | Works, but Doze delays command delivery (see below) and each wake costs battery. |
| Battery | Off | Secure | ❌ | Same wall as charger+off+secure. |

**Battery + on-battery-idle reliability:** today the server *fails a command fast* if the device
is momentarily disconnected (Doze gap) rather than waiting. To make on-battery idle behave like
on-charger, do one of: (a) grant the app battery-optimization exemption so the socket survives
Doze, or (b) add server-side queue-until-reconnect. Until then, treat battery+screen-off as
best-effort with unpredictable latency.

---

## Our support decisions

1. **Blessed configuration: charger + no secure lock.** Everything else is a variation on this.
   Documented as the default in setup. Screen may be left **on** (Stay Awake) or **off** (auto-woken
   on the next command) — both supported on a charger.
2. **No secure lock for hands-off.** A PIN/pattern/biometric is a hard wall we will not pretend
   to solve. We support secure-lock devices only with a human-in-the-loop unlock, and we say so.
3. **Screen-off idle just works** for battery life and screen longevity/discretion: the phone
   sleeps on its own display timeout and the next command auto-wakes it. No device-admin grant is
   needed (there is no software sleep); the overlay permission still helps make the auto-wake
   reliable on strict ROMs.
4. **On-battery is "same features, shorter life + looser latency,"** not a different capability
   set. We don't gate features on power source; we document the Doze latency caveat.
5. **OEM background-launch is a known risk, verified per device.** Launching the waker from the
   background can be blocked on aggressive ROMs (e.g. the ZTE/MiFavor test device). Mitigation:
   grant "Display over other apps" (SYSTEM_ALERT_WINDOW), which exempts background activity
   starts. If a ROM still blocks it, that device falls back to *screen stays on* (Stay Awake).

---

## Setup checklist (hands-off, screen-off idle)

1. Plug into a **charger** (kills Doze).
2. Screen lock → **None** or **Swipe** (never a secure lock).
3. Install the app, set the server URL, enable **Accessibility**.
4. Tap **Allow Display Over Other Apps** → grant (makes auto-wake reliable on strict ROMs).
5. Grant **battery-optimization exemption** for the app (keeps the service alive).
6. Verify: let the screen time out (or press the power button), then from the agent issue a
   `screenshot` → the screen auto-wakes & unlocks and the UI tree shows the real foreground app
   (not the lockscreen).

If you don't need screen-off idle, skip 4 and instead enable Developer Options → **Stay awake
while charging**; the screen never sleeps and the lock question is moot.

---

## Known limits (deferred, not feasibility risks)

- **Secure keyguard** — unlock requires a human; re-locks on reboot. By design, unsolvable in SW.
- **OEM background-activity-launch** — per-device; mitigated by the overlay permission, else fall
  back to Stay Awake.
- **On-battery Doze latency** — needs battery-optimization exemption and/or server queue-until-
  reconnect to match charger reliability.
- **Post-wake window** — the display sleeps again after the system timeout; the next command
  just auto-wakes it. Long sessions that want to avoid the wake latency should use Stay Awake.
