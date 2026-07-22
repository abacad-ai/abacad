package ai.abacad.android

import android.net.Uri
import android.system.Os
import okhttp3.MediaType.Companion.toMediaTypeOrNull
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.asRequestBody
import org.json.JSONObject
import java.io.File
import java.io.FileOutputStream
import java.io.IOException
import java.security.DigestOutputStream
import java.security.MessageDigest
import java.util.concurrent.TimeUnit

/**
 * Device side of the /blobs data plane, backing the push_file / pull_file verbs.
 * File bytes ride HTTP — not the command WebSocket — so a large file never has to
 * be base64'd onto a text frame. Authenticated with the same per-device token the
 * socket uses, carried in the Authorization header.
 *
 * Streamed end to end: [download] copies the response body straight to a temp file
 * (then renames), [upload] posts the file handle as the request body. Neither
 * buffers the whole object in memory.
 */
class BlobClient private constructor(
    private val base: String,   // e.g. https://host/blobs
    private val token: String?,
) {
    // No call timeout: a multi-GB transfer may legitimately run long. The socket's
    // own read timeout doesn't apply to these one-shot HTTP calls.
    private val client = OkHttpClient.Builder()
        .callTimeout(0, TimeUnit.MILLISECONDS)
        .build()

    private fun auth(b: Request.Builder) {
        token?.let { b.header("Authorization", "Bearer $it") }
    }

    /**
     * Stream the blob to [destPath] and return (bytesWritten, hexSha256). Writes to
     * a temp file in the destination directory and renames into place, so a reader
     * never observes a half-written file. The parent directory must already exist.
     */
    fun download(blobId: String, destPath: String, mode: Int): Pair<Long, String> {
        val req = Request.Builder().url("$base/$blobId")
        auth(req)
        client.newCall(req.build()).execute().use { resp ->
            if (!resp.isSuccessful) {
                throw IOException("blob download failed: ${resp.code}${bodySnippet(resp)}")
            }
            val dest = File(destPath)
            val dir = dest.parentFile ?: throw IOException("no parent directory for $destPath")
            val tmp = File.createTempFile(".abacad-dl-", null, dir)
            val digest = MessageDigest.getInstance("SHA-256")
            var size = 0L
            try {
                resp.body!!.byteStream().use { input ->
                    DigestOutputStream(FileOutputStream(tmp), digest).use { out ->
                        val buf = ByteArray(64 * 1024)
                        while (true) {
                            val n = input.read(buf)
                            if (n < 0) break
                            out.write(buf, 0, n)
                            size += n
                        }
                    }
                }
                Os.chmod(tmp.absolutePath, mode)
                if (!tmp.renameTo(dest)) throw IOException("could not move file into $destPath")
            } catch (e: Exception) {
                tmp.delete()
                throw e
            }
            return Pair(size, hex(digest.digest()))
        }
    }

    /** Stream [srcPath] to /blobs and return (blobId, size, hexSha256). */
    fun upload(srcPath: String): Triple<String, Long, String> {
        val f = File(srcPath)
        if (!f.exists()) throw IOException("no such file: $srcPath")
        if (f.isDirectory) throw IOException("$srcPath is a directory, not a file")
        val body = f.asRequestBody("application/octet-stream".toMediaTypeOrNull())
        val req = Request.Builder().url(base).post(body)
        auth(req)
        client.newCall(req.build()).execute().use { resp ->
            if (resp.code != 201) {
                throw IOException("blob upload failed: ${resp.code}${bodySnippet(resp)}")
            }
            val json = JSONObject(resp.body!!.string())
            return Triple(json.getString("id"), json.getLong("size"), json.getString("sha256"))
        }
    }

    private fun bodySnippet(resp: okhttp3.Response): String {
        val s = try { resp.peekBody(200).string().trim() } catch (_: Exception) { "" }
        return if (s.isEmpty()) "" else " — $s"
    }

    companion object {
        /**
         * Derive the /blobs endpoint from the relay URL: same host, over http(s)
         * instead of ws(s). Returns null if the URL can't be parsed into an
         * ws/wss host, which disables file transfer rather than guessing.
         */
        fun fromServerUrl(rawUrl: String): BlobClient? {
            val uri = Uri.parse(rawUrl)
            val scheme = when (uri.scheme?.lowercase()) {
                "wss" -> "https"
                "ws" -> "http"
                else -> return null
            }
            val host = uri.host ?: return null
            val port = if (uri.port > 0) ":${uri.port}" else ""
            val base = "$scheme://$host$port/blobs"
            return BlobClient(base, uri.getQueryParameter("token"))
        }

        private val HEX = "0123456789abcdef".toCharArray()
        private fun hex(bytes: ByteArray): String {
            val out = CharArray(bytes.size * 2)
            for (i in bytes.indices) {
                val v = bytes[i].toInt() and 0xff
                out[i * 2] = HEX[v ushr 4]
                out[i * 2 + 1] = HEX[v and 0x0f]
            }
            return String(out)
        }
    }
}
