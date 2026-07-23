---
title: Tool reference
description: Every operation abacad exposes to an agent on a connected device — screenshot, tap, swipe, type, run_command, execute, push_file/pull_file — split by form factor, with an honest per-platform status for each.
---

Every operation abacad exposes to an agent on a connected device, and which platforms
support it — plus what we deliberately *don't* expose or support. Each marker is
per-platform; see [reading status markers](/docs/reference/status-markers/).

## How to read this

Capabilities sit on a **rung** of the [control-surface ladder](/docs/#the-control-surface-ladder).
A good agent stays as high as the task allows and only drops down when it must. abacad's
leverage is the **semantic** rungs (the accessibility tree); pixel/coordinate operations
are the escape hatch for when structure runs out (canvas, WebView, games).

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
| `composite` | accessibility / pixels | Run an ordered sequence of steps in one call — `click`/`right_click`, `drag`, `scroll`, text, key presses (incl. modifier-held clicks like ⌘-click), `wait(ms)`, and `screenshot`s — executed on-device with real timing. The primitive the named verbs are sugar over. | macOS 🟡 · Windows 🔮 · Linux 🟡 |
| Clipboard get / set | API | Read and write the device clipboard, both directions. | macOS 🟡 · Windows 🔮 · Linux 🔮 |
| TCP tunnel (`/connect`) | API | Raw TCP stream to a `host:port` reachable from the device — ssh, rsync, a DB client. | macOS 🟡 · Windows 🔮 · Linux 🟡 |
| `push_file` / `pull_file` | API | Read and write files on the device's filesystem by absolute path. The bytes ride the `/blobs` data plane over HTTP (the device fetches/posts with its own token), never the command socket, so multi-GB files are fine; the MCP layer stages the upload and inlines small text pulls so the agent never leaves the tool surface. Works headless (no display needed). | macOS 🟡 · Windows 🔮 · Linux ✅ |
| File transfer (`/blobs`) | API | The generic data plane the file verbs (and screenshots) move bytes over: HTTP upload / download of binary payloads by blob id, account-scoped. | Any ✅ |

On **Linux** the input + pixel rungs above are live (X11 capture, XTEST input), but
`screenshot` returns an **empty** UI tree for now — the AT-SPI semantic tree is the next
Linux build. So a Linux device is currently pixel-driven; prefer coordinates from the
screenshot over the (empty) tree. `input_text` types into the focused element rather than
replacing its contents, and Wayland sessions need XWayland.

### Browser

A browser tab acting as a device (open `<device-id>.abacad.ai`; the id in the Host is
the connection key). The tab *is* the surface — one document, no iframe — driven by the
semantic verbs plus `execute`, the JavaScript escape hatch.

| Capability | Rung | Description | Status |
|---|---|---|---|
| `execute` | API | Evaluate JavaScript in the device page and return the JSON result — the browser's power verb. Runs as an async function body (`return`, `await`), so it reads page state, acts by selector, and builds content in place (`document.body.innerHTML = …`). **A top-level navigation (`location.href = …`, or a link/submit that unloads the page) unloads the device client and drops the device offline until it is reopened.** The top rung of the ladder, for the one platform where the native automation API *is* JavaScript. | Browser ✅ |
| `screenshot` | accessibility / pixels | One frame (html2canvas, JPEG) plus a DOM-derived tree by default — elements with tag/role, text, id, clickable flag, and bounds. The page is its own surface, so pixels and tree are always available same-origin. | Browser ✅ |
| `click` / `scroll` / `input_text` | pixels / accessibility | The uniform cross-platform verbs, dispatched as synthetic DOM events into the page. Prefer `execute` for anything structured. | Browser ✅ |
| File transfer (`/blobs`) | API | Generic HTTP upload / download of binary payloads by blob id. | Any ✅ |

Deliberately **not** on a browser device: the nav keys (`back`/`home`/`recents`) and the
desktop OS verbs — a tab has no OS shell to drive, so it rejects them as unknown methods.

**Trust note:** a browser device can't touch the host machine (sandboxed), but `execute`
is *maximal* power over the surface's origin — it can read same-origin cookies/storage and
act as the logged-in user. Low risk to the machine, high power over the page: gate
`execute` accordingly.

## Not exposed to the agent

Behaviors and infrastructure that exist but are not agent tools.

| | Description | Platforms |
|---|---|---|
| Auto-wake on command | The device lights its own screen before acting and manages its own display timeout — power is the device's affair, not an agent tool. (The protocol intentionally has no `wake`/`sleep` methods.) | Android ✅ · macOS 🟡 |
| Per-device auth | Each device dials in with its own hashed token; the agent authenticates separately with a per-account MCP bearer. | All ✅ |

## Not supported

The human remote-desktop surface (a live VNC/RustDesk-style mirror) is deliberately out
of scope — abacad is one screenshot + tree per step for an agent, not a real-time mirror
for a person.

| | Why not |
|---|---|
| Live framebuffer / video stream | Continuous real-time screen mirror. The core loop is per-step, not a live feed. |
| Cursor shape / position | Only meaningful alongside a live video stream, which we don't provide. |
| Audio stream | Device audio to a human. Not agent-facing; out of scope. |
| Human take-over (live driving) | A human dropping into a live stream to drive in real time. Supervision happens through the per-step loop, not a live mirror. |
| Privacy mode (blank local screen) | Blacking out the physical display during a session. Out of scope. |
