package ai.abacad.android

import android.app.Activity
import android.content.Intent
import android.media.projection.MediaProjectionManager
import android.os.Bundle

/**
 * Invisible consent shim for screen recording. MediaProjection can only be
 * obtained through an Activity result, so [ScreenRecorder] launches this from the
 * (background) accessibility service — the same pattern [WakerActivity] uses. It
 * immediately fires the system "Start recording?" dialog and reports the outcome
 * (resultCode + data Intent) back via [onResult], then finishes. It renders nothing.
 *
 * The user consenting here is the one unavoidable UX break from silent operation:
 * Android has no way to grant MediaProjection without this per-session prompt.
 */
class ScreenRecordConsentActivity : Activity() {

    companion object {
        /** One-shot result sink; set by ScreenRecorder just before launch, cleared on report. */
        @Volatile
        var onResult: ((resultCode: Int, data: Intent?) -> Unit)? = null

        private const val REQ_PROJECTION = 4201
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        try {
            val mpm = getSystemService(MediaProjectionManager::class.java)
            @Suppress("DEPRECATION")
            startActivityForResult(mpm.createScreenCaptureIntent(), REQ_PROJECTION)
        } catch (e: Exception) {
            deliver(RESULT_CANCELED, null)
        }
    }

    @Deprecated("startActivityForResult is fine for a single one-shot projection grant")
    override fun onActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        super.onActivityResult(requestCode, resultCode, data)
        if (requestCode == REQ_PROJECTION) deliver(resultCode, data)
    }

    private fun deliver(resultCode: Int, data: Intent?) {
        val cb = onResult
        onResult = null
        finish()
        cb?.invoke(resultCode, data)
    }
}
