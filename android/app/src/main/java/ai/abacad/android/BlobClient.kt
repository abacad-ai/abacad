package ai.abacad.android

import android.net.Uri
import android.os.Environment
import android.system.ErrnoException
import android.system.Os
import android.util.Log
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
     * never observes a half-written file. Missing parent directories are created
     * (best effort) so a push to a fresh folder doesn't require pre-`mkdir`.
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
            // Create the destination tree if it doesn't exist yet. mkdirs() returns
            // false when it can't (no permission, or already present) — don't fail on
            // that here; the temp-file create below reports the real, path-specific error.
            if (!dir.exists()) dir.mkdirs()
            val tmp = try {
                File.createTempFile(".abacad-dl-", null, dir)
            } catch (e: IOException) {
                throw explainWriteFailure(e, destPath)
            }
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
                // Emulated/FUSE external storage (/sdcard) ignores unix perms and throws
                // EPERM on chmod even with All-files access — so this is best effort, not
                // fatal. On real filesystems (app dirs, removable cards) it applies normally.
                try {
                    Os.chmod(tmp.absolutePath, mode)
                } catch (e: ErrnoException) {
                    Log.d(TAG, "chmod skipped on $destPath (emulated storage?): ${e.message}")
                }
                if (!tmp.renameTo(dest)) throw IOException("could not move file into $destPath")
            } catch (e: Exception) {
                tmp.delete()
                throw explainWriteFailure(e, destPath)
            }
            return Pair(size, hex(digest.digest()))
        }
    }

    /**
     * Turn a raw filesystem error into an actionable one when the likely cause is a
     * missing All-files-access grant: a permission/ENOENT failure writing to *shared*
     * storage while [Environment.isExternalStorageManager] is false. Anything else
     * (app-owned dirs, shell-only paths like /data/local/tmp, real I/O errors) passes
     * through unchanged — All-files access wouldn't fix those and we shouldn't imply it.
     */
    private fun explainWriteFailure(e: Exception, destPath: String): Exception {
        val msg = e.message ?: ""
        val permissionish = msg.contains("EACCES") || msg.contains("EPERM") ||
            msg.contains("ENOENT") || msg.contains("Permission denied") ||
            msg.contains("Operation not permitted") || msg.contains("No such file")
        val shared = destPath.startsWith("/sdcard/") || destPath.startsWith("/storage/")
        if (permissionish && shared && !Environment.isExternalStorageManager()) {
            return IOException(
                "cannot write $destPath — grant \"Files & media access\" in the abacad app " +
                    "(Setup → Files & media access), then retry",
            )
        }
        return e
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
        private const val TAG = "ABACAD"

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
