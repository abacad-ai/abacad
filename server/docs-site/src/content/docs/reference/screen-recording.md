---
title: Screen recording
description: The screen_recording MCP tool — continuous device capture with two channels, a high-quality file artifact the agent keeps and an optional live view for a human. One stateful tool, per-platform status marked honestly.
---

Continuous screen capture for a connected device — the moving-picture counterpart of
[`screenshot`](/docs/reference/tools/). One MCP tool, `screen_recording`, serves two
needs that share a capture pipeline but nothing else:

1. **The agent keeps an artifact** — the screen is recorded on-device at full quality
   (resolution + fps) and transferred afterward as a file, so the agent can build a
   demo video, review a test run, etc. Quality matters; latency is irrelevant.
2. **A human observes** — the screen is streamed live to a person. Low latency
   matters; quality can degrade. Ephemeral.

These have opposite priorities, so they are **two channels of one recording**, not one
knob. The screen is being recorded either way — the only question is *where it goes*:
to a file, to a live viewer, or both.

## The tool

`screen_recording` is a **stateful** device tool: it manages an ongoing session rather
than firing a single action. So the verb lives in an `action` parameter and the tool
name is the **noun** it operates on — mirroring `screenshot`.

```
screen_recording(
  device_id: <required — from list_devices; no default device>,
  action:  "start" | "stop" | "status",

  file: {                            # save the recording as a high-quality artifact
    enabled:              bool,
    fps:                  int,         # default = native/max
    format:               string,      # default "mp4" (H.264)
    max_duration_seconds: int          # safety cap
  },                                   # video only — no audio

  live: {                            # stream the recording to a human now
    enabled:     bool,
    mode:        "view" | "control",   # default "view"
    ttl_seconds: int,                  # how long the viewer link stays valid (default 600)
    reason:      string                # short note shown to the operator
  }
)
```

Both channels are optional; either or both may be on. A device has **at most one
active recording at a time**, so `device_id` identifies the session completely — there
is no `session_id`, and artifacts come back as self-describing `download_url`s, so
there is no `recording_id` either.

`screenshot` is unchanged and separate — it remains the per-step still the agent looks
at between actions. `screen_recording` is for continuous capture only.

### Actions

- **`start`** — turns on whichever channels are `enabled`, ensuring on-device capture
  is running. Idempotent. Returns a handle per channel (a `viewer_url` for `live`, a
  `state` for `file`). The agent keeps issuing its normal input verbs while `file`
  records in the background.
- **`status`** — reports the current session per channel. `file.state` walks
  `recording → stopped → uploading → ready`; when `ready`, `download_url` is populated.
- **`stop`** — ends the session (or just one channel, if only that channel object is
  passed). Returns the finalized artifact metadata (`duration`, `width`/`height`,
  `fps`, `size`, `codec`) and, once transferred, the `download_url`.

### Agent ergonomics

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

## Transport

Each channel uses the sanctioned path for its payload (see
[transport](/docs/reference/transport/)):

- **`file`** is a blob: encoded on-device at full quality, then moved device → store
  over the **HTTP data plane** and handed to the agent as a `download_url`. `stop` and
  `status` return a *reference*, never bytes — a full-res clip can be hundreds of MB
  and must not enter the agent's context or the control socket.
- **`live`** rides its **own dedicated connection** — never the control socket or the
  `/connect` tunnel. It's a standard VNC path: a VNC server on the device bound to
  loopback *reverse-connects* out to a repeater on the server, which bridges to a
  stock noVNC viewer in the browser. The control socket is used *only* to start and
  stop it — no media ever flows on it. Because the VNC server only speaks over the
  reverse connection it dials, it's never exposed to the network; the browser side is
  gated by a one-time viewer ticket.

## Trust integration

A `file` recording is a passive capture and needs nothing beyond normal transport. The
`live` channel introduces a human input path, so it is treated as high-privilege (see
[security](/docs/security/)):

- **Scope-gated** — it flows through the same per-key method allowlist as every other
  tool; a key without the scope never sees it.
- **Bounded** — the session ends on `stop`, on its TTL, or when either half of the
  connection drops.
- **Session boundaries are auditable** — who opened it, when, `reason`, `mode`, and
  when a viewer connected/left (individual keystrokes are not logged).
- **No input fight** — while a `control` session is live the agent's own input verbs
  soft-fail with a clear reason, so agent and human don't fight the cursor.

## Platform support

The `file` channel records the display to an on-device `.mp4` (H.264), then uploads it
on stop. The `live` channel is the planned VNC path. Status is per platform — see
[reading status markers](/docs/reference/status-markers/).

| Platform | `file` capture/encode | `live` |
|---|---|---|
| Android | MediaProjection → MediaRecorder (H.264) — ✅ | 🔮 planned |
| macOS | ScreenCaptureKit → AVAssetWriter (H.264) — 🟡 | 🔮 planned |
| Linux | ffmpeg x11grab → libx264 — 🟡 | 🔮 planned |
| Windows | ffmpeg gdigrab → libx264 — 🔮 | 🔮 planned |

Notes:

- **macOS** reuses the Screen Recording permission `screenshot` already requests.
- **Linux / Windows** shell out to **ffmpeg** and report a clear error if it isn't on
  `PATH`.
- **Android** needs a per-session capture-consent dialog (the one break from silent
  operation) while recording; `start` is async (returns a permission-request state,
  then `recording` once the user consents).

## Deliberately out of scope (for now)

- **Live control on every platform.** `live.mode` ships as `view`; `control` is
  additive and can land per-platform later.
- **Concurrent recordings per device.** One at a time keeps `device_id` sufficient as
  the session key.
- **Audio.** Recordings are video only — a live VNC path carries no audio, so audio
  would be asymmetric across channels; skipped for now.
