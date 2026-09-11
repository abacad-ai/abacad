# Session replay

Watch back what an agent did on a device: the frames it saw, in order, with each
action marked on the frame it acted on.

The [activity trail](trust.md#observability--revocation) already answers *what
commands ran*. Replay answers *what the screen looked like while they ran* — the
picture track of the same record. It exists for the moment after the fact, when
an agent did something surprising and "tap at (540, 1180), ok, 120ms" is not
enough to tell you what it touched.

Read alongside [trust.md](trust.md), which places replay in the thin,
non-semantic sliver abacad actually owns: it observes, it does not judge.

---

## Off by default. Per device. With attestation.

Recording is **off for every device** until its owner turns it on in the
dashboard, under **Setup → Session replay**, behind an attestation that says
plainly what is about to happen:

> I understand that screenshots of this device will be stored on the server until
> they age out of the retention window, and that anyone who can sign in to this
> account can watch them.

This is deliberately higher friction than the rest of the audit surface. The
trail is automatic because a log line is cheap and its absence is worse than its
presence. Frames are not cheap: they are pictures of a real screen — a signed-in
inbox, a bank app, whatever the phone in the drawer was showing — and keeping
them is a promise that has to be made on purpose.

**Both directions are recorded.** Turning recording on writes a consent row;
turning it *off* writes one too. An audit surface that can be switched off
silently is worth less than one that cannot be switched off at all, so the trail
always shows when a recording started and when it stopped.

Turning recording off stops it immediately — on the live connection, not at the
agent's next call — and does **not** delete what was already recorded. Erasing
history as a side effect of flipping a switch is the opposite of what an audit
surface is for; the retention window is what removes frames.

---

## What is recorded

Every command relayed to a device with recording on becomes one **step**:
timestamp, method, outcome, duration, and which credential issued it.

Only two verbs return pixels — `screenshot` and `composite` — so only those steps
carry a frame of their own. An action (`tap`, `click`, `swipe`, `input_text`, …)
returns nothing but `{dispatched: true}`.

So an action is recorded as a **frameless step**, and its marker is drawn over
the frame **before** it — the screen it acted on, and the only one its
coordinates mean anything against. To also show what it *produced*, the recorder
takes **its own screenshot** a moment later. A session reads:

```
[frame] ──tap ●──▶ [frame after] ──swipe ●──▶ [frame after] ──▶ …
```

### The follow-up capture, and its rails

This is the one place in abacad where an observability feature **sends a command
to a device**. It is bounded on every side:

- **Only after a successful action.** A tap that timed out or was denied changed
  nothing, so there is nothing new to photograph. The verb list is an allowlist:
  `tap`, `long_press`, `swipe`, `input_text`, `back`, `home`, `recents`, `click`,
  `right_click`, `drag`, `scroll`, `press_keys`, `execute`. Not `screenshot` or
  `composite` (they carry their own pixels), not the file/recording/vnc verbs
  (they change no screen).
- **Never on a sleeping device.** A command auto-wakes a device, and a phone in a
  drawer must not be woken by its own supervision.
- **Never without the `screenshot` capability.** Checked before sending, not left
  to the relay's gate — a denial is an audit row, and filling an owner's trail
  with denials they never caused is worse than not capturing.
- **One in flight per device.** A burst of actions resets the timer rather than
  stacking captures: five taps in a row yield one picture of where they ended.
- **Never when the agent got there first.** After the action the recorder waits
  `ABACAD_REPLAY_SETTLE_MS` (default 800) for the UI to settle, then checks
  whether a frame has already landed for that device. The usual agent loop is
  `tap` → look, so in the common case this fires *nothing*.
- **Never for an unchanged screen.** If the captured frame is byte-identical to
  the one before it, nothing is recorded — no row, no file. Only the recorder's
  own captures are deduped this way; an agent's screenshot of a static screen is
  still a command it really issued, and the timeline has to show it.
- **Best-effort.** A capture that fails is dropped silently and never retried: by
  the time a retry landed, the screen would no longer be the result of the action
  that triggered it.

**It appears on the activity trail, as itself**, with `source: replay` alongside
`agent`/`dashboard`/`ssh`/`tunnel`, and the Activities page can filter on it.
Hiding it would leave the server's record disagreeing with the device's own logs,
and would let a supervision feature quietly inflate a device's command count.

`ABACAD_REPLAY_CAPTURE_ACTIONS=0` turns the whole thing off without turning
recording off; actions then record as frameless steps and nothing extra is ever
sent to a device.

> The cleaner long-term answer is for the device to return its resulting frame
> *with* the action — which is what `product.md` already describes and no client
> implements yet. That is a protocol change across five clients; when it lands,
> the reply will carry pixels and the follow-up will skip itself as already
> satisfied, with no change to anything below.

### What is *not* recorded

- **The UI tree.** It rides along in every screenshot reply and is often larger
  than the JPEG. It is the agent's working data, and it adds nothing to a picture
  of the screen it describes.
- **Anything from a command's parameters except pointer coordinates.** Markers
  are built from an allowlist — `x`, `y`, `x1/y1/x2/y2`, as numbers, for pointer
  verbs only. Text typed with `input_text`, JavaScript passed to `execute`, shell
  arguments, file paths: none of it is stored. The allowlist is the point; a
  denylist would leak the next verb somebody adds.
- **Dashboard activity.** The live view captures a frame every two seconds for as
  long as a device page is open. Recording those would bury the agent's own
  frames under an order of magnitude more of them — and nobody replays a session
  they were watching live.

---

## Sessions

There is **no session id**, on the wire or anywhere else. An agent never
announces that it has started or finished a task, so a session boundary is not
something abacad is told; inventing an id would assert a boundary nobody
declared.

Instead a session is a **gap**: a device's steps split wherever more than three
minutes pass with nothing recorded. That is far longer than the pause between two
steps of one task and far shorter than the pause between two tasks. Sessions are
addressed by their time bounds, which is also what identifies them in the UI.

Playback advances **by step, not by wall clock**. An agent session is mostly
waiting — a screenshot, thirty seconds of the model thinking, a tap — so real-time
playback would be minutes of a frozen picture. Each step holds for a fixed beat
and its true elapsed time is printed instead (`+34s since previous`), which is
the information the wall clock was carrying anyway.

---

## Retention

Two bounds, because the window alone does not bound disk — one busy agent can
record thousands of frames well inside it:

| Setting | Default | Meaning |
|---|---|---|
| `ABACAD_REPLAY_RETENTION_HOURS` | `48` | delete steps and frames older than this (`0` = keep forever) |
| `ABACAD_REPLAY_MAX_STEPS_PER_DEVICE` | `4000` | keep at most this many steps per device, oldest dropped first (`0` = unlimited) |
| `ABACAD_REPLAY_CAPTURE_ACTIONS` | `1` | take a screenshot after each action (see above); `0` leaves actions frameless |
| `ABACAD_REPLAY_SETTLE_MS` | `800` | how long to let the UI settle before that capture |
| `ABACAD_REPLAY` | `replay` | directory frame bytes live in |

Both sweeps run every 15 minutes. Deleting a device deletes its steps and frames
immediately — its frames are pictures of *its* screen, and they must not outlive
it waiting for a window to close.

Rows are deleted before their files, so a crash in between would strand bytes
with no row left to find them by. A third sweep removes any frame file older than
the retention window regardless of what the database says, which makes the window
a promise about the disk rather than about the database.

The step cap is the one that moved when follow-up capture arrived: an action now
costs a step *and* usually a frame, so a session is roughly twice the rows it
used to be, and the cap doubled with it so the window still covers the same
number of sessions.

Sizing it is arithmetic on one number nobody has measured across real devices —
a captured screen is typically 100–300 KB of JPEG — so treat this as a bound to
check against your own fleet rather than a figure: at the 4000-step cap, with
about half of those steps carrying a frame, a device's recordings sit somewhere
around 200 MB–600 MB. Frames are stored exactly as the device sent them; there is
no downscaling pass yet.

The startup banner prints all of it, so an operator running with `keep forever`
or `unlimited` can see it:

```
session replay     : replay (off per device until enabled; kept 48h, max 4000 steps/device, frame 800ms after each action)
```

---

## Cost when it is on

Recording is gated on a per-device flag mirrored onto the live connection, so a
device that is not recording pays **nothing** on the command path — no channel
send, no retained payload, no branch beyond an atomic load.

When it is on, the recorder hands each payload to a buffered channel and returns;
a single goroutine decodes the frame, writes the JPEG and inserts the row. A full
buffer **drops the step** rather than stalling the relay, the same trade the
activity trail makes: replay is an observability surface, and a slow disk must not
become a slow agent. Drops are counted and logged, and a dropped step shows up as
a gap in the timeline.

The activity row is written first and unconditionally, so a wedged recorder can
never cost an account its audit trail.

---

## Reading it back

Three endpoints, in the order a player uses them. All are session-authenticated
and scoped to a device the caller owns.

```
GET /api/devices/{id}/replay/sessions        → what sessions exist (+ whether recording is on)
GET /api/devices/{id}/replay?from=&to=       → the steps of one, oldest first
GET /api/devices/{id}/replay/frames/{frame}  → one frame, as a JPEG
```

A frame id is **not** a capability: its row names the device and account that own
it, and both must match the caller. A mismatch is `404`, never `403` — an id's
existence must not leak across accounts.

Frame ids are immutable, so frames are cacheable (`private, max-age=3600,
immutable`) — which is what makes scrubbing back and forth over a session smooth.
The live `/screenshot` endpoint is `no-store` for the opposite reason: its URL's
content changes underneath it.
