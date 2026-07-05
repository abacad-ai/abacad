# Power, screen, and lock screen — cases & support decisions

The question this answers: **can a phone run as an Abacad device long-term, hands-off, and only
"get busy" when the agent calls it?** Short answer: **yes** — the connection idles cheaply and
the agent wakes the screen on demand — with **one hard constraint (no secure lock)** and **one
OEM-dependent risk (background-launch)**.

This doc is the source of truth for what we support and why. The behavior it describes is
implemented by the `wake` / `sleep` primitives (`WakerActivity`, `AbacadDeviceAdmin`) and the
existing idle WebSocket.

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
   black, `ui_tree` shows the lockscreen (not the target app), and `tap`/`swipe` land on the
   lockscreen. So work can only happen after the screen is on **and** past the keyguard.
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
- **Wake-on-command.** When a `wake` arrives on a dark phone, the service holds a brief CPU
  wakelock and launches `WakerActivity`, which powers the display on, shows over the keyguard,
  and dismisses a *non-secure* keyguard — then the primitives run. `sleep` (device-admin
  `lockNow()`) turns the screen back off between tasks.
- **Working window.** After `wake`, the display stays on for the system display-timeout, then
  sleeps again. The agent should `wake` → act → (optionally) `sleep`. For long uninterrupted
  sessions, enable *Stay awake while charging* instead of relying on the timeout.

---

## Support matrix

✅ supported · ⚠️ supported with a caveat · ❌ not supported

| Power | Screen idle | Lock | Verdict | Notes |
|---|---|---|---|---|
| Charger | **On** (Stay Awake) | None / Swipe | ✅ **recommended** | No Doze, no lock wall, no wake step. The simplest, most reliable posture. |
| Charger | On (Stay Awake) | Secure | ⚠️ | Works only after a human unlocks once; re-locks on reboot/power-button. |
| Charger | **Off** (idle dark) | None / Swipe | ✅ | Agent `wake`s on demand. Subject to the OEM background-launch risk below. |
| Charger | Off | Secure | ❌ | `wake` turns the screen on but can't unlock; agent is stuck at the keyguard. |
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
   Documented as the default in setup. Screen may be left **on** (Stay Awake) or **off** (agent
   wakes it) — both supported on a charger.
2. **No secure lock for hands-off.** A PIN/pattern/biometric is a hard wall we will not pretend
   to solve. We support secure-lock devices only with a human-in-the-loop unlock, and we say so.
3. **Screen-off idle is a first-class, opt-in feature** via `wake`/`sleep` — for battery life and
   screen longevity/discretion. It needs the device-admin grant (for `sleep`) and benefits from
   the overlay permission (for reliable `wake`).
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
4. Tap **Enable Screen Off (device admin)** → approve (unlocks `sleep`).
5. Tap **Allow Display Over Other Apps** → grant (makes `wake` reliable).
6. Grant **battery-optimization exemption** for the app (keeps the service alive).
7. Verify: from the agent, `sleep` → screen off → `wake` → screen on & unlocked → `ui_tree`
   returns the real foreground app (not the lockscreen).

If you don't need screen-off idle, skip 4–5 and instead enable Developer Options → **Stay awake
while charging**; the screen never sleeps and the lock question is moot.

---

## Known limits (deferred, not feasibility risks)

- **Secure keyguard** — unlock requires a human; re-locks on reboot. By design, unsolvable in SW.
- **OEM background-activity-launch** — per-device; mitigated by the overlay permission, else fall
  back to Stay Awake.
- **On-battery Doze latency** — needs battery-optimization exemption and/or server queue-until-
  reconnect to match charger reliability.
- **Post-wake window** — the display sleeps again after the system timeout; long sessions want
  Stay Awake rather than repeated `wake`s.
