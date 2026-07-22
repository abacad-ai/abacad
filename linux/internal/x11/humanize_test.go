package x11

import (
	"math"
	"testing"
)

// bowedPolyline must produce a sane, on-target trajectory: a bounded number of
// samples, ending exactly on the (already-jittered) target with no final tremor.
func TestBowedPolylineBasics(t *testing.T) {
	pts := bowedPolyline(0, 0, 500, 0, false)
	if len(pts) < 12 || len(pts) > 28 {
		t.Fatalf("step count = %d, want 12..28", len(pts))
	}
	last := pts[len(pts)-1]
	if last.x != 500 || last.y != 0 {
		t.Errorf("final point = (%d,%d), want exactly (500,0)", last.x, last.y)
	}
	for i, p := range pts {
		if math.IsNaN(float64(p.x)) || math.IsNaN(float64(p.y)) {
			t.Fatalf("point %d is NaN", i)
		}
	}
}

// The path must actually bow/tremor off the straight line — that's the whole
// point. A single call's bow can be near zero by chance, so we assert over many
// samples that a real sideways deviation shows up (bow σ ≈ 8% of length).
func TestBowedPolylineBows(t *testing.T) {
	const trials = 100
	maxDev := 0.0
	for i := 0; i < trials; i++ {
		for _, p := range bowedPolyline(0, 0, 500, 0, false) {
			if d := math.Abs(float64(p.y)); d > maxDev { // straight line has y==0
				maxDev = d
			}
		}
	}
	if maxDev <= 2 {
		t.Errorf("max lateral deviation over %d trials = %.2fpx; path is essentially straight", trials, maxDev)
	}
}

// fittsMs must stay within its clamp for any travel distance.
func TestFittsMsRange(t *testing.T) {
	for _, dist := range []float64{0, 10, 100, 500, 2000} {
		for i := 0; i < 50; i++ {
			ms := fittsMs(dist)
			if ms < 40 || ms > 700 {
				t.Fatalf("fittsMs(%.0f) = %d, out of [40,700]", dist, ms)
			}
		}
	}
}

// logNormalMs must respect its clamp and never return a non-positive duration.
func TestLogNormalMsClamp(t *testing.T) {
	for i := 0; i < 200; i++ {
		if v := hpTapHoldMs(); v < 45 || v > 140 {
			t.Fatalf("tap hold = %d, out of [45,140]", v)
		}
		if v := hpDwellMs(); v < 25 || v > 350 {
			t.Fatalf("dwell = %d, out of [25,350]", v)
		}
	}
}

// hpJitterDuration stays within [0.5x, 1.6x] of the requested duration.
func TestJitterDurationClamp(t *testing.T) {
	const base = 300
	for i := 0; i < 200; i++ {
		v := hpJitterDuration(base, 0.12)
		if v < base/2 || v > (base*16)/10 {
			t.Fatalf("jitterDuration(%d) = %d, out of clamp", base, v)
		}
	}
}
