package dev.abacad.probe

import android.app.Activity
import android.app.KeyguardManager
import android.content.Context
import android.os.Build
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.util.Log
import android.view.WindowManager

/**
 * The screen-waker. Launched by [ProbeAccessibilityService] when a `wake` command
 * arrives on a dark device: it turns the display on and shows over the keyguard, then
 * dismisses a NON-SECURE (swipe / none) keyguard. A SECURE keyguard (PIN / pattern /
 * biometric) cannot be dismissed programmatically — the screen turns on but stays locked,
 * which is why hands-off use requires no secure lock.
 *
 * It reports the outcome back to the service via [onResult] and finishes immediately —
 * it renders nothing. The service holds a CPU wakelock across this so the process stays
 * alive from "packet arrives on a sleeping phone" through the launch.
 */
class WakerActivity : Activity() {

    data class Outcome(
        val screenOn: Boolean,
        val keyguardSecure: Boolean,
        val unlocked: Boolean,
        val note: String,
    )

    companion object {
        /** One-shot result sink; set by the service just before launch, cleared on report. */
        @Volatile
        var onResult: ((Outcome) -> Unit)? = null
    }

    private val handler = Handler(Looper.getMainLooper())
    private var reported = false

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        // Power the display on and let this (invisible) window sit over the keyguard.
        if (Build.VERSION.SDK_INT >= 27) {
            setShowWhenLocked(true)
            setTurnScreenOn(true)
        }
        @Suppress("DEPRECATION")
        window.addFlags(
            WindowManager.LayoutParams.FLAG_SHOW_WHEN_LOCKED or
                WindowManager.LayoutParams.FLAG_TURN_SCREEN_ON or
                WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON,
        )

        val km = getSystemService(Context.KEYGUARD_SERVICE) as KeyguardManager
        val secure = km.isKeyguardSecure
        val locked = km.isKeyguardLocked

        when {
            locked && !secure && Build.VERSION.SDK_INT >= 26 -> {
                km.requestDismissKeyguard(this, object : KeyguardManager.KeyguardDismissCallback() {
                    override fun onDismissSucceeded() =
                        report(Outcome(true, false, true, "screen on; swipe keyguard dismissed"))
                    override fun onDismissError() =
                        report(Outcome(true, false, false, "screen on; keyguard dismiss error"))
                    override fun onDismissCancelled() =
                        report(Outcome(true, false, false, "screen on; keyguard dismiss cancelled"))
                })
                // Safety net if the callback never fires on this ROM.
                handler.postDelayed(
                    { report(Outcome(true, false, false, "screen on; dismiss callback timed out")) },
                    2500,
                )
            }
            secure -> report(
                Outcome(true, true, false, "screen on, but keyguard is SECURE — cannot auto-unlock; remove PIN/pattern for hands-off use"),
            )
            locked -> report(Outcome(true, false, false, "screen on; keyguard dismiss needs API 26+"))
            else -> report(Outcome(true, false, true, "screen on; no keyguard"))
        }
    }

    private fun report(o: Outcome) {
        if (reported) return
        reported = true
        Log.i(ProbeAccessibilityService.TAG, "waker: ${o.note}")
        onResult?.invoke(o)
        onResult = null
        // Leave the display on (turn-screen-on latched); we just needed to bring it up.
        handler.postDelayed({ finish() }, 300)
    }
}
