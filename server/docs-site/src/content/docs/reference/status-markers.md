---
title: Reading status markers
description: abacad's docs mark every capability with an honest per-platform status — shipped, client built but unproven, or envisioned — so you always know what actually runs today versus what's planned.
---

abacad is built one device type at a time, so a capability can be live on one platform
and still on the roadmap for another. Every reference table marks status **per
platform** with these symbols. We'd rather under-promise than surprise you.

| Marker | Meaning |
|---|---|
| ✅ | **Shipped and working** — implemented and exercised on that platform. |
| 🟡 | **Client built, not yet proven** — the native client exists and works, but hasn't been validated across the range of real end-user hardware and OS versions. Mostly the desktop clients (macOS, Linux). |
| 🔮 | **Envisioned** — in the plan, not yet designed or built. |
| — | **Not applicable** to that platform's form factor. |

When a row reads `Android ✅ · iOS 🔮`, it means the capability is real on Android today
and planned for iOS. If you're evaluating abacad for a specific platform, read the
marker for **that** platform — not the row as a whole.

Platform backends, for reference:

- **Android** — AccessibilityService
- **macOS** — AXUIElement + ScreenCaptureKit + CGEvent
- **Linux** — X11 capture (GetImage) + XTEST input today; a semantic AT-SPI tree is the
  next Linux build
- **Windows** — UI Automation
- **Browser** — the DOM, same-origin
