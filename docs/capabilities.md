# Capabilities

Every operation abacad exposes to an agent on a connected device, and which platforms
support it — plus what we deliberately *don't* expose or support.

Read alongside [product.md](product.md) (the control-surface ladder and the why) and
[transport.md](transport.md) (which channel each capability travels on).

## How to read this

Capabilities sit on a **rung** of the control-surface ladder. A good agent stays as high
as the task allows and only drops down when it must:

```
1. API / programmatic   ← deterministic, structured, cheapest    (best)
2. CLI / shell          ← scriptable, token-cheap, reliable
3. Accessibility tree   ← semantic GUI, structured text
4. Screenshot + pixels  ← vision, slow, error-prone              (last resort)
```

The **Supported Platforms** column marks per-platform status:

- ✅ shipped and working
- 🟡 native client built — the desktop clients (macOS, Linux); working but not yet
  proven across the range of real end-user hardware/sessions
- 🔮 envisioned (in the vision matrix, not yet designed)
- — not applicable to that platform's form factor

Platform backends: **Android** = AccessibilityService · **macOS** = AXUIElement +
ScreenCaptureKit + CGEvent · **Windows** = UIA · **Linux** = XGB (GetImage) +
XTEST (input) today; AT-SPI semantic tree is the next Linux build.

abacad's leverage is the **semantic** rungs (the accessibility tree). Pixel/coordinate
operations are the escape hatch for when structure runs out (canvas, WebView, games) —
the same register RFB/VNC and RustDesk operate in, borrowed, not the primary interface.

---

## Exposed to the agent

The tool surface an agent drives, split by form factor.

### Mobile

| Capability | Rung | Description | Status |
|---|---|---|---|
| `screenshot` | accessibility / pixels | One frame (JPEG) plus the accessibility UI tree by default (`include_ui_tree`, default true) — foreground app + nodes with class, text, id, clickable flag, bounds. The tree is the semantic layer to reason over; set the param false for canvas/game screens. | Android ✅ · iOS 🔮 |
| `tap` | pixels | Tap at absolute pixel coordinates — the center of a target node's bounds. | Android ✅ · iOS 🔮 |
| `long_press` | pixels | Press and hold at coordinates (default 600 ms) — context menus, drag handles. | Android ✅ · iOS 🔮 |
| `swipe` | pixels | Swipe / fling gesture between two points — scroll a feed, navigate. | Android ✅ · iOS 🔮 |
| `input_text` | accessibility | Set the focused field's contents. Tap the field to focus it first, then call. | Android ✅ · iOS 🔮 |
| `press_keys` | accessibility / pixels | Navigation keys (Back, Home, Recents) plus text keys. | Android ✅ · iOS 🔮 |
| `composite` | accessibility / pixels | Run an ordered sequence of steps in one call — taps, `long_press`, `swipe`, text, key presses, `wait(ms)`, and `screenshot`s — executed on-device with real timing. Two wins: batch several actions plus a final screenshot into one round-trip, and express fine-grained input the flat verbs can't (multiple pointers run **concurrently** for multi-touch — pinch, rotate, path gestures). The primitive the named verbs are sugar over. | Android 🟡 · iOS 🔮 |
| Clipboard get / set | API | Read and write the device clipboard, both directions. | Android 🔮 |
| TCP tunnel (`/connect`) | API | Raw TCP stream to a `host:port` reachable from the device. | Android ✅ |
| `push_file` / `pull_file` | API | Read / write files on the device by path, over the `/blobs` data plane. Missing parent dirs are created automatically. Under scoped storage, writes are confined to the app's own external dir until the user opts in to **Files & media access** (All-files access, one toggle in the Setup checklist), which unlocks arbitrary shared-storage paths like `/sdcard/Pictures/…`; pushed media is media-scanned so it appears in the gallery. Shell-only paths (`/data/local/tmp`) stay adb-only — no app permission reaches them. | Android 🟡 |
| File transfer (`/blobs`) | API | Generic HTTP upload / download of binary payloads by blob id. | Android ✅ |

### Desktop

| Capability | Rung | Description | Status |
|---|---|---|---|
| `screenshot` | accessibility / pixels | One frame plus the accessibility UI tree by default (`include_ui_tree`) via AXUIElement + ScreenCaptureKit — windows/controls with role, text, id, bounds. Set the param false for canvas screens. | macOS 🟡 · Windows 🔮 · Linux 🟡 |
| `click` | pixels | Left click at absolute pixel coordinates, with optional modifier keys held. | macOS 🟡 · Windows 🔮 · Linux 🟡 |
| `right_click` | pixels | Right / secondary click to open a context menu. | macOS 🟡 · Windows 🔮 · Linux 🟡 |
| `drag` | pixels | Press, move, and release between two points — move a window, select a range, drag-and-drop. | macOS 🟡 · Windows 🔮 · Linux 🟡 |
| `scroll` | pixels | Wheel / two-finger scroll by a delta at a point. | macOS 🟡 · Windows 🔮 · Linux 🟡 |
| `input_text` | accessibility | Set the focused field's contents. Click the field to focus it first, then call. | macOS 🟡 · Windows 🔮 · Linux 🟡 |
| `press_keys` | accessibility / pixels | Full keyboard and modifier chords (⌘-C, ⌘-Tab, Esc). | macOS 🟡 · Windows 🔮 · Linux 🟡 |
| `composite` | accessibility / pixels | Run an ordered sequence of steps in one call — `click`/`right_click`, `drag`, `scroll`, text, key presses (incl. modifier-held clicks like ⌘-click), `wait(ms)`, and `screenshot`s — executed on-device with real timing. Two wins: batch several actions plus a final screenshot into one round-trip, and express fine-grained input the flat verbs can't (modifier-fused clicks, timing-sensitive sequences, multi-waypoint paths). The primitive the named verbs are sugar over. | macOS 🟡 · Windows 🔮 · Linux 🟡 |
| Clipboard get / set | API | Read and write the device clipboard, both directions. | macOS 🟡 · Windows 🔮 · Linux 🔮 |
| TCP tunnel (`/connect`) | API | Raw TCP stream to a `host:port` reachable from the device — ssh, rsync, a DB client. | macOS 🟡 · Windows 🔮 · Linux 🟡 |
| `push_file` / `pull_file` | API | Read and write files on the device's filesystem by absolute path. The bytes ride the `/blobs` data plane over HTTP (the device fetches/posts with its own token), never the command socket, so multi-GB files are fine; the MCP layer stages the upload and inlines small text pulls so the agent never leaves the tool surface. Works headless (no display needed). | macOS 🟡 · Windows 🔮 · Linux ✅ |
| File transfer (`/blobs`) | API | The generic data plane the file verbs (and screenshots) move bytes over: HTTP upload / download of binary payloads by blob id, account-scoped. | Any ✅ |

On **Linux** the input + pixel rungs above are live (XGB capture, XTEST input), but
`screenshot` returns an **empty** UI tree (`{pkg:"", nodes:[]}`) for now — the AT-SPI
semantic tree is the next Linux build. So a Linux device is currently pixel-driven;
prefer coordinates from the screenshot over the (empty) tree. `input_text` types into
the focused element rather than replacing its contents (no focused-field API without
AT-SPI), and Wayland sessions need XWayland — see `linux/README.md`.

### Browser

A browser tab acting as a device (open `<device-id>.abacad.ai`; the id in the Host is the
connection key). The tab *is* the surface — one document, no iframe — driven by
the semantic verbs plus `execute`, the JS escape hatch. The reach/depth trade is the whole
story — see [product.md](product.md).

| Capability | Rung | Description | Status |
|---|---|---|---|
| `execute` | API | Evaluate JavaScript in the device page and return the JSON result — the browser's power verb. Runs as an async function body (`return`, `await`), so it reads page state, acts by selector, and builds content in place (`document.body.innerHTML = …`). It always has full control because it runs in the device page itself. **But a top-level navigation (`location.href = …`, or a link/submit that unloads the page) unloads the device client and drops the device offline until it is reopened.** The top rung of the ladder, for the one platform where the native automation API *is* JavaScript. | Browser ✅ |
| `screenshot` | accessibility / pixels | One frame (html2canvas, JPEG) plus a DOM-derived tree by default — elements with tag/role, text, id, clickable flag, and bounds. The page is its own surface, so pixels and tree are always available same-origin. | Browser ✅ |
| `click` / `scroll` / `input_text` | pixels / accessibility | The uniform cross-platform verbs, dispatched as synthetic DOM events into the page. Prefer `execute` for anything structured. | Browser ✅ |
| File transfer (`/blobs`) | API | Generic HTTP upload / download of binary payloads by blob id. | Any ✅ |

Deliberately **not** on a browser device: the nav keys (`back`/`home`/`recents`) and the
desktop OS verbs — a tab has no OS shell to drive, so it simply rejects them as unknown
methods. `load`/`show` are folded into `execute` (`location.href = …` / `innerHTML = …`).

Trust note: a browser device can't touch the host machine (sandboxed), but `execute` is
*maximal* power over the surface's origin — it can read same-origin cookies/storage and act as
the logged-in user. Low risk to the machine, high power over the page: gate `execute` accordingly.

---

## Not exposed to the agent

Behaviors and infrastructure that exist but are not agent tools.

| | Description | Supported Platforms |
|---|---|---|
| Auto-wake on command | The device lights its own screen before acting and manages its own display timeout — power is the device's affair, not an agent tool. (The protocol intentionally has no `wake`/`sleep` methods.) | Android ✅ · macOS 🟡 |
| Per-device auth | Each device dials in with its own hashed token; the agent authenticates separately with a per-account MCP bearer. | All ✅ |

---

## Not supported

The human remote-desktop surface (RFB/VNC, RustDesk). Deliberately out of scope — abacad
is one screenshot + tree per step for an agent, not a real-time mirror for a person. A
human-takeover video stream is a possible premium add-on later, not the core.

| | Why not |
|---|---|
| Live framebuffer / video stream | Continuous real-time screen mirror. The core loop is per-step, not a live feed. |
| Cursor shape / position | Only meaningful alongside a live video stream, which we don't provide. |
| Audio stream | Device audio to a human. Not agent-facing; out of scope. |
| Human take-over (live driving) | A human dropping into a live stream to drive in real time — depends on the framebuffer above. Supervision happens through the per-step loop, not a live mirror. |
| Privacy mode (blank local screen) | Blacking out the physical display during a session (RustDesk-style). Out of scope. |
</content>
