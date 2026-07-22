package x11

import (
	"math"
	"math/rand"
	"time"
)

// Human-like pointer motion. This is the Go port of the shared spec in
// docs/humanize.md (reference implementation: Android's Humanize.kt). When the
// server marks a device humanize=on, Click/LongPress/Drag/Scroll route through
// these helpers instead of teleporting: a curved cursor approach from the real
// current pointer position, jittered landing, log-normal hold, and human-scaled
// timing — the signals a behavioral bot classifier keys on.
//
// Every duration is log-normal, never uniform: a flat distribution is itself a
// fingerprint. Constants are kept identical to the spec so all clients match.

// gaussian samples N(0,1). Go's global rand is auto-seeded (1.20+), so the
// stream differs across process starts without an explicit seed.
func gaussian() float64 { return rand.NormFloat64() }

// logNormalMs returns a positive log-normal duration in ms, clamped to [lo,hi].
func logNormalMs(median, sigma float64, lo, hi int) int {
	v := int(median * math.Exp(gaussian()*sigma))
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// hpJitter nudges a coordinate off the exact pixel: coord + N(0,sigma).
func hpJitter(coord int, sigma float64) int {
	return int(math.Round(float64(coord) + gaussian()*sigma))
}

// hpTapHoldMs is a realistic button-down hold for a click (~75ms typical).
func hpTapHoldMs() int { return logNormalMs(75, 0.35, 45, 140) }

// hpDwellMs is the log-normal "think time" before an action (~70ms typical).
func hpDwellMs() int { return logNormalMs(70, 0.55, 25, 350) }

// hpJitterDuration perturbs a requested hold/drag duration by ±~frac, clamped to
// [0.5x, 1.6x] so repeats aren't identical.
func hpJitterDuration(duration int, frac float64) int {
	scaled := int(float64(duration) * (1.0 + gaussian()*frac))
	lo := int(float64(duration) * 0.5)
	if lo < 1 {
		lo = 1
	}
	hi := int(float64(duration) * 1.6)
	if scaled < lo {
		return lo
	}
	if scaled > hi {
		return hi
	}
	return scaled
}

type hpoint struct{ x, y int }

// bowedPolyline samples a quadratic Bézier from (sx,sy) to (ex,ey) that bows to
// one random side with small per-sample tremor — a human arc, not a ruler line.
// Returns the intermediate+final points (excluding the start). When ease is set,
// the parameter is run through smoothstep so velocity ramps up and down (used
// for a mouse approach; touch swipes sample at constant speed, ease=false).
func bowedPolyline(sx, sy, ex, ey float64, ease bool) []hpoint {
	dx, dy := ex-sx, ey-sy
	length := math.Max(1, math.Hypot(dx, dy))
	px, py := -dy/length, dx/length // unit perpendicular

	bow := gaussian() * (length * 0.08)
	slide := gaussian() * (length * 0.05)
	mx := (sx+ex)/2 + px*bow + (dx/length)*slide
	my := (sy+ey)/2 + py*bow + (dy/length)*slide

	steps := int(length / 25)
	if steps < 12 {
		steps = 12
	}
	if steps > 28 {
		steps = 28
	}
	tremor := math.Min(2.5, length*0.01)

	out := make([]hpoint, 0, steps)
	for i := 1; i <= steps; i++ {
		t := float64(i) / float64(steps)
		if ease {
			t = t * t * (3 - 2*t) // smoothstep
		}
		u := 1 - t
		bx := u*u*sx + 2*u*t*mx + t*t*ex
		by := u*u*sy + 2*u*t*my + t*t*ey
		if i < steps { // leave the final point exactly on the (jittered) target
			bx += gaussian() * tremor
			by += gaussian() * tremor
		}
		out = append(out, hpoint{int(math.Round(bx)), int(math.Round(by))})
	}
	return out
}

// fittsMs estimates the movement time for a cursor travel of dist px, per
// Fitts's law with an assumed target width, then jitters and clamps it.
func fittsMs(dist float64) int {
	const a, b, w = 50.0, 120.0, 24.0
	mt := a + b*math.Log2(dist/w+1)
	return clampInt(hpJitterDuration(int(mt), 0.12), 40, 700)
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func sleepMs(ms int) {
	if ms > 0 {
		time.Sleep(time.Duration(ms) * time.Millisecond)
	}
}

// humanMoveTo walks the cursor from its real current position to a jittered
// target along an eased bowed path, and returns the landed point so the caller
// presses exactly where the cursor ended up. Falls back to a direct move if the
// current position can't be read.
func (c *Conn) humanMoveTo(toX, toY int) (int, int) {
	tx, ty := hpJitter(toX, 4.0), hpJitter(toY, 4.0)
	fx, fy, err := c.PointerPos()
	if err != nil {
		c.fake(evMotion, 0, tx, ty)
		c.sync()
		return tx, ty
	}
	pts := bowedPolyline(float64(fx), float64(fy), float64(tx), float64(ty), true)
	per := fittsMs(math.Hypot(float64(tx-fx), float64(ty-fy))) / len(pts)
	for _, p := range pts {
		c.fake(evMotion, 0, p.x, p.y)
		c.sync()
		sleepMs(int(math.Max(1, float64(per)*(1+gaussian()*0.25))))
	}
	return tx, ty
}
