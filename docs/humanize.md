# Humanize — human-like pointer motion (client spec)

This is the single source of truth for the "humanize" behavior. Each client
(Android, Linux, macOS, Windows, browser) is a **faithful port of the constants
and formulas below** — keep them in sync here first, then mirror into each
client. The reference implementation is Android's `Humanize.kt`; this document
generalizes it and adds the desktop pointer-approach section that touch devices
don't need.

## Why

Behavioral bot-detection is a statistical classifier: it asks whether the
*distribution* of your input matches a human's. Raw agent commands fail in four
ways — every action lands on the exact same pixel, is held for a fixed time,
moves in a straight line at constant speed, and follows the previous action with
zero pause. Each is a distribution with variance ≈ 0, which no hand produces. We
sample from the distributions a real hand produces instead.

Crucial subtlety: **uniform `rand(a,b)` is itself a fingerprint** — a flat
distribution is as unhuman as a constant one. Human durations are **log-normal**
(a floor, a common case, a long tail), so every duration below is log-normal.

## Delivery & default

Generation is **pure-client**. The server stores a per-device `humanize` flag
(default **on**) and folds it into each pointer command's params
(`"humanize": true|false`). The client reads it and branches:

- `humanize == true`  → synthesize the motion below.
- `humanize == false` → the client's existing exact/teleport behavior.
- **param absent**     → treat as **true** (default-on, and graceful against an
  un-upgraded server).

## Primitives

- `gaussian()` → sample N(0,1). (Go `rand.NormFloat64`, Swift/C# Box–Muller,
  JS Box–Muller, Kotlin `Random.nextGaussian`.)
- `logNormalMs(median, sigma, lo, hi)` = `clamp(median * exp(gaussian()*sigma), lo, hi)`,
  result in integer milliseconds.

## Shared constants (identical across all clients)

| Name              | Formula / value                                             | Meaning |
|-------------------|-------------------------------------------------------------|---------|
| positional jitter | `coord + gaussian()*σ`, **σ = 4.0 px**                       | never repeat a pixel |
| tap/press hold    | `logNormalMs(75, 0.35, 45, 140)`                            | finger/button down time |
| duration jitter   | `duration*(1 + gaussian()*0.12)`, clamp `[0.5×, 1.6×]`, min 1 | perturb a requested hold/swipe |
| pre-action dwell  | `logNormalMs(70, 0.55, 25, 350)`                            | "think time" before an action |

## Swipe / drag trajectory (bowed Bézier)

From `(x1,y1)`→`(x2,y2)`:

1. Jitter both endpoints (σ = 4.0).
2. `dx,dy` = end − start; `len = max(1, hypot(dx,dy))`; unit perpendicular
   `(px,py) = (-dy/len, dx/len)`.
3. Control point at the midpoint, bowed sideways + a little axial slack:
   - `bow   = gaussian() * len * 0.08`  (sign random, ~8% of length)
   - `slide = gaussian() * len * 0.05`
   - `C = midpoint + (px,py)*bow + (dx,dy)/len*slide`
4. `steps = clamp(len/25, 12, 28)` — one sample per ~25 px.
5. `tremor = min(2.5, len*0.01)`.
6. Quadratic Bézier `B(t) = u²·P0 + 2ut·C + t²·P1` (`u = 1-t`) sampled at
   `t = i/steps`, `i = 1..steps`. Add `gaussian()*tremor` to every intermediate
   point **except the last** (the stroke must land exactly on the jittered
   target).

## Desktop pointer approach (mouse clients only — not Android)

A touch has no cursor; a mouse does, and **the approach trajectory is the
dominant desktop behavioral signal** — more than the jittered landing. So before
a mouse click/press, walk the cursor to the target instead of teleporting:

1. Query the **real current cursor position** `(fx,fy)` (X11 `QueryPointer`,
   macOS `CGEvent` location, Windows `GetCursorPos`). This is why generation must
   be client-side — only the device knows where the cursor actually is.
2. Jitter the target (σ = 4.0) → `(tx,ty)`.
3. Build a bowed Bézier polyline from `(fx,fy)`→`(tx,ty)` using the **same**
   bow/slide/steps/tremor rules as the swipe trajectory, but sample `t` through
   an **ease-in-out** map `smoothstep(t) = t²(3−2t)` so velocity ramps up and
   down (a hand accelerates off the start and decelerates into the target).
4. Total approach time from **Fitts's law**:
   `MT = a + b·log2(distance/W + 1)`, with `a = 50 ms`, `b = 120 ms/bit`,
   `W = 24 px` (assumed target width), then apply the duration-jitter rule and
   clamp to `[40, 700] ms`. Distribute across steps (`≈ MT/steps` each) with a
   per-step `×(1 + gaussian()*0.25)` jitter, min 1 ms.
5. Press at the landed `(tx,ty)`, hold `logNormalMs(75,…)`, release. Precede the
   whole sequence with one pre-action dwell.

Drag on desktop = approach the grab point `(x1,y1)`, press, then run the swipe
trajectory (no ease — matches touch swipe constant-speed sampling) to `(x2,y2)`
over a jittered `duration_ms`, then release.

## Explicitly out of scope

- **Per-keystroke typing rhythm** — text is committed atomically (Android
  `ACTION_SET_TEXT`; desktop types fast). No per-key event to jitter without a
  custom IME.
- **Composite verb** stays mechanical: the agent built the exact step sequence,
  so humanize does **not** apply to `composite` (`humanize=false` at those call
  sites).
- **Browser** has no OS cursor: "humanize" there means synthesizing a
  `mousemove` event stream along the Bézier before the synthetic click. Weakest
  fidelity; see the browser client notes.
