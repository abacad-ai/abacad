package ai.abacad.android

import android.app.Activity
import android.content.Intent
import android.hardware.display.DisplayManager
import android.hardware.display.VirtualDisplay
import android.media.MediaRecorder
import android.media.projection.MediaProjection
import android.media.projection.MediaProjectionManager
import android.os.Build
import android.os.Handler
import android.os.Looper
import android.util.Log
import org.json.JSONObject
import java.io.File
import java.util.concurrent.Executors

/**
 * The Android file channel of screen_recording: captures the display via
 * MediaProjection into a MediaRecorder-backed VirtualDisplay, writing an H.264
 * .mp4, then uploads it to /blobs on stop — the moving-picture counterpart of the
 * accessibility screenshot.
 *
 * MediaProjection needs a per-session user consent ([ScreenRecordConsentActivity]),
 * so `start` is asynchronous: it returns state "requesting_permission" and the
 * recording only begins once the user taps "Start now". The agent polls `status`
 * until it reads "recording", then "ready" with a blob id after `stop`.
 *
 * One recording at a time. The transfer is async (a big clip never blocks the
 * command window); the temp file is deleted after a successful upload (automatic
 * retention). Video only — MediaProjection can capture audio, but the live/VNC
 * channel can't, so audio is kept off for symmetry.
 */
class ScreenRecorder(private val service: AbacadAccessibilityService) {

    private val main = Handler(Looper.getMainLooper())
    private val io = Executors.newSingleThreadExecutor()

    // All state below is guarded by `this`.
    private var phase = "idle" // idle | requesting | recording | uploading | ready | failed
    private var projection: MediaProjection? = null
    private var recorder: MediaRecorder? = null
    private var virtualDisplay: VirtualDisplay? = null
    private var path = ""
    private var startAtMs = 0L
    private var width = 0
    private var height = 0
    private var fps = 0
    private var durationMs = 0L
    private var sizeBytes = 0L
    private var blobId = ""
    private var sha256 = ""
    private var error = ""
    private var blobs: BlobClient? = null

    private val projectionCallback = object : MediaProjection.Callback() {
        override fun onStop() { /* teardown happens in stopInternal */ }
    }

    /** Dispatch a screen_recording action, calling [done] exactly once. */
    fun handle(params: JSONObject, blobClient: BlobClient?, done: (CmdResult) -> Unit) {
        when (params.optString("action")) {
            "start" -> start(params.optJSONObject("file") ?: JSONObject(), blobClient, done)
            "stop" -> done(CmdResult.Ok(stop()))
            "status" -> done(CmdResult.Ok(synchronized(this) { statusJson() }))
            else -> done(CmdResult.Err("screen_recording action must be \"start\", \"stop\", or \"status\""))
        }
    }

    @Synchronized
    private fun start(file: JSONObject, blobClient: BlobClient?, done: (CmdResult) -> Unit) {
        if (phase == "recording" || phase == "requesting") {
            done(CmdResult.Err("a recording is already in progress; stop it first")); return
        }
        if (blobClient == null) {
            done(CmdResult.Err("screen recording needs the /blobs data plane, which is not configured on this device")); return
        }
        blobs = blobClient
        fps = file.optInt("fps", 0).let { if (it <= 0) 30 else it }
        phase = "requesting"
        blobId = ""; sha256 = ""; error = ""; sizeBytes = 0; durationMs = 0

        val capSecs = file.optInt("max_duration_seconds", 0)

        // Launch the consent shim; begin recording (or fail) when the user answers.
        ScreenRecordConsentActivity.onResult = { resultCode, data ->
            main.post { onConsent(resultCode, data, capSecs) }
        }
        val intent = Intent(service, ScreenRecordConsentActivity::class.java)
            .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
        try {
            service.startActivity(intent)
            done(CmdResult.Ok(statusJson())) // state = requesting; agent polls status
        } catch (e: Exception) {
            phase = "failed"; error = "could not request screen-recording permission: ${e.message}"
            done(CmdResult.Ok(statusJson()))
        }
    }

    @Synchronized
    private fun onConsent(resultCode: Int, data: Intent?, capSecs: Int) {
        if (phase != "requesting") return
        if (resultCode != Activity.RESULT_OK || data == null) {
            phase = "failed"; error = "screen-recording permission denied"; return
        }
        try {
            val metrics = service.resources.displayMetrics
            val w = metrics.widthPixels and 1.inv()
            val h = metrics.heightPixels and 1.inv()
            val dpi = metrics.densityDpi

            // API 34+: a foreground service of type mediaProjection must be running
            // before getMediaProjection. The accessibility service is already
            // foreground; promote it to include the mediaProjection type.
            service.promoteForegroundForRecording()

            val mpm = service.getSystemService(MediaProjectionManager::class.java)
            val mp = mpm.getMediaProjection(resultCode, data)
                ?: throw IllegalStateException("null MediaProjection")
            mp.registerCallback(projectionCallback, main)

            val outFile = File(service.cacheDir, "abacad-rec-${System.currentTimeMillis()}.mp4")
            val rec = newRecorder()
            rec.setVideoSource(MediaRecorder.VideoSource.SURFACE)
            rec.setOutputFormat(MediaRecorder.OutputFormat.MPEG_4)
            rec.setVideoEncoder(MediaRecorder.VideoEncoder.H264)
            rec.setVideoSize(w, h)
            rec.setVideoFrameRate(fps)
            rec.setVideoEncodingBitRate(estimateBitrate(w, h, fps))
            rec.setOutputFile(outFile.absolutePath)
            rec.prepare()

            val vd = mp.createVirtualDisplay(
                "abacad-rec", w, h, dpi,
                DisplayManager.VIRTUAL_DISPLAY_FLAG_AUTO_MIRROR,
                rec.surface, null, null,
            )
            rec.start()

            projection = mp
            recorder = rec
            virtualDisplay = vd
            path = outFile.absolutePath
            width = w; height = h
            startAtMs = System.currentTimeMillis()
            phase = "recording"

            if (capSecs > 0) main.postDelayed({ stop() }, capSecs * 1000L)
        } catch (e: Exception) {
            Log.w(AbacadAccessibilityService.TAG, "screen recording start failed: ${e.message}")
            phase = "failed"; error = e.message ?: e.toString()
            teardownCapture()
            service.demoteForegroundAfterRecording()
        }
    }

    @Synchronized
    fun stop(): JSONObject {
        if (phase != "recording") return statusJson()
        durationMs = System.currentTimeMillis() - startAtMs
        val file = path
        val client = blobs
        try {
            recorder?.stop()
        } catch (e: Exception) {
            Log.w(AbacadAccessibilityService.TAG, "recorder.stop: ${e.message}")
        }
        teardownCapture()
        service.demoteForegroundAfterRecording()
        sizeBytes = fileSize(file)
        phase = "uploading"

        // Upload off the main thread; flip to ready/failed when it settles.
        io.execute {
            if (client == null || sizeBytes == 0L) {
                synchronized(this) {
                    phase = "failed"
                    if (error.isEmpty()) error = if (sizeBytes == 0L) "recording produced no data" else "file transfer is not configured"
                }
                File(file).delete()
                return@execute
            }
            try {
                val (id, _, sha) = client.upload(file)
                synchronized(this) { phase = "ready"; blobId = id; sha256 = sha }
                File(file).delete() // auto-retention: keep only the store copy
            } catch (e: Exception) {
                synchronized(this) { phase = "failed"; error = e.message ?: e.toString() }
            }
        }
        return statusJson()
    }

    /** Release capture resources (recorder/virtual display/projection). Caller holds the lock. */
    private fun teardownCapture() {
        try { virtualDisplay?.release() } catch (_: Exception) {}
        try { recorder?.reset(); recorder?.release() } catch (_: Exception) {}
        try { projection?.unregisterCallback(projectionCallback); projection?.stop() } catch (_: Exception) {}
        virtualDisplay = null
        recorder = null
        projection = null
    }

    private fun newRecorder(): MediaRecorder =
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) MediaRecorder(service)
        else @Suppress("DEPRECATION") MediaRecorder()

    /** Renders the current state; caller holds the lock (or the value is stable). */
    private fun statusJson(): JSONObject {
        val o = JSONObject().put("state", if (phase == "requesting") "requesting_permission" else phase)
        if (width > 0) o.put("width", width)
        if (height > 0) o.put("height", height)
        if (fps > 0) o.put("fps", fps)
        when (phase) {
            "recording" -> {
                o.put("elapsed_ms", System.currentTimeMillis() - startAtMs)
                o.put("size_bytes", fileSize(path))
            }
            "uploading", "ready", "failed" -> {
                o.put("duration_ms", durationMs)
                o.put("size_bytes", sizeBytes)
                o.put("codec", "h264")
                o.put("transfer_state", if (phase == "ready") "ready" else if (phase == "failed") "failed" else "uploading")
                if (blobId.isNotEmpty()) o.put("blob_id", blobId)
                if (sha256.isNotEmpty()) o.put("sha256", sha256)
                if (error.isNotEmpty()) o.put("error", error)
            }
        }
        return o
    }

    private fun fileSize(p: String): Long = try { File(p).length() } catch (_: Exception) { 0L }

    /** A generous bitrate so the artifact stays high quality; ~0.2 bits/pixel/frame, clamped. */
    private fun estimateBitrate(w: Int, h: Int, fps: Int): Int =
        (w.toLong() * h * fps / 5).coerceIn(4_000_000L, 40_000_000L).toInt()
}
