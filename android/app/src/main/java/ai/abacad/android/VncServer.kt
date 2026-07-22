package ai.abacad.android

import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import okio.ByteString
import okio.ByteString.Companion.toByteString
import org.json.JSONObject
import java.io.DataInputStream
import java.io.IOException
import java.io.PipedInputStream
import java.io.PipedOutputStream

/**
 * The Android live channel (screen_recording live): a minimal, view-only RFB (VNC)
 * server spoken over the dedicated reverse-connect WebSocket — mirrors
 * macos/VNCServer.swift and windows/VncServer.cs. On "start" it dials the server's
 * VNC ingress and serves RFB: banner + security(None) + ServerInit, then a
 * Raw-encoded BGRX framebuffer (from the accessibility screenshot) per request.
 * Input messages are parsed and dropped — view only for now.
 *
 * Frames come from the accessibility screenshot (no MediaProjection consent), which
 * is rate-limited to ~3 fps by the platform — a low but silent live view. The
 * pixels ride this dedicated WS, never the command socket.
 *
 * UNVERIFIED at runtime: the RFB byte protocol needs a real noVNC client to shake
 * out, and full-frame Raw at phone resolution is bandwidth-heavy.
 */
class VncServer(private val service: AbacadAccessibilityService) {
    private val client = OkHttpClient.Builder().build()

    @Volatile private var running = false
    private var ws: WebSocket? = null

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

        val pin = PipedInputStream(1 shl 20)
        val pout = PipedOutputStream(pin)
        running = true

        val socket = client.newWebSocket(
            Request.Builder().url(url).build(),
            object : WebSocketListener() {
                override fun onMessage(webSocket: WebSocket, bytes: ByteString) {
                    try {
                        pout.write(bytes.toByteArray())
                        pout.flush()
                    } catch (_: Exception) {
                    }
                }

                override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) = stop()
                override fun onClosing(webSocket: WebSocket, code: Int, reason: String) = stop()
            },
        )
        ws = socket

        Thread {
            try {
                serve(DataInputStream(pin), socket)
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
        ws = null
    }

    private fun serve(inp: DataInputStream, socket: WebSocket) {
        write(socket, "RFB 003.008\n".toByteArray()) // ProtocolVersion
        readN(inp, 12) // client version
        write(socket, byteArrayOf(1, 1)) // 1 security type: None(1)
        readN(inp, 1) // client selects
        write(socket, byteArrayOf(0, 0, 0, 0)) // SecurityResult OK
        readN(inp, 1) // ClientInit (shared flag)

        var frame = service.captureRawBGRA() ?: throw IOException("no frame")
        write(socket, serverInit(frame.first, frame.second))

        while (running) {
            val type = readN(inp, 1)[0].toInt() and 0xff
            when (type) {
                0 -> readN(inp, 19) // SetPixelFormat (ignored; keep BGRX)
                2 -> { // SetEncodings
                    val hdr = readN(inp, 3)
                    val count = ((hdr[1].toInt() and 0xff) shl 8) or (hdr[2].toInt() and 0xff)
                    if (count > 0) readN(inp, count * 4)
                }
                3 -> { // FramebufferUpdateRequest
                    readN(inp, 9)
                    frame = service.captureRawBGRA() ?: continue
                    write(socket, framebufferUpdate(frame))
                }
                4 -> readN(inp, 7) // KeyEvent
                5 -> readN(inp, 5) // PointerEvent
                6 -> { // ClientCutText
                    val h = readN(inp, 7)
                    val n = ((h[3].toInt() and 0xff) shl 24) or ((h[4].toInt() and 0xff) shl 16) or
                        ((h[5].toInt() and 0xff) shl 8) or (h[6].toInt() and 0xff)
                    if (n > 0) readN(inp, n)
                }
                else -> throw IOException("unknown RFB client message $type")
            }
        }
    }

    private fun readN(inp: DataInputStream, n: Int): ByteArray {
        val b = ByteArray(n)
        inp.readFully(b)
        return b
    }

    private fun write(socket: WebSocket, bytes: ByteArray) {
        socket.send(bytes.toByteString(0, bytes.size))
    }

    private fun serverInit(w: Int, h: Int): ByteArray {
        val name = "abacad".toByteArray()
        val b = ArrayList<Byte>()
        b.addAll(be16(w).toList())
        b.addAll(be16(h).toList())
        // 32bpp, depth 24, little-endian, true-colour, BGRX (redShift 16/green 8/blue 0).
        b.addAll(byteArrayOf(32, 24, 0, 1, 0, 255.toByte(), 0, 255.toByte(), 0, 255.toByte(), 16, 8, 0, 0, 0, 0).toList())
        b.addAll(be32(name.size).toList())
        b.addAll(name.toList())
        return b.toByteArray()
    }

    private fun framebufferUpdate(f: Triple<Int, Int, ByteArray>): ByteArray {
        val out = java.io.ByteArrayOutputStream(f.third.size + 16)
        out.write(byteArrayOf(0, 0)) // message type 0, padding
        out.write(be16(1))           // one rectangle
        out.write(be16(0))           // x
        out.write(be16(0))           // y
        out.write(be16(f.first))     // width
        out.write(be16(f.second))    // height
        out.write(be32(0))           // encoding 0 = Raw
        out.write(f.third)           // BGRX pixels
        return out.toByteArray()
    }

    private fun be16(v: Int) = byteArrayOf(((v shr 8) and 0xff).toByte(), (v and 0xff).toByte())
    private fun be32(v: Int) = byteArrayOf(
        ((v shr 24) and 0xff).toByte(), ((v shr 16) and 0xff).toByte(),
        ((v shr 8) and 0xff).toByte(), (v and 0xff).toByte(),
    )
}
