package ai.abacad.android

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
 */
class DeviceClient(
    private val url: String,
    private val executor: (method: String, params: JSONObject, done: (CmdResult) -> Unit) -> Unit,
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

    private fun open() {
        if (closed) return
        Log.i(tag, "ws connecting: $url")
        AbacadStatus.setState(AbacadStatus.State.CONNECTING, "connecting to $url")
        ws = client.newWebSocket(Request.Builder().url(url).build(), listener)
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
            Log.i(tag, "ws open -> $url")
            connected = true
            AbacadStatus.setState(AbacadStatus.State.CONNECTED, "connected to $url")
            backoffMs = 1000L
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
