# Power, screen, and lock screen — cases & support decisions

The question this answers: **can a phone run as an Abacad device long-term, hands-off, and only
"get busy" when the agent calls it?** Short answer: **yes** — the connection idles cheaply and
the agent wakes the screen on demand — with **one hard constraint (no secure lock)** and **one
OEM-dependent risk (background-launch)**.

This doc is the source of truth for what we support and why. The behavior it describes is
implemented by **automatic wake** (`WakerActivity`, run before every command) plus the phone's
own display timeout for sleep, over a **held idle WebSocket**. To keep that socket alive through
screen-off, the app runs as a **foreground service** (ongoing notification), requests a
**battery-optimization exemption**, holds a **CPU wakelock while unplugged**, and force-reconnects
on **screen-on / network-regained** — so a screen-off device stays *reachable* ("sleeping"), not
offline. There is no agent-facing wake/sleep tool — power is transparent to the agent.

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
  until a command arrives). A **foreground service** keeps the process (and this socket) off the
  OEM idle/kill path; on a charger it stays alive indefinitely at near-zero cost.
- **Doze (battery only).** Unplugged + screen-off + stationary triggers Doze, which suspends
  network for non-exempt apps. We counter it: the app requests a **battery-optimization exemption**
  (so Doze doesn't defer its network) and holds a **`PARTIAL_WAKE_LOCK` while unplugged** (so the
  20s pings keep firing and the socket doesn't half-open). With both granted the socket survives
  screen-off off-charger; without the exemption it falls back to the old behavior (half-open until
  a maintenance window, which stretches toward ~an hour as Doze deepens). **Charger = no Doze**, so
  the wakelock is skipped there — it would only waste power.
- **Reconnect triggers.** If the socket does die (deep Doze, a network blip, an OEM freeze), the
  client force-reconnects immediately on **screen-on**, **unlock**, and **network-regained** —
  instead of waiting out the backoff — so the device is reachable again the moment it's touched.
- **Automatic wake-on-command.** When a command arrives on a dark or locked phone, the service
  holds a brief CPU wakelock and launches `WakerActivity`, which powers the display on, shows
  over the keyguard, and dismisses a *non-secure* keyguard — then the command runs. This is
  invisible to the agent: it just issues `screenshot`/`tap`/etc. and the screen is brought up
  first. A *secure* keyguard can't be dismissed, so the command returns a clear error instead.
- **Sleep is the device's own timeout.** There is no sleep command; after the system
  display-timeout the screen turns off on its own, and the next command auto-wakes it again. For
  long uninterrupted sessions, enable *Stay awake while charging* so the screen never sleeps.

---

## What you'll see on the device (expected behavior)

Concretely, once set up and connected:

- **A permanent notification.** While connected, an ongoing, low-priority notification sits in the
  shade: *"Abacad — Keeping this device reachable for the agent."* It can't be swiped away (that's
  the foreground service staying alive) and won't make sound. This is the one always-visible cost.
- **The screen still turns off on its own.** The app does **not** keep the screen on while idle —
  after the system display timeout the screen goes dark normally. (It only holds the screen awake
  *during* an active session: for ~3 min after the last command, via a 1px invisible overlay, so it
  doesn't re-wake on every command. After that window it lets the screen sleep again.)
- **Screen off ≠ offline.** This is the whole change: with the screen dark the socket stays held, so
  the dashboard keeps showing the device **online** and an agent can reach it. Before this fix the
  socket died and it went *offline* — that's the bug being closed.
- **The Abacad app is never brought to the front.** Neither the setup screen (`MainActivity`) nor
  anything with UI is raised when the agent works. Commands run over the accessibility service on
  **whatever app is already foreground** — the agent drives that app in place; Abacad stays in the
  background.
- **What waking a dark screen looks like.** When a command arrives on a dark/locked phone, a
  *transparent, empty* activity (`WakerActivity`) is launched purely to turn the display on and
  dismiss a non-secure keyguard, then it finishes after ~0.3s. It renders nothing and is excluded
  from Recents, so you don't see an "Abacad" screen — you see the display light up to the lock
  screen / last app, then the app the agent is driving. The screen stays on for the session, then
  sleeps again ~3 min after the last command.
- **A secure lock stops here.** If a PIN/pattern/biometric is set, the waker turns the screen on but
  can't get past the keyguard; the command returns a clear error. Hands-off use needs None/Swipe.
- **Battery.** On a charger this is all effectively free. Off-charger the app holds a CPU wakelock to
  keep the socket healthy through Doze, which adds meaningful drain — the plugged-in drawer phone is
  the intended posture.

Net: a connected phone looks *asleep* (dark screen, one quiet notification), stays reachable, and
briefly lights up to do work when the agent calls — returning to dark on its own afterward.

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
| Battery | **Off** | None / Swipe | ⚠️ | Now survives Doze with the battery-optimization exemption + off-charger wakelock; the wakelock costs battery, and the most aggressive OEMs (see below) may still freeze the app. Best-effort, much improved. |
| Battery | Off | Secure | ❌ | Same wall as charger+off+secure. |

**On-battery-idle reliability:** option (a) is now **implemented** — the app runs as a foreground
service, requests the battery-optimization exemption, and holds a CPU wakelock while unplugged, so
the socket survives Doze off-charger. Option (b), server-side queue-until-reconnect, remains
deferred; it (plus an FCM push-wake channel) is what would let the phone *fully* deep-sleep with
the socket down and still revive — the next step if aggressive OEMs kill even the foreground
service.

**Aggressive OEMs (Samsung/Xiaomi/Huawei/Oppo).** A foreground service + battery-opt exemption is
the standard mitigation, but some ROMs sleep apps anyway. **Samsung One UI:** add the app to
**Settings → Battery → Background usage limits → Never sleeping apps** (Device Care), or it will
be deep-slept and the socket will drop despite everything above. See dontkillmyapp.com per brand.

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
5. Tap **Ignore Battery Optimization** → grant (keeps the held socket alive through Doze).
6. **Samsung only:** Settings → Battery → Background usage limits → **Never sleeping apps** → add
   Abacad Probe (One UI deep-sleeps apps otherwise and drops the socket).
7. Verify: let the screen time out (or press the power button), then from the agent issue a
   `screenshot` → the screen auto-wakes & unlocks and the UI tree shows the real foreground app
   (not the lockscreen).

If you don't need screen-off idle, skip 4 and instead enable Developer Options → **Stay awake
while charging**; the screen never sleeps and the lock question is moot.

---

## Known limits (deferred, not feasibility risks)

- **Secure keyguard** — unlock requires a human; re-locks on reboot. By design, unsolvable in SW.
- **OEM background-activity-launch** — per-device; mitigated by the overlay permission, else fall
  back to Stay Awake.
- **On-battery Doze latency** — countered by the foreground service + battery-opt exemption +
  off-charger wakelock (implemented). Fully deep-sleeping the phone (socket down) and reviving it
  still needs server queue-until-reconnect + an FCM push-wake channel (deferred).
- **Aggressive OEM app-sleep** — Samsung/Xiaomi/etc. may freeze even a foreground service; needs a
  per-brand "never sleeping" allowlisting by the user (Samsung step in the checklist above).
- **Post-wake window** — the display sleeps again after the system timeout; the next command
  just auto-wakes it. Long sessions that want to avoid the wake latency should use Stay Awake.
