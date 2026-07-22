package ai.abacad.android

import android.content.ComponentName
import android.content.Intent
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
 * Android live channel (screen_recording live): the app never implements RFB. It
 * launches the **droidVNC-NG companion app** — a real, MediaProjection-based RFB
 * server (GPL, shipped as a SEPARATE app so our APK stays MIT) — bound to a
 * localhost port, then pipes the dedicated reverse-connect WebSocket to it. Same
 * shape as the Linux client's x11vnc pipe. droidVNC-NG's own reverse-connect is
 * raw-TCP (not our WSS ingress), so we bridge WS <-> localhost TCP ourselves.
 *
 * Requires droidVNC-NG installed; starting it triggers droidVNC-NG's own
 * MediaProjection consent dialog. UNVERIFIED at runtime.
 *
 * SECURITY CAVEAT: droidVNC-NG binds all interfaces; during a session the screen is
 * reachable on the LAN on LOCAL_PORT. A per-session password (below, currently
 * empty) or a localhost bind should be added before this is production-grade.
 */
class VncPipe(private val service: AbacadAccessibilityService) {
    private val client = OkHttpClient.Builder().build()

    @Volatile private var running = false
    private var ws: WebSocket? = null
    private var sock: Socket? = null

    companion object {
        private const val PKG = "net.christianbeier.droidvnc_ng"
        private const val SVC = "net.christianbeier.droidvnc_ng.MainService"
        private const val ACTION_START = "net.christianbeier.droidvnc_ng.ACTION_START"
        private const val ACTION_STOP = "net.christianbeier.droidvnc_ng.ACTION_STOP"
        private const val EXTRA_PORT = "net.christianbeier.droidvnc_ng.EXTRA_PORT"
        private const val EXTRA_PASSWORD = "net.christianbeier.droidvnc_ng.EXTRA_PASSWORD"
        private const val EXTRA_ACCESS_KEY = "net.christianbeier.droidvnc_ng.EXTRA_ACCESS_KEY"
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
        if (!companionInstalled()) {
            throw IllegalStateException("live view needs the droidVNC-NG companion app installed")
        }
        running = true

        // Launch droidVNC-NG's server on a localhost port (it pops its own
        // MediaProjection consent).
        val intent = Intent(ACTION_START).apply {
            component = ComponentName(PKG, SVC)
            putExtra(EXTRA_PORT, LOCAL_PORT)
            putExtra(EXTRA_PASSWORD, "")
            putExtra(EXTRA_ACCESS_KEY, "abacad")
        }
        service.startForegroundService(intent)

        Thread {
            try {
                waitForPort(LOCAL_PORT, 15_000)
                pipe(url, LOCAL_PORT)
            } catch (_: Exception) {
            } finally {
                stop()
            }
        }.apply { isDaemon = true; name = "abacad-vnc"; start() }
    }

    @Synchronized
    fun stop() {
        running = false
        try { ws?.cancel() } catch (_: Exception) {}
        try { sock?.close() } catch (_: Exception) {}
        ws = null
        sock = null
        if (companionInstalled()) {
            try {
                service.startService(Intent(ACTION_STOP).apply { component = ComponentName(PKG, SVC) })
            } catch (_: Exception) {}
        }
    }

    private fun pipe(url: String, port: Int) {
        val tcp = Socket().apply { connect(InetSocketAddress("127.0.0.1", port), 5_000) }
        sock = tcp
        val out = tcp.getOutputStream()

        // WS -> TCP (viewer input)
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

    private fun companionInstalled(): Boolean = try {
        service.packageManager.getPackageInfo(PKG, 0)
        true
    } catch (_: Exception) {
        false
    }

    private fun waitForPort(port: Int, timeoutMs: Long) {
        val deadline = System.currentTimeMillis() + timeoutMs
        while (System.currentTimeMillis() < deadline) {
            try {
                Socket().use { it.connect(InetSocketAddress("127.0.0.1", port), 300) }
                return
            } catch (_: Exception) {
            }
            Thread.sleep(200)
        }
        throw IllegalStateException("droidVNC-NG did not start listening")
    }
}
