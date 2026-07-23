package ai.abacad.android

import android.net.Uri
import android.os.Handler
import android.os.Looper
import android.util.Log
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import org.json.JSONObject
import java.util.concurrent.TimeUnit

/**
 * Outbound WebSocket to the abacad server. Dials out (NAT-friendly), reconnects
 * with backoff, and answers each incoming command by running it through
 * [executor] (the accessibility service) and sending the reply back.
 *
 * Wire: recv {id, method, params}  ->  send {id, ok, result} | {id, ok:false, error}
 *
 * Security:
 *  - The device token is sent in the `Authorization: Bearer` header, never in the
 *    connect URL — so it can't leak through logs, the status panel, or a proxy's
 *    access log. A `?token=` in a stored URL is migrated to the header here.
 *  - Cleartext `ws://` is refused for any non-loopback host: this device carries a
 *    control channel and screen contents, so a plaintext hop is a full MITM /
 *    device takeover. Real deployments must use `wss://`.
 */
class DeviceClient(
    rawUrl: String,
    private val executor: (method: String, params: JSONObject, done: (CmdResult) -> Unit) -> Unit,
    // Returns the device's current power state ("active" | "asleep") so we can
    // report it to the server on connect. The service supplies it (it knows the
    // screen state); null means "always report active".
    private val currentActivity: (() -> String)? = null,
) {
    private val tag = AbacadAccessibilityService.TAG
    private val client = OkHttpClient.Builder()
        .pingInterval(20, TimeUnit.SECONDS)
        .readTimeout(0, TimeUnit.MILLISECONDS) // long-lived socket, idle between commands
        .build()
    private val handler = Handler(Looper.getMainLooper())
    private var ws: WebSocket? = null
    private var closed = false
    private var connected = false
    private val reconnectRunnable = Runnable { open() }
    private var backoffMs = 1000L

    private val uri: Uri = Uri.parse(rawUrl)
    private val scheme: String = (uri.scheme ?: "").lowercase()
    private val host: String = uri.host ?: ""

    // The token is carried as a header; strip it from the URL we actually dial.
    private val token: String? = uri.getQueryParameter("token")
    private val connectUrl: String = run {
        val b = uri.buildUpon().clearQuery()
        for (name in uri.queryParameterNames) {
            if (name == "token") continue
            for (v in uri.getQueryParameters(name)) b.appendQueryParameter(name, v)
        }
        // Advertise our version so the relay can show it in the dashboard /
        // list_devices. Unlike the token it rides in the URL — the server reads
        // ?version= off the dial. VERSION_NAME comes from the repo-root VERSION
        // file (see android/app/build.gradle.kts).
        if (uri.getQueryParameter("version") == null) {
            b.appendQueryParameter("version", BuildConfig.VERSION_NAME)
        }
        b.build().toString()
    }

    // Safe to log/display: scheme + host + path only, never the token/query.
    private val safeUrl: String =
        "$scheme://$host${if (uri.port > 0) ":${uri.port}" else ""}${uri.path ?: ""}"

    fun connect() {
        closed = false
        open()
    }

    /**
     * Bring the socket back *now* if it isn't up — called on screen-on / network-regained so a
     * socket that died during idle doesn't wait out the backoff. Resets the backoff and, if we're
     * not currently connected, cancels any pending delayed reconnect and dials immediately. A
     * no-op while the socket is healthy.
     */
    fun forceReconnect() {
        if (closed || connected) return
        backoffMs = 1000L
        handler.removeCallbacks(reconnectRunnable)
        open()
    }

    fun close() {
        closed = true
        connected = false
        handler.removeCallbacks(reconnectRunnable)
        try { ws?.close(1000, "bye") } catch (_: Exception) {}
        ws = null
        AbacadStatus.setState(AbacadStatus.State.DISCONNECTED, "disconnected")
    }

    /**
     * Report a power-state change ("active" | "asleep") to the server. Best-effort:
     * dropped if the socket isn't up (on reconnect, onOpen re-reports the current
     * state anyway). The server treats it as a display signal — an asleep device
     * stays reachable, and any command still auto-wakes it.
     */
    fun sendPresence(state: String) {
        val sock = ws ?: return
        if (!connected) return
        try { sock.send(presenceFrame(state)) } catch (_: Exception) {}
    }

    private fun presenceFrame(state: String): String =
        JSONObject().put("type", "presence").put("state", state).toString()

    // Reflect live-view / recording sessions in the status panel so the person at
    // the device sees when their screen is being watched or recorded. Best-effort,
    // inferred from the command verbs the server relays.
    private fun updateAwareness(method: String, params: JSONObject) {
        when (method) {
            "vnc" -> when (params.optString("action")) {
                "start" -> AbacadStatus.setWatched(true)
                "stop" -> AbacadStatus.setWatched(false)
            }
            "screen_recording" -> when (params.optString("action")) {
                "start" -> AbacadStatus.setRecording(true)
                "stop" -> AbacadStatus.setRecording(false)
            }
        }
    }

    private fun isLoopback(h: String): Boolean =
        h == "127.0.0.1" || h == "::1" || h == "localhost" || h == "10.0.2.2" // 10.0.2.2 = emulator host

    private fun open() {
        if (closed) return
        // Refuse a plaintext control channel to anything but loopback/dev.
        if (scheme == "ws" && !isLoopback(host)) {
            Log.e(tag, "refusing plaintext ws:// to $host — use wss://")
            AbacadStatus.setState(AbacadStatus.State.DISCONNECTED, "refused plaintext ws:// — use wss://")
            return
        }
        Log.i(tag, "ws connecting: $safeUrl")
        AbacadStatus.setState(AbacadStatus.State.CONNECTING, "connecting to $safeUrl")
        val req = Request.Builder().url(connectUrl)
        token?.let { req.header("Authorization", "Bearer $it") }
        ws = client.newWebSocket(req.build(), listener)
    }

    private fun scheduleReconnect() {
        if (closed) return
        val delay = backoffMs
        backoffMs = (backoffMs * 2).coerceAtMost(15000L)
        Log.i(tag, "ws reconnect in ${delay}ms")
        AbacadStatus.setState(AbacadStatus.State.RECONNECTING, "reconnecting in ${delay}ms")
        handler.removeCallbacks(reconnectRunnable)
        handler.postDelayed(reconnectRunnable, delay)
    }

    private val listener = object : WebSocketListener() {
        override fun onOpen(webSocket: WebSocket, response: Response) {
            if (webSocket !== ws) return // stale socket (superseded by a forceReconnect)
            Log.i(tag, "ws open -> $safeUrl")
            connected = true
            AbacadStatus.setState(AbacadStatus.State.CONNECTED, "connected to $safeUrl")
            backoffMs = 1000L
            // Tell the server our current power state up front, so a device that
            // connects while the screen is already off shows as asleep, not active.
            currentActivity?.let { webSocket.send(presenceFrame(it())) }
        }

        override fun onMessage(webSocket: WebSocket, text: String) {
            val cmd = try {
                JSONObject(text)
            } catch (e: Exception) {
                Log.w(tag, "bad command json: ${text.take(120)}")
                return
            }
            val id = cmd.optString("id")
            val method = cmd.optString("method")
            val params = cmd.optJSONObject("params") ?: JSONObject()
            Log.i(tag, "cmd $method (id=$id)")
            // Soft-kill: while the operator has paused control from the app, reject
            // every command locally without touching the device. The agent sees an
            // error; only the app can clear the pause. This is the on-device stop.
            if (AbacadStatus.paused) {
                Log.i(tag, "cmd $method rejected — paused")
                AbacadStatus.event("$method · rejected · paused")
                webSocket.send(
                    JSONObject().put("id", id).put("ok", false)
                        .put("error", "paused by device operator").toString(),
                )
                return
            }
            AbacadStatus.noteCommand(method)
            updateAwareness(method, params)
            val startNs = System.nanoTime()
            executor(method, params) { result ->
                val ms = (System.nanoTime() - startNs) / 1_000_000
                val reply = JSONObject().put("id", id)
                when (result) {
                    is CmdResult.Ok -> {
                        Log.i(tag, "cmd $method ok ${ms}ms")
                        AbacadStatus.event("$method · ok · ${ms}ms")
                        reply.put("ok", true).put("result", result.result)
                    }
                    is CmdResult.Err -> {
                        Log.w(tag, "cmd $method error ${ms}ms: ${result.message}")
                        AbacadStatus.event("$method · error · ${ms}ms · ${result.message}")
                        reply.put("ok", false).put("error", result.message)
                    }
                }
                webSocket.send(reply.toString())
            }
        }

        override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
            if (webSocket !== ws) return // stale socket; a newer one is already live/connecting
            connected = false
            val reason = t.message ?: t.javaClass.simpleName
            Log.w(tag, "ws failure: $reason")
            AbacadStatus.event("connection failed: $reason")
            scheduleReconnect()
        }

        override fun onClosed(webSocket: WebSocket, code: Int, reason: String) {
            if (webSocket !== ws) return
            connected = false
            Log.i(tag, "ws closed: $code $reason")
            AbacadStatus.event("connection closed: $code ${reason.ifEmpty { "(no reason)" }}")
            scheduleReconnect()
        }
    }
}
