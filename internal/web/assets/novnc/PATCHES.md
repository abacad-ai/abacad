# Local patches to noVNC 1.6.0

abacad vendors noVNC unchanged except for the following, kept here so a future
upgrade knows what to re-apply.

## `core/rfb.js` — trackpad wheel handling

- `WHEEL_STEP` reduced from 50 to 20 pixels.
- Wheel accumulation now uses a `while` loop and subtracts `WHEEL_STEP` instead
  of a single `if` followed by resetting the accumulator to zero.

Why: the upstream logic collapses a fast trackpad flick into one wheel notch and
throws away the remainder on every step, which feels like jumpy scrolling. The
loop preserves the physical distance and emits every step, which is affordable
on a LAN.

If you upgrade noVNC, re-apply this patch or tune `WHEEL_STEP` to taste.
