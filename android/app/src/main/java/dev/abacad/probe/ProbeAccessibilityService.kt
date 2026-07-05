package dev.abacad.probe

import android.accessibilityservice.AccessibilityService
import android.accessibilityservice.AccessibilityService.GestureResultCallback
import android.accessibilityservice.AccessibilityService.ScreenshotResult
import android.accessibilityservice.AccessibilityService.TakeScreenshotCallback
import android.accessibilityservice.GestureDescription
import android.app.KeyguardManager
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.graphics.Bitmap
import android.graphics.Path
import android.graphics.Rect
import android.hardware.HardwareBuffer
import android.os.Build
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.os.PowerManager
import android.util.Base64
import android.util.Log
import android.view.Display
import android.view.accessibility.AccessibilityEvent
import android.view.accessibility.AccessibilityNodeInfo
import org.json.JSONArray
import org.json.JSONObject
import java.io.ByteArrayOutputStream

/** Result of executing one agent command. */
sealed class CmdResult {
    data class Ok(val result: JSONObject) : CmdResult()
    data class Err(val message: String) : CmdResult()
}

/**
 * The device agent. From the single accessibility grant it lets an agent drive the
 * phone the way a human would — look (screenshot + UI tree), touch (tap / long_press /
 * swipe), type (input_text), and press the nav keys (back / home / recents) — over a
 * [DeviceClient] WebSocket to the Abacad server. Command-driven: no work happens until
 * the server (on behalf of the agent) sends a command.
 *
 * Power is transparent: if a command arrives on a dark/locked device the screen is woken
 * automatically (see [ensureAwake]) before the command runs. Sleeping is left to the
 * device's own display timeout — the agent never manages it.
 */
class ProbeAccessibilityService : AccessibilityService() {

    companion object {
        const val TAG = "ABACAD"
        const val PREFS = "abacad"
        const val KEY_SERVER_URL = "server_url"
        const val ACTION_RECONNECT = "dev.abacad.probe.RECONNECT"

        // The accessibility screenshot API rejects calls that arrive within its
        // rate-limit window (~1s) with ERROR_TAKE_SCREENSHOT_INTERVAL_TIME_SHORT.
        // We retry a few times, each after slightly more than the window, so the
        // whole thing stays inside the server's 15s per-command deadline.
        const val SCREENSHOT_MAX_RETRIES = 3
        const val SCREENSHOT_RETRY_DELAY_MS = 1100L
    }

    private val handler = Handler(Looper.getMainLooper())
    private var device: DeviceClient? = null
    private var wakeLock: PowerManager.WakeLock? = null

    private val reconnectReceiver = object : BroadcastReceiver() {
        override fun onReceive(context: Context?, intent: Intent?) {
            Log.i(TAG, "RECONNECT")
            connectFromPrefs()
        }
    }

    override fun onServiceConnected() {
        super.onServiceConnected()
        Log.i(TAG, "service LIVE — ${Build.MANUFACTURER} ${Build.MODEL} sdk=${Build.VERSION.SDK_INT}")
        val filter = IntentFilter(ACTION_RECONNECT)
        if (Build.VERSION.SDK_INT >= 33) {
            registerReceiver(reconnectReceiver, filter, Context.RECEIVER_EXPORTED)
        } else {
            registerReceiver(reconnectReceiver, filter)
        }
        connectFromPrefs()
    }

    override fun onAccessibilityEvent(event: AccessibilityEvent?) { /* command-driven; no-op */ }
    override fun onInterrupt() {}

    override fun onUnbind(intent: Intent?): Boolean {
        try { unregisterReceiver(reconnectReceiver) } catch (_: Exception) {}
        wakeLock?.let { if (it.isHeld) it.release() }
        wakeLock = null
        device?.close()
        device = null
        return super.onUnbind(intent)
    }

    private fun connectFromPrefs() {
        val url = getSharedPreferences(PREFS, Context.MODE_PRIVATE)
            .getString(KEY_SERVER_URL, "")?.trim().orEmpty()
        device?.close()
        device = null
        if (url.isEmpty()) {
            Log.w(TAG, "no server URL set — open the app and enter ws://<host>:8848/device")
            return
        }
        device = DeviceClient(url, ::execute).also { it.connect() }
    }

    /**
     * Run [method] on the main thread; deliver the outcome via [done]. Every command is
     * gated on [ensureAwake] first, so the agent can drive a phone that idled its screen
     * off without ever having to think about power.
     */
    fun execute(method: String, params: JSONObject, done: (CmdResult) -> Unit) {
        handler.post {
            ensureAwake(
                onReady = {
                    try {
                        when (method) {
                            "screenshot" -> captureScreenshot(params.optBoolean("include_ui_tree", true), done)
                            "tap" -> tapAt(params.optInt("x", -1), params.optInt("y", -1), done)
                            "long_press" -> longPressAt(
                                params.optInt("x", -1), params.optInt("y", -1),
                                params.optLong("duration_ms", 600L), done,
                            )
                            "swipe" -> swipeAt(
                                params.optInt("x1", -1), params.optInt("y1", -1),
                                params.optInt("x2", -1), params.optInt("y2", -1),
                                params.optLong("duration_ms", 300L), done,
                            )
                            "input_text" -> inputText(params.optString("text", ""), done)
                            "back" -> globalAction(GLOBAL_ACTION_BACK, done)
                            "home" -> globalAction(GLOBAL_ACTION_HOME, done)
                            "recents" -> globalAction(GLOBAL_ACTION_RECENTS, done)
                            else -> done(CmdResult.Err("unknown method: $method"))
                        }
                    } catch (e: Exception) {
                        done(CmdResult.Err(e.message ?: e.toString()))
                    }
                },
                onFail = done,
            )
        }
    }

    // ---- perceive --------------------------------------------------------------

    private fun captureScreenshot(includeTree: Boolean, done: (CmdResult) -> Unit, attempt: Int = 0) {
        takeScreenshot(Display.DEFAULT_DISPLAY, mainExecutor, object : TakeScreenshotCallback {
            override fun onSuccess(result: ScreenshotResult) {
                var hb: HardwareBuffer? = null
                try {
                    hb = result.hardwareBuffer
                    val hw = Bitmap.wrapHardwareBuffer(hb, result.colorSpace)
                    if (hw == null) {
                        done(CmdResult.Err("wrapHardwareBuffer returned null"))
                        return
                    }
                    val bmp = hw.copy(Bitmap.Config.ARGB_8888, false)
                    val baos = ByteArrayOutputStream()
                    bmp.compress(Bitmap.CompressFormat.PNG, 90, baos)
                    val b64 = Base64.encodeToString(baos.toByteArray(), Base64.NO_WRAP)
                    val out = JSONObject()
                        .put("w", bmp.width)
                        .put("h", bmp.height)
                        .put("png_base64", b64)
                    if (includeTree) out.put("tree", buildUiTree())
                    done(CmdResult.Ok(out))
                } catch (e: Exception) {
                    done(CmdResult.Err(e.message ?: "screenshot failed"))
                } finally {
                    hb?.close()
                }
            }
            override fun onFailure(errorCode: Int) {
                // The platform rate-limits accessibility screenshots to one per
                // ACCESSIBILITY_TAKE_SCREENSHOT interval (~1s). A request that lands
                // too soon fails with ERROR_TAKE_SCREENSHOT_INTERVAL_TIME_SHORT rather
                // than a real failure — wait out the interval and retry a few times so
                // rapid callers (e.g. the dashboard's live preview) just get the next
                // frame instead of an error.
                if (errorCode == ERROR_TAKE_SCREENSHOT_INTERVAL_TIME_SHORT && attempt < SCREENSHOT_MAX_RETRIES) {
                    handler.postDelayed(
                        { captureScreenshot(includeTree, done, attempt + 1) },
                        SCREENSHOT_RETRY_DELAY_MS,
                    )
                    return
                }
                done(CmdResult.Err("screenshot error code $errorCode"))
            }
        })
    }

    private fun buildUiTree(): JSONObject {
        val out = JSONObject()
        val nodes = JSONArray()
        val root = rootInActiveWindow
        out.put("pkg", root?.packageName?.toString() ?: "")
        if (root != null) {
            var count = 0
            val queue = ArrayDeque<AccessibilityNodeInfo>()
            queue.add(root)
            while (queue.isNotEmpty() && count < 3000) {
                val n = queue.removeFirst()
                count++
                val b = Rect()
                n.getBoundsInScreen(b)
                nodes.put(
                    JSONObject()
                        .put("cls", n.className?.toString() ?: "")
                        .put("text", n.text?.toString() ?: "")
                        .put("id", n.viewIdResourceName ?: "")
                        .put("clickable", n.isClickable)
                        .put("bounds", JSONArray().put(b.left).put(b.top).put(b.right).put(b.bottom)),
                )
                for (i in 0 until n.childCount) n.getChild(i)?.let { queue.add(it) }
            }
        }
        out.put("nodes", nodes)
        return out
    }

    // ---- touch -----------------------------------------------------------------

    private fun tapAt(x: Int, y: Int, done: (CmdResult) -> Unit) {
        if (x < 0 || y < 0) {
            done(CmdResult.Err("tap requires non-negative x,y"))
            return
        }
        // Jittered pixel + log-normal hold, after a short "think-time" dwell (see Humanize).
        afterDwell { dispatchStroke(Humanize.pointPath(x, y), Humanize.tapHoldMs(), done) }
    }

    private fun longPressAt(x: Int, y: Int, durationMs: Long, done: (CmdResult) -> Unit) {
        if (x < 0 || y < 0) {
            done(CmdResult.Err("long_press requires non-negative x,y"))
            return
        }
        val hold = Humanize.jitterDuration(durationMs).coerceIn(100L, 5000L)
        afterDwell { dispatchStroke(Humanize.pointPath(x, y), hold, done) }
    }

    private fun swipeAt(x1: Int, y1: Int, x2: Int, y2: Int, durationMs: Long, done: (CmdResult) -> Unit) {
        if (x1 < 0 || y1 < 0 || x2 < 0 || y2 < 0) {
            done(CmdResult.Err("swipe requires non-negative coords"))
            return
        }
        // Bowed, tremored trajectory with jittered endpoints and a jittered duration.
        val path = Humanize.swipePath(x1, y1, x2, y2)
        val dur = Humanize.jitterDuration(durationMs).coerceIn(50L, 3000L)
        afterDwell { dispatchStroke(path, dur, done) }
    }

    /** Run [action] after a short log-normal pre-action dwell, so touches aren't back-to-back. */
    private fun afterDwell(action: () -> Unit) {
        handler.postDelayed(action, Humanize.preActionDwellMs())
    }

    /** Dispatch a single-stroke gesture ([path] held for [durationMs]) and report acceptance. */
    private fun dispatchStroke(path: Path, durationMs: Long, done: (CmdResult) -> Unit) {
        val stroke = GestureDescription.StrokeDescription(path, 0L, durationMs)
        val gesture = GestureDescription.Builder().addStroke(stroke).build()
        val accepted = dispatchGesture(gesture, object : GestureResultCallback() {
            override fun onCompleted(g: GestureDescription?) {
                done(CmdResult.Ok(JSONObject().put("dispatched", true)))
            }
            override fun onCancelled(g: GestureDescription?) {
                done(CmdResult.Ok(JSONObject().put("dispatched", false)))
            }
        }, null)
        if (!accepted) done(CmdResult.Err("gesture not dispatched"))
    }

    // ---- type ------------------------------------------------------------------

    private fun inputText(text: String, done: (CmdResult) -> Unit) {
        val focused = rootInActiveWindow?.findFocus(AccessibilityNodeInfo.FOCUS_INPUT)
        if (focused == null) {
            done(CmdResult.Err("no focused input field — tap a text field first"))
            return
        }
        val args = Bundle().apply {
            putCharSequence(AccessibilityNodeInfo.ACTION_ARGUMENT_SET_TEXT_CHARSEQUENCE, text)
        }
        val ok = focused.performAction(AccessibilityNodeInfo.ACTION_SET_TEXT, args)
        done(CmdResult.Ok(JSONObject().put("set", ok)))
    }

    // ---- nav keys --------------------------------------------------------------

    private fun globalAction(action: Int, done: (CmdResult) -> Unit) {
        val ok = performGlobalAction(action)
        done(CmdResult.Ok(JSONObject().put("performed", ok)))
    }

    // ---- power (auto-wake; see docs/power-lockscreen.md) ------------------------

    /**
     * Ensure the screen is on and the keyguard is out of the way before running a command,
     * transparently to the agent. If the device is already interactive and unlocked we
     * proceed immediately; otherwise we hold a short CPU wakelock (so the process survives
     * from packet-arrival through the launch) and delegate the screen-on + non-secure-keyguard
     * dismiss to [WakerActivity]. A SECURE keyguard (PIN/pattern) can't be dismissed — that's
     * reported to the agent as an error, since nothing on the locked screen is drivable.
     */
    private fun ensureAwake(onReady: () -> Unit, onFail: (CmdResult) -> Unit) {
        val pm = getSystemService(Context.POWER_SERVICE) as PowerManager
        val km = getSystemService(Context.KEYGUARD_SERVICE) as KeyguardManager
        if (pm.isInteractive && !km.isKeyguardLocked) {
            onReady()
            return
        }

        wakeLock?.let { if (it.isHeld) it.release() }
        @Suppress("DEPRECATION")
        wakeLock = pm.newWakeLock(PowerManager.PARTIAL_WAKE_LOCK, "abacad:wake").apply {
            setReferenceCounted(false)
            acquire(60_000L)
        }

        var settled = false
        val timeout = Runnable {
            if (settled) return@Runnable
            settled = true
            WakerActivity.onResult = null
            onFail(CmdResult.Err("wake activity did not start in 4s — likely an OEM background-activity-launch restriction; grant this app \"Display over other apps\""))
        }
        handler.postDelayed(timeout, 4000L)

        WakerActivity.onResult = { o ->
            handler.removeCallbacks(timeout)
            if (!settled) {
                settled = true
                if (o.keyguardSecure && !o.unlocked) {
                    onFail(CmdResult.Err("device is locked with a PIN/pattern and can't be unlocked remotely — unlock it once by hand, or remove the secure lock for hands-off use"))
                } else {
                    onReady()
                }
            }
        }

        try {
            startActivity(
                Intent(this, WakerActivity::class.java)
                    .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_NO_ANIMATION),
            )
        } catch (e: Exception) {
            handler.removeCallbacks(timeout)
            WakerActivity.onResult = null
            if (!settled) {
                settled = true
                onFail(CmdResult.Err("could not start waker activity: ${e.message}"))
            }
        }
    }
}
