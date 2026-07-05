package dev.abacad.probe

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
 * Outbound WebSocket to the Abacad server. Dials out (NAT-friendly), reconnects
 * with backoff, and answers each incoming command by running it through
 * [executor] (the accessibility service) and sending the reply back.
 *
 * Wire: recv {id, method, params}  ->  send {id, ok, result} | {id, ok:false, error}
 */
class DeviceClient(
    private val url: String,
    private val executor: (method: String, params: JSONObject, done: (CmdResult) -> Unit) -> Unit,
) {
    private val tag = ProbeAccessibilityService.TAG
    private val client = OkHttpClient.Builder()
        .pingInterval(20, TimeUnit.SECONDS)
        .readTimeout(0, TimeUnit.MILLISECONDS) // long-lived socket, idle between commands
        .build()
    private val handler = Handler(Looper.getMainLooper())
    private var ws: WebSocket? = null
    private var closed = false
    private var backoffMs = 1000L

    fun connect() {
        closed = false
        open()
    }

    fun close() {
        closed = true
        try { ws?.close(1000, "bye") } catch (_: Exception) {}
        ws = null
    }

    private fun open() {
        if (closed) return
        Log.i(tag, "ws connecting: $url")
        ws = client.newWebSocket(Request.Builder().url(url).build(), listener)
    }

    private fun scheduleReconnect() {
        if (closed) return
        val delay = backoffMs
        backoffMs = (backoffMs * 2).coerceAtMost(15000L)
        Log.i(tag, "ws reconnect in ${delay}ms")
        handler.postDelayed({ open() }, delay)
    }

    private val listener = object : WebSocketListener() {
        override fun onOpen(webSocket: WebSocket, response: Response) {
            Log.i(tag, "ws open -> $url")
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
            executor(method, params) { result ->
                val reply = JSONObject().put("id", id)
                when (result) {
                    is CmdResult.Ok -> reply.put("ok", true).put("result", result.result)
                    is CmdResult.Err -> {
                        Log.w(tag, "cmd $method error: ${result.message}")
                        reply.put("ok", false).put("error", result.message)
                    }
                }
                webSocket.send(reply.toString())
            }
        }

        override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
            Log.w(tag, "ws failure: ${t.message}")
            scheduleReconnect()
        }

        override fun onClosed(webSocket: WebSocket, code: Int, reason: String) {
            Log.i(tag, "ws closed: $code $reason")
            scheduleReconnect()
        }
    }
}
