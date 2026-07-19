# abacad

**A device interface for agents. Plug any device into an agent — once, from anywhere, safely.**

abacad turns a device (starting with an old Android phone) into something a remote AI
agent can see and control. Install once, grant one permission, drop it in a drawer — and
an agent anywhere on the internet can operate it, step by step, with a human supervising.

---

## The need (deliberately singular)

> Agents are minds with no body. They can think, but they can't touch anything.
> abacad gives them a body — eyes and hands — in a real device.

Everything else is implementation. If we ever find ourselves "building the platform"
instead of serving this one need, that's the signal we've drifted.

---

## Who it's for (and who it's *not*)

**This is not a remote-desktop tool for humans.** No smooth video, no low-latency mirror.
The core loop is **one screenshot + the UI tree per step** — perfect for an agent that acts
one action at a time, useless for a person who wants to watch and click in real time.
That's a deliberate fork, not a limitation. We optimized for the agent's eyes.

- **Primary user: the agent.** Machine-to-machine interface, agent-native ergonomics.
- **Human role: oversight, not operation.** Setup, approve sensitive actions, monitor the
  fleet, take over when the agent is stuck. A *control tower*, not a remote desktop.
- We are **not** competing with AirDroid / TeamViewer / VNC. Different customer entirely.

A smooth "human take-over" video stream is a possible **premium add-on later** — not the core.

---

## The control-surface ladder

Any device can be driven at one of four levels. A good agent stays as high as the task
allows and only drops down when it must.

```
1. API / programmatic   ← deterministic, structured, cheapest    (best)
2. CLI / shell          ← scriptable, token-cheap, reliable
3. Accessibility tree   ← semantic GUI, structured text
4. Screenshot + pixels  ← vision, slow, error-prone              (last resort)
```

- **Desktops expose the top rungs** (SSH/CLI is native) → mostly a solved, commodity problem.
- **Phones expose only the bottom rungs** (GUI-only, no shell for a normal user) → the hard,
  novel, differentiated problem. This is the wedge.

The device connection surfaces **every rung the OS offers at once**; the agent picks per
action. Not a mode the human toggles — the agent chooses `run_command` vs `tap` itself.

---

## Architecture: a matrix inside, one simple promise outside

```
                 ┌─────────────────────────────────────────────┐
   any agent ──▶ │  ONE tool contract  +  ONE relay  +          │ ──▶ any device
   (MCP)         │  ONE trust / approval layer                  │
                 └─────────────────────────────────────────────┘
                        │  backends (the matrix — internal)  │
   Android: accessibility  ·  macOS: AX + screen + ssh
   Windows: UIA + ssh      ·  Linux: AT-SPI / X11 + ssh   ·  iOS: (WDA / cloud only)
```

**The tool contract (identical across every device):**

| Tool | Purpose | Rung |
|------|---------|------|
| `run_command()` / `read_output()` | shell (desktop; via SSH) | CLI |
| `screenshot()` | one frame, on demand | pixels |
| `ui_tree()` | structured, semantic elements | accessibility |
| `tap(id)` / `type(text)` / `swipe(dir)` | inject input | — |

Each action waits for UI-idle and returns the resulting screen, so the agent sees its own effect.

**The relay:** the device **dials out** and holds one persistent connection (WebSocket).
The agent connects to the same relay; the relay pipes between them. This punches through
NAT / CGNAT with no port-forwarding and no public inbound port. The relay is
**protocol-agnostic** — a phone carries the accessibility protocol; a desktop can just
tunnel SSH/PTY over the same channel.

---

## The Android wedge (why this is defensible)

On a normal user's own **drawer phone**, no root, no ADB, install a normal app:

- **Control + vision from ONE permanent permission.** `AccessibilityService` gives the UI
  tree (`getRootInActiveWindow`) and input (`dispatchGesture`). On **Android 11+**,
  `AccessibilityService.takeScreenshot()` captures the screen **with no MediaProjection and
  no per-session consent** — verified against Google's API reference + a shipping app
  (droidVNC-NG uses it precisely to avoid the MediaProjection dialog).
- **Skip MediaProjection entirely** → the recurring per-reboot consent dialog disappears.
- **One-time setup, then zero clicks forever.** Accessibility is a permanent grant that
  survives reboots. Even a power-cut reboot **self-heals with zero user interaction**:
  boot receiver restarts the service → accessibility already enabled → screenshots resume →
  reconnects to relay. The user never touches it again after setup.

Setup, one time (~60s): install → scan QR to pair → toggle Accessibility (guided deep-link)
→ allow "ignore battery optimization" → drop it on a charger in the drawer.

**Why it's the wedge:** iOS is walled (no cross-app control for third-party apps); desktop
GUI control is crowded (VNC/RDP/computer-use). The one place a *normal person* can turn a
device they already own into an agent-controllable machine, with no recurring friction, is
**Android**. That asymmetry is the moat.

---

## What's actually defensible (not the backends)

Anyone can wrap SSH or an accessibility service. The product — and the moat — is the three
things on the *outside* of the box:

1. **The universal contract** — one clean agent interface across every device type. The
   standard-setting play.
2. **The relay + fleet layer** — NAT-proof out-dial reachability, reconnection, pairing,
   managing many devices. Boring, essential, sticky.
3. **The trust / approval layer** — risk-weighted human-in-the-loop gating. The gate should
   scale with the rung: a `run_command` (whole machine in one line) or a "Confirm payment"
   tap needs approval; routine taps flow freely. This is what makes it *safe* to hand an
   agent a real device, and it's the hardest thing to copy — it's judgment, not plumbing.

---

## Platform matrix

| Platform | Tree | Input + capture | Consent | Verdict |
|---|---|---|---|---|
| **Android** | AccessibilityService | `dispatchGesture` + `takeScreenshot` | one-time toggle | ✅ the wedge |
| **macOS** | AXUIElement | CGEvent + ScreenCaptureKit | two one-time toggles | ✅ easy |
| **Windows** | UI Automation | SendInput + Desktop Duplication | none | ✅ easiest |
| **Linux (X11)** | AT-SPI | XTEST + XGetImage | none | ✅ open |
| **Linux (Wayland)** | AT-SPI | PipeWire portal + libei | portal prompt | ⚠️ gated |
| **iOS** | ❌ not exposed | view-only, no cross-app input | — | ❌ walled (WDA / cloud) |

Desktop CLI (SSH) is commodity — reverse-tunneled or via mesh (Tailscale/WireGuard), never a
naked public port. It's the cheap technical-user beachhead; Android is the bigger prize.

---

## Open risks (the real obstacles are not the OS)

- **Google Play policy** — apps using AccessibilityService for non-accessibility purposes get
  scrutinized/pulled. Likely channel: sideload / direct APK / enterprise, or a genuine
  accessibility-framed exemption. *This gates a consumer launch more than any technical wall.*
- **OEM battery killers** — Xiaomi/MIUI, Huawei, Oppo aggressively kill background/accessibility
  services. Keeping the service alive on every brand is the real engineering slog (see
  dontkillmyapp.com). Plugged-in drawer phone is the friendly case.
- **Trust onboarding** — enabling accessibility shows a scary system warning; the install/consent
  moment is where normal users drop off. Brand + copy have to earn it.
- **Pixel-blind spots** — games, canvas/WebGL, `FLAG_SECURE` screens (banking) render blank to
  both tree and accessibility screenshot. Fine for most standard app UIs; a wall for a few.
- **Android 9–10** predate `takeScreenshot()` → UI-tree-only or MediaProjection fallback there.

---

## Sequencing (wedge-first, grow behind one contract)

The need is singular, so ship **one cell at a time** behind the single contract — each new
backend is expansion, not rescope.

1. **Prove the loop.** Relay + MCP contract (`screenshot/ui_tree/tap/type/run_command`) against
   a mock device → watch an agent step through screens.
2. **Android APK.** Accessibility service on a real Android 11+ phone; confirm reboot self-heal
   and taps/screenshots with zero clicks. Turns the last "assume" into "asserted."
3. **Trust layer.** Approval gate + control-tower dashboard.
4. **Commodity cells fall out.** Desktop CLI (SSH over relay) ≈ free; mac/Win/Linux GUI as needed.
5. **Only if we hit integrity/authenticity walls:** evaluate real-device vs cloud-phone (Redroid)
   for scale.

---

*Status: concept / product definition. Next concrete step: prove the relay ↔ MCP ↔ agent loop
against a mock device.*
