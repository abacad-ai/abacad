package dev.abacad.probe

import android.accessibilityservice.AccessibilityService
import android.accessibilityservice.AccessibilityService.GestureResultCallback
import android.accessibilityservice.AccessibilityService.ScreenshotResult
import android.accessibilityservice.AccessibilityService.TakeScreenshotCallback
import android.accessibilityservice.GestureDescription
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.graphics.Bitmap
import android.graphics.Path
import android.graphics.Rect
import android.hardware.HardwareBuffer
import android.os.Build
import android.os.Handler
import android.os.Looper
import android.os.SystemClock
import android.util.Log
import android.view.Display
import android.view.accessibility.AccessibilityEvent
import android.view.accessibility.AccessibilityNodeInfo
import java.io.File
import java.io.FileOutputStream

/**
 * Throwaway feasibility probe. From the SINGLE accessibility grant it exercises
 * the three capabilities Abacad depends on and logs the outcome under tag
 * ABACAD_PROBE:
 *
 *   1. TREE  - getRootInActiveWindow() richness (nodes / text / clickable / ids)
 *   2. TAP   - dispatchGesture() a center tap; did it deliver?
 *   3. SHOT  - takeScreenshot(); real pixels, non-black, and CRUCIALLY with no
 *              MediaProjection / per-session consent dialog.
 *
 * Runs on connect, on each window change (debounced past the ~1/sec screenshot
 * rate limit), and on demand via `adb shell am broadcast -a dev.abacad.probe.RUN`.
 */
class ProbeAccessibilityService : AccessibilityService() {

    companion object {
        const val TAG = "ABACAD_PROBE"
        const val ACTION_RUN = "dev.abacad.probe.RUN"
        const val MIN_INTERVAL_MS = 3000L // keep clear of takeScreenshot() rate limit
    }

    private val handler = Handler(Looper.getMainLooper())
    private var lastRun = 0L

    private val runReceiver = object : BroadcastReceiver() {
        override fun onReceive(context: Context?, intent: Intent?) {
            Log.i(TAG, "broadcast RUN received")
            runProbe("broadcast")
        }
    }

    override fun onServiceConnected() {
        super.onServiceConnected()
        Log.i(TAG, "==================================================")
        Log.i(TAG, "onServiceConnected - accessibility service is LIVE")
        Log.i(TAG, "device=${Build.MANUFACTURER} ${Build.MODEL} sdk=${Build.VERSION.SDK_INT}")

        val filter = IntentFilter(ACTION_RUN)
        // Exported so `adb shell am broadcast` can reach it. Fine for a throwaway.
        if (Build.VERSION.SDK_INT >= 33) {
            registerReceiver(runReceiver, filter, Context.RECEIVER_EXPORTED)
        } else {
            registerReceiver(runReceiver, filter)
        }

        handler.postDelayed({ runProbe("onConnected") }, 1500)
    }

    override fun onAccessibilityEvent(event: AccessibilityEvent?) {
        if (event?.eventType == AccessibilityEvent.TYPE_WINDOW_STATE_CHANGED) {
            val now = SystemClock.elapsedRealtime()
            if (now - lastRun >= MIN_INTERVAL_MS) {
                runProbe("windowChange:${event.packageName}")
            }
        }
    }

    override fun onInterrupt() {}

    override fun onUnbind(intent: Intent?): Boolean {
        try {
            unregisterReceiver(runReceiver)
        } catch (_: Exception) {
        }
        return super.onUnbind(intent)
    }

    private fun runProbe(trigger: String) {
        lastRun = SystemClock.elapsedRealtime()
        Log.i(TAG, "----- runProbe (trigger=$trigger) -----")
        probeTree()
        probeTap()
        probeScreenshot()
    }

    // 1. UI TREE ---------------------------------------------------------------
    private fun probeTree() {
        try {
            val root = rootInActiveWindow
            if (root == null) {
                Log.w(TAG, "TREE: rootInActiveWindow == null (no window content available)")
                return
            }
            var count = 0
            var withText = 0
            var clickable = 0
            var withId = 0
            val samples = ArrayList<String>()

            val queue = ArrayDeque<AccessibilityNodeInfo>()
            queue.add(root)
            while (queue.isNotEmpty() && count < 5000) {
                val n = queue.removeFirst()
                count++
                val text = n.text?.toString()
                val id = n.viewIdResourceName
                if (!text.isNullOrBlank()) withText++
                if (n.isClickable) clickable++
                if (!id.isNullOrBlank()) withId++
                if (samples.size < 8 && (!text.isNullOrBlank() || n.isClickable)) {
                    val b = Rect()
                    n.getBoundsInScreen(b)
                    samples.add("<${n.className}> text='${text ?: ""}' id='${id ?: ""}' clickable=${n.isClickable} bounds=$b")
                }
                for (i in 0 until n.childCount) {
                    n.getChild(i)?.let { queue.add(it) }
                }
            }
            Log.i(TAG, "TREE: pkg=${root.packageName} nodes=$count withText=$withText clickable=$clickable withResId=$withId")
            for (s in samples) Log.i(TAG, "TREE_SAMPLE $s")
        } catch (e: Exception) {
            Log.e(TAG, "TREE: exception", e)
        }
    }

    // 2. TAP INJECTION ----------------------------------------------------------
    private fun probeTap() {
        try {
            val metrics = resources.displayMetrics
            val cx = metrics.widthPixels / 2f
            val cy = metrics.heightPixels / 2f
            val path = Path().apply { moveTo(cx, cy) }
            val stroke = GestureDescription.StrokeDescription(path, 0L, 60L)
            val gesture = GestureDescription.Builder().addStroke(stroke).build()
            val dispatched = dispatchGesture(gesture, object : GestureResultCallback() {
                override fun onCompleted(gestureDescription: GestureDescription?) {
                    Log.i(TAG, "TAP: onCompleted (gesture delivered) at ($cx,$cy)")
                }

                override fun onCancelled(gestureDescription: GestureDescription?) {
                    Log.w(TAG, "TAP: onCancelled at ($cx,$cy)")
                }
            }, null)
            Log.i(TAG, "TAP: dispatchGesture returned=$dispatched (center $cx,$cy)")
        } catch (e: Exception) {
            Log.e(TAG, "TAP: exception", e)
        }
    }

    // 3. SCREENSHOT (the crown-jewel test) --------------------------------------
    private fun probeScreenshot() {
        try {
            takeScreenshot(Display.DEFAULT_DISPLAY, mainExecutor, object : TakeScreenshotCallback {
                override fun onSuccess(result: ScreenshotResult) {
                    var hb: HardwareBuffer? = null
                    try {
                        hb = result.hardwareBuffer
                        val hw = Bitmap.wrapHardwareBuffer(hb, result.colorSpace)
                        if (hw == null) {
                            Log.e(TAG, "SHOT: wrapHardwareBuffer returned null")
                            return
                        }
                        val bmp = hw.copy(Bitmap.Config.ARGB_8888, false)
                        val nonBlack = isNonBlack(bmp)
                        Log.i(TAG, "SHOT: SUCCESS ${bmp.width}x${bmp.height} nonBlack=$nonBlack  <-- no MediaProjection prompt")
                        val out = File(getExternalFilesDir(null), "probe_shot.png")
                        FileOutputStream(out).use { bmp.compress(Bitmap.CompressFormat.PNG, 90, it) }
                        Log.i(TAG, "SHOT: saved -> ${out.absolutePath}")
                    } catch (e: Exception) {
                        Log.e(TAG, "SHOT: onSuccess exception", e)
                    } finally {
                        hb?.close()
                    }
                }

                override fun onFailure(errorCode: Int) {
                    Log.e(TAG, "SHOT: FAILURE errorCode=$errorCode (see AccessibilityService.ERROR_TAKE_SCREENSHOT_*)")
                }
            })
        } catch (e: Exception) {
            Log.e(TAG, "SHOT: dispatch exception (missing canTakeScreenshot capability?)", e)
        }
    }

    /** Sample a sparse grid; true if any sampled pixel has a non-zero RGB. */
    private fun isNonBlack(bmp: Bitmap): Boolean {
        val stepX = (bmp.width / 20).coerceAtLeast(1)
        val stepY = (bmp.height / 40).coerceAtLeast(1)
        var x = 0
        while (x < bmp.width) {
            var y = 0
            while (y < bmp.height) {
                if ((bmp.getPixel(x, y) and 0x00FFFFFF) != 0) return true
                y += stepY
            }
            x += stepX
        }
        return false
    }
}
