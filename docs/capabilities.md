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

## Turning capabilities off

Everything below is what a device *can* do. Which of it a device *does* expose is
per-device configuration, set by its owner in the dashboard (device → Access → "What
this device exposes") or via `PATCH /api/devices/{id}` with a `capabilities` array.

- **Default is everything**, including capabilities added in later versions.
  Enrollment is the authorization ([trust.md](trust.md)); this narrows an
  already-trusted device rather than gating a new one.
- **It binds every caller**, not just agents — the dashboard's own live view and
  screenshot go through the same gate. Enforcement is at the relay
  (`DeviceConn.Send` / `OpenStream`), the only point every path crosses.
- **Only a signed-in human can change it.** An API key cannot widen its own reach
  (trust.md principle 2), and each change is recorded on the activity trail.
- **Refusals are visible.** A denied call returns an error naming the capability and
  is logged with `outcome=denied`, so probing what a device exposes leaves a trace.
- **Narrowing takes effect immediately**, including on sessions already running:
  revoking the tunnel closes open streams, and revoking live view stops it.

The dashboard groups the switches; storage is per protocol method.

| Switch | Covers | Worth knowing |
|---|---|---|
| See the screen | `screenshot`, `screen_recording` | A screenshot also returns the accessibility tree — the text of every on-screen field, not just pixels. |
| Control input | `tap`, `long_press`, `swipe`, `input_text`, `back`, `home`, `recents`, `click`, `right_click`, `drag`, `scroll`, `press_keys`, `composite` | On a desktop this is effectively full control: anything that can type can open a terminal. |
| Read and write files | `push_file`, `pull_file` | Writing files is equivalent to full control — it can add an SSH authorized key or a startup entry. |
| Run JavaScript | `execute` | Full power over the page's origin, acting as the logged-in user. |
| Live view | `vnc` | **Not read-only** — the RFB channel carries keyboard and pointer events back to the device. |
| Network tunnel | `/connect` | The broadest switch here. See below. |
| SSH | the jump host | Reaches the device's own `127.0.0.1:22`. |

**These are not peers.** The network tunnel reaches any port the device can reach,
*including its own* — sshd, an ADB port, a Chrome DevTools port (JS evaluation and
screen capture), a local database. So a tunnel grants in substance what the narrower
switches grant, whatever those are set to. In particular, turning SSH off while
leaving the tunnel on does **not** close SSH: a tunnel dials `127.0.0.1:22` directly.
The dashboard warns about exactly this combination.

`composite` is authorized step by step rather than as a single verb, so a permitted
`composite` cannot smuggle a denied screenshot or keystroke.

### The device gets a vote too

The switches above are the **account grant**, set from the dashboard. A device can also
declare its own **ceiling**, set locally on the device itself, and the effective surface
is the intersection — either side may narrow, neither may widen.

The device reports its ceiling over the command socket (a `capabilities` frame on
connect and again on every local change, always the full set), the dashboard shows it,
and the relay stops sending what the device refuses. But the report is advisory and the
refusal is not: the device checks its own ceiling before acting, so a relay that never
received the frame, ignored it, or was lying gets the same answer. That is what makes
"even the relay cannot turn this back on" true rather than aspirational.

Where each client puts the switches, and where it stores them:

| Client | Set it in | Stored in |
|---|---|---|
| Linux | `abacad capabilities` (see below), or the desktop app's panel | `~/.config/abacad/capabilities` |
| macOS | menu-bar panel → "What this Mac exposes", or `abacad capabilities` | login Keychain |
| Android | app screen → "What this device exposes" | app-private `SharedPreferences` |
| Windows | tray menu → "What this PC exposes", or `abacad capabilities` | DPAPI-encrypted, per user |
| Browser | badge → "Limits" | `localStorage` for that device's origin |

Every desktop platform has the CLI form, and it reads and writes the same store as
that platform's panel — a machine you administer over ssh is not a machine you can
only pair and never constrain. The flags and the group names are identical across
Linux, macOS and Windows:

```console
$ abacad capabilities                      # what is on and off
$ abacad capabilities --disable files,ssh  # groups or individual names
$ abacad capabilities --only screenshot    # exactly this, nothing else
$ abacad capabilities --none               # expose nothing
```

In every case a missing stored value means no ceiling, so an existing install behaves
exactly as it did before.

**A client that reports nothing imposes no ceiling.** Silence means *unspecified*, not
*denied* — reading it as a refusal would take every device on an older build offline the
moment the relay upgraded — so those devices are governed by the account grant alone,
exactly as before. The dashboard shows which devices have reported, so "this device has
no opinion" and "this device exposes nothing" stay distinguishable rather than looking
identical.

The browser is the weak one, and worth saying plainly: its ceiling lives in
`localStorage` and `execute` runs arbitrary JS in that same page, so a tab still exposing
`execute` can rewrite its own limits. Turning `execute` off is meaningful; leaving it on
and expecting the rest to hold is not.

A ceiling cannot constrain a capability that already grants code execution as the device
user: a device still exposing file-write is one `push_file` from having its own config
rewritten. See [trust.md](trust.md#the-ceilings-hard-limit).

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
