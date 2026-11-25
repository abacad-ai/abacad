# iosvnc

Expose a paired iPhone or iPad as a **standard RFB (VNC) endpoint** — no
jailbreak, no app on the phone.

Two processes, split on purpose:

```
browser / abacad ──RFB──▶ iosvnc (Go)                    one viewer per session
                            │  length-prefixed JPEG  ↑
                            │  JSON input commands    ↓
                          bridge/iosbridge.py (Python, pymobiledevice3)
                            └─ CoreDevice: HEVC stream, HID touch, keep-awake
```

**Why the split.** iOS only delivers HID reports from a process that holds a
display stream, and the digitizer surface is matched asynchronously *after* that
stream settles. Splitting the stream and the input across processes produces a
state that works intermittently — the failure mode that started this project.
So one process owns the whole device session (Python, where pymobiledevice3
lives), and everything that is commodity and stable — RFB framing, encodings,
scaling, auth — stays in Go.

**Why not reuse an existing VNC server for iOS.** `pymobiledevice3 ... serve-vnc`
exists, and we tried it: it offers only Raw encoding (12 MB per frame at native
size), ignores the client's `SetPixelFormat` (red and blue come out swapped),
opens HID outside the auth context (input is dropped), waits for the first frame
before sending the RFB banner, and has no keep-awake or reconnect. All five are
fixed here.

## What it does today

| | |
|---|---|
| Frame rate | 25–40 fps while the screen changes |
| Frame size | ~78 KB (Tight/JPEG, 592×1280 for an iPhone 15 Pro) |
| Input | tap, drag, swipe (a press shorter than 80 ms is ignored by iOS; the bridge enforces the dwell) |
| Keep-awake | a `PreventUserIdleSystemSleep` assertion on the device, renewed every 2 min |
| Streaming lifetime | RTCP receiver reports every second — without them the device reaps the video session after ~25 s |
| Loss handling | RTP sequence gaps drop the corrupt access unit and request a fresh IDR |
| Session lifetime | supervised: the bridge is restarted after a dropped tunnel, a crash or a wedged process, and viewers are disconnected the moment it is gone |

**Not done yet: keyboard input.** Tapping and dragging work; typing does not.

**A dead session is never shown as live.** A frozen frame and a still screen are
indistinguishable, so the Go side treats the bridge as the source of truth:
when it ends, `Wait` returns an error, every RFB connection is closed, and new
ones are refused until a fresh session produces a frame. Two failures are
covered — the process exits (pipe EOF) and the process wedges (no frames and no
log line for 15 s, killed by a watchdog). A quiet screen is not a wedge: a still
screen sends no frames, but the bridge logs a stats line every two seconds, so
the heartbeat keeps running.

The Python side exits with `os._exit` on any fatal error rather than unwinding:
its shutdown joins VideoToolbox threads, and a bridge that lingers after a
dropped tunnel keeps its stdout open — which is exactly how a viewer ends up
staring at yesterday's screenshot while the status says connected.

## Requirements

- macOS with **Xcode 27**, a device **paired**, **Developer Mode** on, **unlocked**
  and on **USB**.
- Python 3.9+ with pymobiledevice3:

  ```bash
  python3 -m venv .venv && .venv/bin/pip install pymobiledevice3
  ```

- No root: the bridge uses the in-process userspace tunnel and falls back to the
  native (remoted-piggyback) one only if that fails. The native path is tried
  second on purpose — when it fails it takes 60 s to time out.

## Run

```bash
./serve-ios.sh                          # 0.0.0.0:5970
./serve-ios.sh 192.168.1.10:5970 --password Abcd1234
```

Then add it to abacad as a device: **Host** the Mac's address, **Port** 5970,
**Sign-in → VNC password**, **Platform → iOS**.

## Diagnostics

`iosvnc devices`, `iosvnc probe` and `iosvnc doctor` predate the bridge: they use
go-ios and the DVT screenshot service, which the bridge no longer relies on. They
are still useful for answering "is the device reachable at all", but they are
the legacy path in this tree.

## Tests

```bash
go test ./...        # RFB wire format, session supervision, input mapping, doctor
```

The device transport itself needs hardware; `bridge/iosbridge.py` is exercised by
running the bridge and watching the frames it emits.
