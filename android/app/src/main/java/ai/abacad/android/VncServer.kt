package ai.abacad.android

import android.app.Activity
import android.content.Intent
import android.graphics.PixelFormat
import android.hardware.display.DisplayManager
import android.hardware.display.VirtualDisplay
import android.media.ImageReader
import android.media.projection.MediaProjection
import android.media.projection.MediaProjectionManager
import android.os.Handler
import android.os.HandlerThread
import android.os.Looper
import android.util.Log
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import okio.ByteString
import okio.ByteString.Companion.toByteString
import org.json.JSONObject
import java.net.InetSocketAddress
import java.net.Socket

/**
 * Android live channel (screen_recording live). The RFB server is IN-APP:
 * LibVNCServer compiled into our own .so and driven over JNI ([RfbNative]) — no
 * droidVNC-NG companion, nothing to install.
 *
 * Flow: MediaProjection captures the screen into an [ImageReader]; each frame is
 * pushed to the native server (bound to 127.0.0.1); the dedicated reverse-connect
 * WebSocket is bridged to that localhost socket. RFB bytes flow
 * browser <-> server <-> this WS <-> LibVNCServer. Same shape as the Linux client's
 * x11vnc pipe; the framebuffer is reachable ONLY on 127.0.0.1, never the LAN.
 *
 * View-only in this pass: the native server ignores viewer pointer/keyboard events
 * (matches live.mode = "view"; interactive control is a documented follow-on).
 *
 * MediaProjection needs a per-session consent, so `start` fires the consent shim
 * ([ScreenRecordConsentActivity]) and the server/capture/bridge only come up once
 * the user grants. UNVERIFIED at runtime.
 */
class VncServer(private val service: AbacadAccessibilityService) {
    private val client = OkHttpClient.Builder().build()
    private val main = Handler(Looper.getMainLooper())

    // frameLock serializes nativePushFrame against nativeStop so a frame callback
    // can never touch a freed native server. Distinct from `this` (the @Synchronized
    // lifecycle lock) to avoid contending with start/stop bookkeeping.
    private val frameLock = Any()

    @Volatile private var running = false
    private var promoted = false // whether THIS session promoted the projection FGS
    private var ws: WebSocket? = null
    private var sock: Socket? = null
    private var handle: Long = 0
    private var imageReader: ImageReader? = null
    private var virtualDisplay: VirtualDisplay? = null
    private var projection: MediaProjection? = null
    private var frameThread: HandlerThread? = null

    companion object {
        // Must match RFB_PORT in rfb_jni.c.
        private const val LOCAL_PORT = 5901
    }

    fun handle(params: JSONObject): JSONObject = when (params.optString("action")) {
        "start" -> {
            start(params.optString("url"))
            JSONObject().put("started", true)
        }
        "stop" -> {
            stop()
            JSONObject().put("stopped", true)
        }
        else -> throw IllegalArgumentException("vnc action must be \"start\" or \"stop\"")
    }

    @Synchronized
    private fun start(url: String) {
        if (url.isEmpty()) throw IllegalArgumentException("vnc start requires url")
        stop()
        running = true
        // MediaProjection consent (reuse the recording shim). Everything else waits
        // for the grant so we never boot the RFB server the user then declines.
        ScreenRecordConsentActivity.onResult = { code, data ->
            main.post { onConsent(url, code, data) }
        }
        val intent = Intent(service, ScreenRecordConsentActivity::class.java)
            .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
        service.startActivity(intent)
    }

    @Synchronized
    private fun onConsent(url: String, resultCode: Int, data: Intent?) {
        if (!running) return // stopped before the user answered
        if (resultCode != Activity.RESULT_OK || data == null) {
            stop()
            return
        }
        try {
            val metrics = service.resources.displayMetrics
            val w = metrics.widthPixels and 1.inv()
            val h = metrics.heightPixels and 1.inv()
            val dpi = metrics.densityDpi

            // Android 14+ needs a mediaProjection foreground service before
            // getMediaProjection; reuse the recorder's promotion (ref-counted so it
            // coexists with a concurrent file recording).
            service.promoteForegroundForRecording()
            promoted = true
            val mpm = service.getSystemService(MediaProjectionManager::class.java)
            val mp = mpm.getMediaProjection(resultCode, data)
                ?: throw IllegalStateException("null MediaProjection")
            projection = mp

            handle = RfbNative.nativeStart(w, h)
            if (handle == 0L) throw IllegalStateException("in-app RFB server failed to start")

            // Push frames from a dedicated thread so the ~30fps memcpy never rides the
            // main looper.
            val ht = HandlerThread("abacad-vnc-frames").apply { start() }
            frameThread = ht
            val reader = ImageReader.newInstance(w, h, PixelFormat.RGBA_8888, 2)
            reader.setOnImageAvailableListener({ r ->
                val img = try { r.acquireLatestImage() } catch (_: Exception) { null } ?: return@setOnImageAvailableListener
                try {
                    synchronized(frameLock) {
                        if (running && handle != 0L) {
                            val plane = img.planes[0]
                            RfbNative.nativePushFrame(handle, plane.buffer, w, h, plane.rowStride)
                        }
                    }
                } catch (_: Exception) {
                } finally {
                    img.close()
                }
            }, Handler(ht.looper))
            imageReader = reader

            virtualDisplay = mp.createVirtualDisplay(
                "abacad-vnc", w, h, dpi,
                DisplayManager.VIRTUAL_DISPLAY_FLAG_AUTO_MIRROR,
                reader.surface, null, null,
            )

            // Bridge the reverse-connect WS to the localhost RFB server.
            Thread {
                try {
                    waitForPort(LOCAL_PORT, 15_000)
                    pipe(url, LOCAL_PORT)
                } catch (_: Exception) {
                } finally {
                    stop()
                }
            }.apply { isDaemon = true; name = "abacad-vnc"; start() }
        } catch (e: Exception) {
            Log.w(AbacadAccessibilityService.TAG, "vnc start failed: ${e.message}")
            stop()
        }
    }

    @Synchronized
    fun stop() {
        running = false
        try { ws?.cancel() } catch (_: Exception) {}
        try { sock?.close() } catch (_: Exception) {}
        ws = null
        sock = null
        // Stop the frame source BEFORE freeing the native server.
        try { virtualDisplay?.release() } catch (_: Exception) {}
        virtualDisplay = null
        try { imageReader?.close() } catch (_: Exception) {}
        imageReader = null
        // Free the native server under frameLock so no in-flight frame push races it.
        synchronized(frameLock) {
            if (handle != 0L) {
                val h = handle
                handle = 0
                try { RfbNative.nativeStop(h) } catch (_: Exception) {}
            }
        }
        try { frameThread?.quitSafely() } catch (_: Exception) {}
        frameThread = null
        try { projection?.stop() } catch (_: Exception) {}
        projection = null
        // Only release the foreground promotion this session actually took, so a
        // consent-denied stop can't drop a concurrent file recording's FGS type.
        if (promoted) {
            promoted = false
            service.demoteForegroundAfterRecording()
        }
    }

    private fun pipe(url: String, port: Int) {
        val tcp = Socket().apply { connect(InetSocketAddress("127.0.0.1", port), 5_000) }
        sock = tcp
        val out = tcp.getOutputStream()

        // WS -> TCP (viewer input; a no-op server-side in view-only mode, but the RFB
        // handshake still flows this way).
        val socket = client.newWebSocket(
            Request.Builder().url(url).build(),
            object : WebSocketListener() {
                override fun onMessage(webSocket: WebSocket, bytes: ByteString) {
                    try {
                        out.write(bytes.toByteArray())
                        out.flush()
                    } catch (_: Exception) {
                        stop()
                    }
                }

                override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) = stop()
                override fun onClosing(webSocket: WebSocket, code: Int, reason: String) = stop()
            },
        )
        ws = socket

        // TCP -> WS (framebuffer)
        val inp = tcp.getInputStream()
        val buf = ByteArray(32 * 1024)
        while (running) {
            val n = inp.read(buf)
            if (n <= 0) break
            socket.send(buf.toByteString(0, n))
        }
    }

    private fun waitForPort(port: Int, timeoutMs: Long) {
        val deadline = System.currentTimeMillis() + timeoutMs
        while (System.currentTimeMillis() < deadline) {
            if (!running) throw IllegalStateException("vnc stopped before it started listening")
            try {
                Socket().use { it.connect(InetSocketAddress("127.0.0.1", port), 300) }
                return
            } catch (_: Exception) {
            }
            Thread.sleep(200)
        }
        throw IllegalStateException("in-app RFB server did not start listening")
    }
}
