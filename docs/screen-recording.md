# Screen recording

Continuous screen capture for a connected device — the moving-picture counterpart of
[`screenshot`](capabilities.md). One MCP tool, `screen_recording`, serves two needs
that share a capture pipeline but nothing else:

1. **A human observes** — the screen is streamed live to a person (VNC/RFB under the
   hood). Low latency matters; quality can degrade. Ephemeral.
2. **The agent keeps an artifact** — the screen is recorded on-device at the best
   possible quality (full resolution, full fps) and transferred afterward as a file, so
   the agent can build a demo/promo video, review a test run, etc. Quality matters;
   latency is irrelevant.

These have opposite priorities, so they are **two channels of one recording**, not one
knob. The screen is being recorded either way — the only question is *where it goes*:
to a live viewer, to a file, or both.

Read alongside [transport.md](transport.md) (the file lands on the data plane) and
[trust.md](trust.md) (a live session is a human input path that must fold into audit +
kill). The live channel is a **standard VNC path** — see
[The live channel: a standard VNC path](#the-live-channel-a-standard-vnc-path) — with its
media deliberately decoupled from the control plane and the `/connect` tunnel.

---

## The tool

`screen_recording` is the first **stateful** device tool: it manages an ongoing session
rather than firing a single action. So, unlike the single-shot verbs (`tap`, `click`),
the verb lives in an `action` parameter and the tool name is the **noun** it operates on
— mirroring `screenshot` (`screen` + `shot` → `screen` + `recording`).

`device_id` comes first, matching the convention across every device tool: the
required target selector always in the leading position, then the tool's own args.

```
screen_recording(
  device_id: <required — the device_id from list_devices; no default device>,
  action:  "start" | "stop" | "status",

  live: {                            # stream the recording to a human now
    enabled:     bool,
    mode:        "view" | "control",   # default "view"; control is optional/later
    ttl_seconds: int,                  # how long the viewer link stays valid (default 600)
    reason:      string                # short note shown to the operator
  },

  file: {                            # save the recording as a high-quality artifact
    enabled:              bool,
    fps:                  int,         # default = native/max
    format:               string,      # default "mp4" (H.264)
    max_duration_seconds: int          # safety cap
  }                                    # video only — no audio
)
```

Both channels are optional; either or both may be on. A device has **at most one active
recording at a time**, so `device_id` identifies the session completely — there is no
`session_id`. Artifacts are returned as self-describing `download_url`s, so there is no
`recording_id` either. (See [No identifiers](#no-identifiers).)

`screenshot` is unchanged and separate — it remains the per-step still the agent looks at
between actions. `screen_recording` is for continuous capture only.

### Channel names are semantic, not mechanism

`live` / `file` say what the channel is *for*. That the live channel is implemented with
VNC/RFB is an implementation detail the agent never needs — so the tool does not expose
`vnc` as a name. A future channel (e.g. an HLS broadcast, periodic thumbnails) slots in
as another nested object with **zero new tools**.

---

## Actions

### `start`

Turns on whichever channels are `enabled`, ensuring the on-device capture is running.
Idempotent: calling `start` again with a new channel adds it to the live session.

Returns a handle per enabled channel — no ids:

```jsonc
{
  "live": { "viewer_url": "https://…/watch/<ticket>", "mode": "view", "expires_at": "…" },
  "file": { "state": "recording" }
}
```

The agent hands `viewer_url` to its operator. It keeps issuing its normal input verbs
(`tap`/`click`/…) while `file` records in the background — recording is a passive capture,
orthogonal to input.

### `status`

Reports the current session, per channel. Used to poll a large upload to completion:

```jsonc
{
  "live": { "viewer_connected": true, "viewer_count": 1, "expires_at": "…" },
  "file": { "state": "uploading", "elapsed_seconds": 42, "size_bytes": 128034567,
            "download_url": null }
}
```

`file.state` walks `recording → stopped → uploading → ready` (or `failed`). When `ready`,
`download_url` is populated.

### `stop`

Ends the session — or, if only one channel object is passed, just that channel (stop
`file` but keep the human watching, or vice versa). Returns the finalized artifact:

```jsonc
{
  "file": {
    "duration_seconds": 42, "width": 2880, "height": 1800, "fps": 60,
    "size_bytes": 128034567, "codec": "h264",
    "transfer_state": "uploading",     // becomes "ready"; poll status for download_url
    "download_url": null
  }
}
```

---

## No identifiers

Both `session_id` and `recording_id` are deliberately absent.

- **No `session_id`** — one active recording per device means `device_id` already
  identifies the session. Threading a session id back and forth invents a uniqueness we
  don't have.
- **No `recording_id`** — the recording's identity only mattered for *fetching* and
  *cleanup*. Fetching uses the self-contained `download_url` (opaque token inside).
  Cleanup is **automatic**: record to a device temp file, transfer to the store on stop,
  delete the device temp after a successful transfer, and let the store expire the blob
  on a TTL. That removes on-device housekeeping — and with it the `list`/`discard`
  actions and the last need for an id.

If explicit enumeration/deletion is ever wanted, a recording is addressed by its
`download_url`, not by a separate opaque id.

---

## Agent ergonomics

```
# Record a test run to a file while driving the app:
screen_recording(action="start", file={enabled:true})
… agent taps/clicks through the flow …
screen_recording(action="status")            # poll → file.download_url when ready
screen_recording(action="stop")

# Let a human take a look, live:
screen_recording(action="start", live={enabled:true, reason:"stuck on 2FA — take a look"})

# Both at once — human watches while the agent films:
screen_recording(action="start", live={enabled:true}, file={enabled:true})
```

Three actions, one device key, self-contained URLs.

---

## Transport & quality

Each channel uses the sanctioned path for its payload — see [transport.md](transport.md):

- **`file`** is a blob. It is encoded on-device at full quality (no network pressure
  during capture), then moved device → store over the **HTTP data plane** and handed to
  the agent as a `download_url`. `stop`/`status` return a *reference*, never bytes — a
  full-res clip can be hundreds of MB and must not enter the agent's context or the
  control WebSocket.
- **`live`** is a stream, and it rides its **own dedicated connection** — never the
  control WebSocket or the `/connect` tunnel. It's a standard VNC path: a normal VNC
  server on the device *reverse-connects* out to a VNC repeater on abacad.ai, which
  bridges to a stock noVNC viewer in the browser. See
  [The live channel: a standard VNC path](#the-live-channel-a-standard-vnc-path).

---

## The live channel: a standard VNC path

The `live` channel is deliberately an **off-the-shelf VNC deployment**, not anything muxed
into abacad's existing channels. The control WebSocket is used *only* to turn it on and
off; the pixels flow on their own connection through standard components.

```
[device: standard VNC server]                          [browser: noVNC]
        │ reverse RFB dial-out (session token)                 │ RFB-over-WebSocket
        ▼                                                       ▼
[abacad.ai:   VNC repeater  ⇄  websockify  ⇄  ticketed noVNC page ]
```

- **Device** runs a normal VNC server (x11vnc on Linux, LibVNCServer on macOS/Windows,
  LibVNCServer compiled into the APK on Android) bound to `127.0.0.1`, and makes a
  **reverse VNC connection**
  outward to abacad.ai. Reverse-connect (`x11vnc -connect`, the UltraVNC-repeater model)
  is a standard VNC feature built exactly for the NAT case — the server dials the viewer
  instead of listening, so nothing ever has to reach the NAT'd device.
- **abacad.ai** runs a standard **VNC repeater + websockify**: it matches the device's
  reverse connection to a waiting browser by session token, and adapts RFB-over-TCP to
  RFB-over-WebSocket (what noVNC speaks).
- **Browser** loads stock **noVNC** at the ticketed `viewer_url`.

NAT still forces a relay hop through abacad.ai (the device can't be reached directly), but
that hop is a **standard VNC repeater**, not a custom tunnel — almost no bespoke code, and
the media path is fully isolated from the command/control plane.

**What the control channel does (only this).** On `start` with `live.enabled`, the server
mints a session token + viewer ticket and, over the command WebSocket, tells the device:
*start your VNC server and reverse-connect to the VNC ingress with token X*. It returns
`viewer_url`. On `stop`/kill, the command channel tells the device to drop its VNC server
and reverse connection, and the server tears down the repeater session. That start/stop
trigger is the whole extent of the control channel's involvement — no media on it.

**Auth.** The VNC server binds `127.0.0.1` and only ever speaks over the reverse connection
it dials, carrying the session token, so RFB's own (weak) auth is never exposed to the
network. The browser side is gated by the one-time viewer ticket.

---

## Trust integration

A `file` recording is a passive capture and needs nothing beyond normal transport. The
`live` channel introduces a human input path and must fold into abacad's
integrity + audit + kill posture ([trust.md](trust.md)):

1. **Scope-gated.** `screen_recording` is high-privilege (a live session can hand a human
   real-time control). It flows through the same per-key method allowlist as every other
   tool — a key without the scope never sees it.
2. **Kill reaches it.** The server owns the repeater session, so killing it (or `stop`, or
   the TTL) tears down the browser↔repeater side at once; the command channel also tells
   the device to drop its VNC server and reverse connection. Either half dying ends the
   session.
3. **Audited at the boundary.** Individual RFB keystrokes aren't logged, but the session
   *boundaries* are first-class audit events: who opened it, when, `reason`, `mode`, and
   when a viewer connected/left. The audit trail can always answer "was there a human
   takeover, when, and why."
4. **No input fight.** While a `control` session is live the device is marked
   "under takeover"; the agent's own input verbs soft-fail with a clear reason so agent
   and human don't fight the cursor. `view` mode needs no such lock.

---

## Platform capture backends

Each `file`-channel implementation records the display to a temp `.mp4` (H.264), then
uploads it to `/blobs` on stop via the client's existing blob path; the temp is deleted
after a successful transfer. The `live` column is the planned RFB approach (not yet built).

| Platform | `file` capture/encode (shipped) | Build status | `live` (planned) |
|----------|----------------------------------|--------------|------------------|
| macOS    | ScreenCaptureKit `SCStream` → `AVAssetWriter` H.264 | compiles + links (Mac mini) | LibVNCServer |
| Linux    | `ffmpeg` x11grab → libx264 (shells out) | builds + vets | x11vnc / LibVNCServer |
| Windows  | `ffmpeg` gdigrab → libx264 (shells out) | code-complete (no .NET to compile) | LibVNCServer or managed RFB |
| Android  | MediaProjection → `MediaRecorder` H.264 | compiles (Gradle, Mac mini) | LibVNCServer in-app (code-complete, unverified) |

Notes:
- **macOS** reuses the Screen Recording permission `screenshot` already requests.
- **Linux / Windows** shell out to **ffmpeg** (x11grab / gdigrab) — the encoder no pure-Go /
  managed path provides — and report a clear error if ffmpeg isn't on `PATH`.
- **Android** needs a per-session MediaProjection consent dialog (the one break from silent
  operation) and a `mediaProjection` foreground-service type while recording; `start` is
  async (returns `requesting_permission`, then `recording` once the user consents).

`live` uses each platform's VNC server in standard **reverse-connect** mode (dialing the
abacad.ai repeater), so the server side is stock **repeater + websockify + noVNC** with
almost no bespoke code. Desktops unify on **LibVNCServer** (portable C) / **x11vnc**, fed
by the existing capture + input; Android **compiles LibVNCServer straight into the APK**
(no droidVNC-NG companion), fed by MediaProjection frames pushed over JNI — view-only for
now, with viewer input (AccessibilityService injection) as a follow-on.

---

## Deliberately out of scope (v0)

- **Live control on every platform.** `live.mode` ships as `view`; `control` is additive
  (a flag + input injection that already exists) and can land per-platform later.
- **`list` / `discard`.** Superseded by automatic retention. Add only if manual
  management proves necessary.
- **Concurrent recordings per device.** One at a time keeps `device_id` sufficient as the
  session key.
- **Streaming the file to the agent live.** The file is an after-the-fact artifact by
  design; the live channel is the only real-time path.
- **Audio.** Recordings are video only. The `file` channel could mux audio later, but
  the `live` channel can't — RFB/VNC carries no audio — so audio would be asymmetric
  across channels; skipped for now.
