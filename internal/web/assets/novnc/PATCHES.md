# Local patches to noVNC 1.6.0

abacad vendors noVNC unchanged except for the following, kept here so a future
upgrade knows what to re-apply.

## `core/rfb.js` — trackpad wheel handling

- `WHEEL_STEP` reduced from 50 to 20 pixels.
- A single wheel event of 100 pixels or more, or any line/page-mode event, is
treated as one mouse notch:
  emit one step and reset the accumulator. On macOS a mouse wheel reports about
  100–120 pixels per notch, so dividing that by `WHEEL_STEP` sent five or six
  steps for one physical click.
- Smaller, continuous events (trackpad) use a `while` loop that subtracts
  `WHEEL_STEP` instead of resetting to zero, and keep the remainder clamped to
  ±2 steps.

Why: the upstream reset collapses a fast trackpad flick into one wheel notch
and throws away the remainder on every step, which feels like jumpy scrolling.
The loop preserves the physical distance for trackpads. The discrete branch
keeps a mouse wheel at one notch per click, otherwise the same loop makes it
five times too fast.

If you upgrade noVNC, re-apply this patch or tune `WHEEL_STEP` /
`WHEEL_DISCRETE` to taste.
