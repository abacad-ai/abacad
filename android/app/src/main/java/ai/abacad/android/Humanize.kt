package ai.abacad.android

import android.graphics.Path
import java.util.Random
import kotlin.math.exp
import kotlin.math.hypot
import kotlin.math.max
import kotlin.math.min

/**
 * Turns geometrically-perfect agent commands into inputs that look like a human thumb.
 *
 * Behavioural bot-detection (as opposed to environment/root fingerprinting) is a statistical
 * classifier: it asks whether the *distribution* of your touches matches a person's. The raw
 * commands fail that test in four obvious ways — every tap lands on the exact same pixel, every
 * tap is held for exactly 60 ms, every swipe is a straight line at constant speed, and there is
 * zero pause between actions. Each of those is a distribution with variance ≈ 0, which no finger
 * produces. We fix them by sampling from the distributions a real thumb produces:
 *
 *   - positional noise  → Gaussian around the target (fingers never repeat a pixel)
 *   - press duration     → log-normal hold times (clustered low, occasional long press)
 *   - swipe trajectory   → a bowed Bézier polyline with per-point tremor, not a ruler line
 *   - pre-action dwell    → log-normal think-time before each touch
 *
 * The important subtlety: uniform `rand(a, b)` is *itself* a fingerprint — a flat distribution is
 * as unhuman as a constant one. Human timings are log-normal (a floor, a common case, a long
 * tail), so we sample log-normal, not uniform, for every duration.
 *
 * What this does NOT cover: per-keystroke typing rhythm. Text is committed atomically through the
 * accessibility ACTION_SET_TEXT (see AbacadAccessibilityService.inputText); there is no per-key
 * event to jitter without shipping a custom IME. And true velocity *easing* within one stroke
 * isn't reachable either — the platform samples a gesture at arc-length-uniform-in-time, so a
 * single stroke has ~constant speed regardless of geometry. The Bézier bow still breaks the
 * straight-line-at-constant-velocity signal, which is the one detectors actually key on.
 */
object Humanize {

    // Default constructor seeds from the wall clock + an internal counter, so the stream differs
    // across process starts. (Kotlin/JVM has no Math.random ban — that restriction is workflow-JS
    // only — but Random gives us nextGaussian() directly, which we want.)
    private val rng = Random()

    /** Sample N(0, 1). */
    private fun gaussian(): Double = rng.nextGaussian()

    /**
     * A positive log-normal duration in ms with the given [median], shaped by [sigma] (spread of
     * the underlying normal). Clamped to [lo, hi]. Log-normal — not uniform — because real
     * human inter-action and hold times pile up near a floor with a long right tail.
     */
    private fun logNormalMs(median: Double, sigma: Double, lo: Long, hi: Long): Long {
        val v = median * exp(gaussian() * sigma)
        return v.toLong().coerceIn(lo, hi)
    }

    /** Nudge a coordinate off the exact pixel: target + N(0, sigma). */
    fun jitter(coord: Int, sigma: Double = 4.0): Float =
        (coord + gaussian() * sigma).toFloat()

    /** Realistic finger-down hold for a plain tap (~75 ms typical, quick-but-not-instant). */
    fun tapHoldMs(): Long = logNormalMs(median = 75.0, sigma = 0.35, lo = 45L, hi = 140L)

    /** Jitter a requested hold/swipe [duration] by ±~[frac] so repeats aren't identical. */
    fun jitterDuration(duration: Long, frac: Double = 0.12): Long {
        val scaled = duration * (1.0 + gaussian() * frac)
        return scaled.toLong().coerceIn(max(1L, (duration * 0.5).toLong()), (duration * 1.6).toLong())
    }

    /** Log-normal "think time" to wait before dispatching a touch (~70 ms typical, fat tail). */
    fun preActionDwellMs(): Long = logNormalMs(median = 70.0, sigma = 0.55, lo = 25L, hi = 350L)

    /** A single jittered point, as a Path, for tap / long_press. */
    fun pointPath(x: Int, y: Int): Path =
        Path().apply { moveTo(jitter(x), jitter(y)) }

    /**
     * A human-looking swipe from (x1,y1) to (x2,y2): jittered endpoints, a quadratic Bézier that
     * bows to one side (real thumbs arc; they don't draw straight lines), plus small per-sample
     * tremor. Rendered as a dense polyline so the platform's gesture sampler follows the curve.
     */
    fun swipePath(x1: Int, y1: Int, x2: Int, y2: Int): Path {
        val sx = jitter(x1); val sy = jitter(y1)
        val ex = jitter(x2); val ey = jitter(y2)

        val dx = (ex - sx).toDouble(); val dy = (ey - sy).toDouble()
        val len = max(1.0, hypot(dx, dy))
        // Unit perpendicular to the travel direction.
        val px = -dy / len; val py = dx / len

        // Control point at the midpoint, bowed sideways by a random fraction of the length
        // (sign random, magnitude ~8%), plus a little slack along the travel axis.
        val bow = gaussian() * (len * 0.08)
        val slide = gaussian() * (len * 0.05)
        val mx = (sx + ex) / 2.0 + px * bow + (dx / len) * slide
        val my = (sy + ey) / 2.0 + py * bow + (dy / len) * slide

        val steps = min(28, max(12, (len / 25).toInt()))
        val path = Path().apply { moveTo(sx, sy) }
        // Quadratic Bézier B(t) = (1-t)^2 P0 + 2(1-t)t C + t^2 P1, with tiny tremor per sample.
        val tremor = min(2.5, len * 0.01)
        for (i in 1..steps) {
            val t = i.toDouble() / steps
            val u = 1 - t
            var bxp = u * u * sx + 2 * u * t * mx + t * t * ex
            var byp = u * u * sy + 2 * u * t * my + t * t * ey
            if (i < steps) { // leave the final point exactly on the (jittered) target
                bxp += gaussian() * tremor
                byp += gaussian() * tremor
            }
            path.lineTo(bxp.toFloat(), byp.toFloat())
        }
        return path
    }
}
