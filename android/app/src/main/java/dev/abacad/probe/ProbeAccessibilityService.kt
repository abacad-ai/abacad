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
import android.graphics.PixelFormat
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
import android.view.Gravity
import android.view.View
import android.view.WindowManager
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

        /** How long the keep-awake overlay lingers after the last command before the screen may sleep. */
        const val SESSION_KEEPALIVE_MS = 180_000L

        /** A screenshot requested within this window of the last capture is served from cache, so the
         *  dashboard's poll and the agent's captures coalesce instead of each paying a fresh (main-
         *  thread) encode. Any drive command invalidates it — see [invalidateShotCache]. */
        const val SCREENSHOT_CACHE_MS = 1000L
    }

    private val handler = Handler(Looper.getMainLooper())
    private var device: DeviceClient? = null
    private var wakeLock: PowerManager.WakeLock? = null

    /** A 1px, invisible, non-interactive overlay carrying FLAG_KEEP_SCREEN_ON; see [keepScreenAwake]. */
    private var keepAwakeView: View? = null
    private val dropKeepAwake = Runnable { releaseScreenAwake() }

    // Short-lived screenshot cache + single-flight (main-thread only, so no locks). At most one
    // takeScreenshot runs at a time; near-simultaneous requests (dashboard poll + agent) queue as
    // waiters and share its result, and a repeat within SCREENSHOT_CACHE_MS reuses the last frame.
    // shotGen is bumped by every drive command so a post-action capture never serves a pre-action frame.
    private var shotW = 0
    private var shotH = 0
    private var shotB64: String? = null
    private var shotTree: JSONObject? = null
    private var shotStampNs = 0L
    private var shotGen = 0          // screen-mutation generation; ++ on every drive command
    private var shotCacheGen = -1    // gen the cached frame belongs to
    private var shotCaptureGen = 0   // gen the in-flight capture was started at
    private var shotCapturing = false
    private data class ShotWaiter(val includeTree: Boolean, val done: (CmdResult) -> Unit)
    private val shotWaiters = ArrayList<ShotWaiter>()

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
        handler.removeCallbacks(dropKeepAwake)
        releaseScreenAwake()
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
            ProbeStatus.setState(ProbeStatus.State.DISCONNECTED, "no server URL set — open the app to connect")
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
                    // Hold the display awake for the session so we don't churn the waker (and
                    // steal the foreground) on every command while the agent thinks between them.
                    keepScreenAwake()
                    // Every command other than a read is about to change the screen, so drop the
                    // screenshot cache: the next capture must be fresh, never a pre-action frame.
                    if (method != "screenshot") invalidateShotCache()
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

    /**
     * Serve a screenshot, coalescing near-simultaneous requests. A cache hit (a capture within
     * [SCREENSHOT_CACHE_MS], same screen generation, and carrying a tree if this caller wants one)
     * returns immediately. Otherwise the caller is queued and a single capture is kicked ([startShotCapture])
     * whose result fans out to everyone waiting — so the dashboard's 2s poll and the agent's captures
     * never pile independent PNG encodes onto the main thread or collide on the platform's ~333ms limit.
     */
    private fun captureScreenshot(includeTree: Boolean, done: (CmdResult) -> Unit) {
        val fresh = shotB64 != null && shotCacheGen == shotGen &&
            (System.nanoTime() - shotStampNs) < SCREENSHOT_CACHE_MS * 1_000_000L
        if (fresh && (!includeTree || shotTree != null)) {
            done(CmdResult.Ok(shotResponse(includeTree)))
            return
        }
        shotWaiters.add(ShotWaiter(includeTree, done))
        if (!shotCapturing) startShotCapture()
    }

    /** Kick one fresh capture; its outcome is delivered to every queued [shotWaiters]. */
    private fun startShotCapture() {
        shotCapturing = true
        shotCaptureGen = shotGen
        takeScreenshot(Display.DEFAULT_DISPLAY, mainExecutor, object : TakeScreenshotCallback {
            override fun onSuccess(result: ScreenshotResult) {
                var hb: HardwareBuffer? = null
                try {
                    hb = result.hardwareBuffer
                    val hw = Bitmap.wrapHardwareBuffer(hb, result.colorSpace)
                    if (hw == null) {
                        failShotWaiters("wrapHardwareBuffer returned null")
                        return
                    }
                    val bmp = hw.copy(Bitmap.Config.ARGB_8888, false)
                    val baos = ByteArrayOutputStream()
                    bmp.compress(Bitmap.CompressFormat.PNG, 90, baos)
                    val b64 = Base64.encodeToString(baos.toByteArray(), Base64.NO_WRAP)
                    // A drive command landed while we were capturing: this frame predates it and is
                    // stale for the queued waiters. Recapture, paced past the platform's ~333ms limit.
                    if (shotCaptureGen != shotGen) {
                        handler.postDelayed({ startShotCapture() }, 350L)
                        return
                    }
                    shotW = bmp.width
                    shotH = bmp.height
                    shotB64 = b64
                    shotTree = if (shotWaiters.any { it.includeTree }) buildUiTree() else null
                    shotCacheGen = shotCaptureGen
                    shotStampNs = System.nanoTime()
                    shotCapturing = false
                    val serve = ArrayList(shotWaiters)
                    shotWaiters.clear()
                    for (w in serve) w.done(CmdResult.Ok(shotResponse(w.includeTree)))
                } catch (e: Exception) {
                    failShotWaiters(e.message ?: "screenshot failed")
                } finally {
                    hb?.close()
                }
            }
            override fun onFailure(errorCode: Int) {
                // Report whatever the platform says. Single-flight means we no longer collide with our
                // own captures, so ERROR_TAKE_SCREENSHOT_INTERVAL_TIME_SHORT (the ~333ms rate limit) is
                // now rare; when it does happen the caller can just ask again.
                val reason = when (errorCode) {
                    ERROR_TAKE_SCREENSHOT_INTERVAL_TIME_SHORT -> "rate-limited (interval too short)"
                    ERROR_TAKE_SCREENSHOT_SECURE_WINDOW -> "secure window (FLAG_SECURE)"
                    ERROR_TAKE_SCREENSHOT_NO_ACCESSIBILITY_ACCESS -> "no accessibility access"
                    ERROR_TAKE_SCREENSHOT_INVALID_DISPLAY -> "invalid display"
                    ERROR_TAKE_SCREENSHOT_INTERNAL_ERROR -> "internal error"
                    else -> "error code $errorCode"
                }
                failShotWaiters("screenshot failed: $reason")
            }
        })
    }

    /** Build the wire result from the cached frame, attaching the UI tree only when asked (and present). */
    private fun shotResponse(includeTree: Boolean): JSONObject {
        val out = JSONObject()
            .put("w", shotW)
            .put("h", shotH)
            .put("png_base64", shotB64)
        if (includeTree && shotTree != null) out.put("tree", shotTree)
        return out
    }

    /** Fail every queued waiter with the same message and clear the in-flight flag. */
    private fun failShotWaiters(message: String) {
        shotCapturing = false
        val serve = ArrayList(shotWaiters)
        shotWaiters.clear()
        for (w in serve) w.done(CmdResult.Err(message))
    }

    /** Bump the screen generation so the cached frame no longer matches; called after any drive command. */
    private fun invalidateShotCache() {
        shotGen++
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

        // Screen is off or the keyguard is up: waking adds latency to this command,
        // which is a common reason a call brushes the server's 15s deadline. Surface it.
        Log.i(TAG, "waking screen before command (interactive=${pm.isInteractive} locked=${km.isKeyguardLocked})")
        ProbeStatus.event("waking screen…")
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

    /**
     * Keep the display lit for the duration of an active session, then let it sleep again.
     *
     * The waker ([ensureAwake]) can turn a *dark* screen on, but it's a foreground activity — so
     * firing it on every command (whenever the phone's display timeout beat the gap between the
     * agent's commands) flashed Abacad to the front and stole focus from the app being driven.
     * Instead we add a 1px, invisible, untouchable [WindowManager.LayoutParams.TYPE_ACCESSIBILITY_OVERLAY]
     * window carrying [WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON] — the same well-supported
     * primitive video players use, and one an AccessibilityService may add with no SYSTEM_ALERT_WINDOW
     * grant. While it's up the screen never sleeps, so the (non-secure) keyguard never re-arms and the
     * waker doesn't fire. Called on every command with a sliding [SESSION_KEEPALIVE_MS] timeout, so once
     * the agent stops driving the overlay drops and the device returns to normal screen-off idle.
     *
     * FLAG_KEEP_SCREEN_ON only *keeps* a lit screen on; it can't wake a dark one — so the first
     * command after real idle still wakes via the activity once. After that this suppresses repeats.
     */
    private fun keepScreenAwake() {
        handler.removeCallbacks(dropKeepAwake)
        handler.postDelayed(dropKeepAwake, SESSION_KEEPALIVE_MS)
        if (keepAwakeView != null) return
        val wm = getSystemService(Context.WINDOW_SERVICE) as WindowManager
        val params = WindowManager.LayoutParams(
            1, 1,
            WindowManager.LayoutParams.TYPE_ACCESSIBILITY_OVERLAY,
            WindowManager.LayoutParams.FLAG_NOT_FOCUSABLE or
                WindowManager.LayoutParams.FLAG_NOT_TOUCHABLE or
                WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON,
            PixelFormat.TRANSLUCENT,
        ).apply { gravity = Gravity.TOP or Gravity.START }
        val view = View(this)
        try {
            wm.addView(view, params)
            keepAwakeView = view
            Log.i(TAG, "keep-awake overlay up")
        } catch (e: Exception) {
            Log.w(TAG, "keep-awake overlay failed: ${e.message}")
        }
    }

    private fun releaseScreenAwake() {
        val view = keepAwakeView ?: return
        keepAwakeView = null
        try {
            (getSystemService(Context.WINDOW_SERVICE) as WindowManager).removeView(view)
            Log.i(TAG, "keep-awake overlay down")
        } catch (_: Exception) {}
    }
}
