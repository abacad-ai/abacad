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
 * The device agent. From the single accessibility grant it exposes the three
 * primitives the agent drives — ui_tree / tap / screenshot — over a [DeviceClient]
 * WebSocket to the Abacad server. Command-driven: no work happens until the
 * server (on behalf of the agent) sends a command.
 */
class ProbeAccessibilityService : AccessibilityService() {

    companion object {
        const val TAG = "ABACAD"
        const val PREFS = "abacad"
        const val KEY_SERVER_URL = "server_url"
        const val ACTION_RECONNECT = "dev.abacad.probe.RECONNECT"
    }

    private val handler = Handler(Looper.getMainLooper())
    private var device: DeviceClient? = null

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

    /** Run [method] on the main thread; deliver the outcome via [done]. */
    fun execute(method: String, params: JSONObject, done: (CmdResult) -> Unit) {
        handler.post {
            try {
                when (method) {
                    "ui_tree" -> done(CmdResult.Ok(buildUiTree()))
                    "screenshot" -> captureScreenshot(done)
                    "tap" -> tapAt(params.optInt("x", -1), params.optInt("y", -1), done)
                    "swipe" -> swipeAt(
                        params.optInt("x1", -1), params.optInt("y1", -1),
                        params.optInt("x2", -1), params.optInt("y2", -1),
                        params.optLong("duration_ms", 300L), done,
                    )
                    else -> done(CmdResult.Err("unknown method: $method"))
                }
            } catch (e: Exception) {
                done(CmdResult.Err(e.message ?: e.toString()))
            }
        }
    }

    // ---- capabilities (the primitives verified by the probe) -------------------

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

    private fun tapAt(x: Int, y: Int, done: (CmdResult) -> Unit) {
        if (x < 0 || y < 0) {
            done(CmdResult.Err("tap requires non-negative x,y"))
            return
        }
        val path = Path().apply { moveTo(x.toFloat(), y.toFloat()) }
        val stroke = GestureDescription.StrokeDescription(path, 0L, 60L)
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

    private fun swipeAt(x1: Int, y1: Int, x2: Int, y2: Int, durationMs: Long, done: (CmdResult) -> Unit) {
        if (x1 < 0 || y1 < 0 || x2 < 0 || y2 < 0) {
            done(CmdResult.Err("swipe requires non-negative coords"))
            return
        }
        val path = Path().apply {
            moveTo(x1.toFloat(), y1.toFloat())
            lineTo(x2.toFloat(), y2.toFloat())
        }
        val dur = durationMs.coerceIn(50L, 3000L)
        val stroke = GestureDescription.StrokeDescription(path, 0L, dur)
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

    private fun captureScreenshot(done: (CmdResult) -> Unit) {
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
                    done(CmdResult.Ok(JSONObject().put("w", bmp.width).put("h", bmp.height).put("png_base64", b64)))
                } catch (e: Exception) {
                    done(CmdResult.Err(e.message ?: "screenshot failed"))
                } finally {
                    hb?.close()
                }
            }
            override fun onFailure(errorCode: Int) {
                done(CmdResult.Err("screenshot error code $errorCode"))
            }
        })
    }
}
