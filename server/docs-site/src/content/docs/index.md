---
title: What abacad is
description: abacad connects a real device — an Android phone, a Mac, a Linux box, or a browser tab — to a coding agent, so the agent can see the screen and act on it one step at a time, with a human approving. Honest about what ships today.
---

abacad connects a real device — an Android phone, a Mac, a Linux box, or a browser
tab — to an AI agent, so the agent can **see the screen and act on it**, one step at
a time, while a human supervises.

Agents can reason, but they have no hands. abacad gives them eyes and hands on a
device you already own: install once, grant one permission, and point your coding
agent at a single [MCP](https://modelcontextprotocol.io) endpoint.

:::note
This is **not** a remote-desktop tool for people. There is no smooth video mirror.
The core loop is **one screenshot plus the accessibility tree per step** — shaped for
an agent that acts one action at a time, with a human approving sensitive steps. Think
*control tower*, not remote desktop.
:::

## The control-surface ladder

Any device can be driven at one of four levels. A good agent stays as **high** as the
task allows and only drops down when it must:

```
1. API / programmatic   ← deterministic, structured, cheapest    (best)
2. CLI / shell          ← scriptable, token-cheap, reliable
3. Accessibility tree   ← semantic GUI, structured text
4. Screenshot + pixels  ← vision, slow, error-prone              (last resort)
```

abacad's leverage is the **semantic** rungs — the accessibility tree, which describes
the screen as structured elements (role, text, id, bounds) instead of raw pixels.
Pixel and coordinate operations are the escape hatch for when structure runs out
(canvas, WebView, games), not the primary interface.

## One contract across every device

Every device type exposes the **same** tool surface, so an agent written once drives a
phone, a laptop, or a browser tab without special-casing:

| Tool | Purpose | Rung |
|---|---|---|
| `run_command` / `read_output` | shell (desktops, via SSH) | CLI |
| `screenshot` | one frame plus the accessibility tree, on demand | pixels + accessibility |
| `tap` / `click` · `type` · `swipe` / `scroll` | inject input | — |
| `push_file` / `pull_file` | move files to/from the device | API |

Each action waits for the UI to settle and returns the resulting screen, so the agent
sees the effect of what it just did. The full list is in the
[tool reference](/docs/reference/tools/).

## How a device connects

The device **dials out** and holds one persistent WebSocket connection to the abacad
relay. The agent connects to the same relay, and the relay pipes bytes between them:

```
   your coding agent ──▶  abacad relay  ◀── device (dials out, holds the connection)
        (MCP)                  │
                     routes each command to the
                     right online device
```

Because the device only ever dials out, this works through home NAT and carrier CGNAT
with **no port-forwarding and no public inbound port** on the device. The relay is a
blind byte-mover — for an SSH session it moves ciphertext and never holds the session
keys (see [SSH access](/docs/guides/ssh/)).

## You stay in control

A human sets the device up, approves sensitive actions, and can take over or pull the
plug at any time. Supervision happens through the per-step loop — the agent proposes an
action, you can gate the risky ones — not through a live video feed.

## Platform support

What runs today, honestly marked. See [reading status markers](/docs/reference/status-markers/)
for what each symbol means.

| Platform | How it's driven | Status |
|---|---|---|
| **Android** | AccessibilityService (tree + input + screenshot) | ✅ shipped |
| **macOS** | AXUIElement + ScreenCaptureKit + CGEvent | 🟡 client built |
| **Linux** | X11 capture + XTEST input (AT-SPI tree next) | 🟡 client built |
| **Windows** | UI Automation + SendInput | 🔮 envisioned |
| **Browser** | a tab becomes the device; driven by the DOM + `execute` | ✅ shipped |
| **iOS** | not exposed by the OS to third-party apps | 🔮 envisioned |

Android is the furthest along: on Android 11+ a single accessibility permission gives
the agent the UI tree, input, and on-demand screenshots — a one-time setup, then the
phone can sit on a charger in a drawer and stay reachable. See
[running a phone hands-off](/docs/guides/running-hands-off/).

## What abacad is not

- **Not a live video mirror.** No continuous framebuffer, cursor stream, or audio — the
  loop is per-step, not real-time.
- **Not human take-over software.** Real-time driving by a person is out of scope; a
  human supervises through the step loop.

These are deliberate choices, not missing features — abacad is optimized for the
agent's eyes, not a person's.
