package dev.abacad.probe

import android.Manifest
import android.app.Activity
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.graphics.Color
import android.graphics.ImageFormat
import android.graphics.SurfaceTexture
import android.hardware.camera2.CameraCaptureSession
import android.hardware.camera2.CameraCharacteristics
import android.hardware.camera2.CameraDevice
import android.hardware.camera2.CameraManager
import android.hardware.camera2.CaptureRequest
import android.media.ImageReader
import android.os.Bundle
import android.os.Handler
import android.os.HandlerThread
import android.util.Log
import android.util.Size
import android.view.Gravity
import android.view.Surface
import android.view.TextureView
import android.view.View
import android.widget.FrameLayout
import android.widget.TextView
import com.google.zxing.BinaryBitmap
import com.google.zxing.DecodeHintType
import com.google.zxing.PlanarYUVLuminanceSource
import com.google.zxing.common.HybridBinarizer
import com.google.zxing.qrcode.QRCodeReader

/**
 * Camera QR scanner for the connection URL. Opens the back camera with the framework
 * Camera2 API, feeds each frame's luminance plane to ZXing's [QRCodeReader], and on the
 * first successful decode returns the text via [RESULT_TEXT] and finishes. No AndroidX —
 * this keeps the probe's framework-only footprint (see build.gradle.kts).
 */
class ScanActivity : Activity() {

    companion object {
        const val RESULT_TEXT = "scanned_text"
        private const val REQ_CAMERA = 42
        private val TAG = ProbeAccessibilityService.TAG
    }

    private lateinit var textureView: TextureView
    private lateinit var hint: TextView

    private var bgThread: HandlerThread? = null
    private var bgHandler: Handler? = null
    private var camera: CameraDevice? = null
    private var session: CameraCaptureSession? = null
    private var imageReader: ImageReader? = null
    private var cameraId: String? = null
    private var previewSize = Size(1280, 720)

    private val reader = QRCodeReader()
    private val hints = mapOf(DecodeHintType.TRY_HARDER to true)
    @Volatile private var decoding = false
    @Volatile private var delivered = false

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val root = FrameLayout(this).apply { setBackgroundColor(Color.BLACK) }
        textureView = TextureView(this)
        root.addView(
            textureView,
            FrameLayout.LayoutParams(
                FrameLayout.LayoutParams.MATCH_PARENT,
                FrameLayout.LayoutParams.MATCH_PARENT,
            ),
        )
        hint = TextView(this).apply {
            text = "Point at the connection QR on the dashboard"
            setTextColor(Color.WHITE)
            setBackgroundColor(0xAA000000.toInt())
            val p = (12 * resources.displayMetrics.density).toInt()
            setPadding(p, p, p, p)
        }
        root.addView(
            hint,
            FrameLayout.LayoutParams(
                FrameLayout.LayoutParams.MATCH_PARENT,
                FrameLayout.LayoutParams.WRAP_CONTENT,
                Gravity.BOTTOM,
            ),
        )
        setContentView(root)
    }

    override fun onResume() {
        super.onResume()
        if (checkSelfPermission(Manifest.permission.CAMERA) != PackageManager.PERMISSION_GRANTED) {
            requestPermissions(arrayOf(Manifest.permission.CAMERA), REQ_CAMERA)
            return
        }
        startBackground()
        if (textureView.isAvailable) {
            openCamera()
        } else {
            textureView.surfaceTextureListener = object : TextureView.SurfaceTextureListener {
                override fun onSurfaceTextureAvailable(s: SurfaceTexture, w: Int, h: Int) = openCamera()
                override fun onSurfaceTextureSizeChanged(s: SurfaceTexture, w: Int, h: Int) {}
                override fun onSurfaceTextureDestroyed(s: SurfaceTexture) = true
                override fun onSurfaceTextureUpdated(s: SurfaceTexture) {}
            }
        }
    }

    override fun onPause() {
        closeCamera()
        stopBackground()
        super.onPause()
    }

    override fun onRequestPermissionsResult(requestCode: Int, permissions: Array<out String>, grantResults: IntArray) {
        if (requestCode == REQ_CAMERA) {
            if (grantResults.firstOrNull() == PackageManager.PERMISSION_GRANTED) {
                startBackground()
                if (textureView.isAvailable) openCamera()
            } else {
                hint.text = "Camera permission denied"
                finish()
            }
        }
    }

    private fun startBackground() {
        if (bgThread != null) return
        bgThread = HandlerThread("abacad-scan").also { it.start() }
        bgHandler = Handler(bgThread!!.looper)
    }

    private fun stopBackground() {
        bgThread?.quitSafely()
        bgThread = null
        bgHandler = null
    }

    private fun openCamera() {
        val mgr = getSystemService(Context.CAMERA_SERVICE) as CameraManager
        try {
            val id = pickBackCamera(mgr) ?: run {
                hint.text = "No camera available"
                return
            }
            cameraId = id
            val map = mgr.getCameraCharacteristics(id)
                .get(CameraCharacteristics.SCALER_STREAM_CONFIGURATION_MAP)
            previewSize = chooseSize(map?.getOutputSizes(ImageFormat.YUV_420_888))
            imageReader = ImageReader.newInstance(
                previewSize.width, previewSize.height, ImageFormat.YUV_420_888, 2,
            ).apply { setOnImageAvailableListener(onFrame, bgHandler) }
            mgr.openCamera(id, stateCallback, bgHandler)
        } catch (e: Exception) {
            Log.w(TAG, "openCamera failed: ${e.message}")
            hint.text = "Could not open camera: ${e.message}"
        }
    }

    private fun pickBackCamera(mgr: CameraManager): String? {
        var fallback: String? = null
        for (id in mgr.cameraIdList) {
            val facing = mgr.getCameraCharacteristics(id).get(CameraCharacteristics.LENS_FACING)
            if (fallback == null) fallback = id
            if (facing == CameraCharacteristics.LENS_FACING_BACK) return id
        }
        return fallback
    }

    private fun chooseSize(sizes: Array<Size>?): Size {
        if (sizes == null || sizes.isEmpty()) return Size(1280, 720)
        // Prefer the largest frame no bigger than 1080p — enough resolution to decode a
        // QR from a screen without paying for huge buffers.
        return sizes.filter { it.width <= 1920 && it.height <= 1080 }
            .maxByOrNull { it.width.toLong() * it.height }
            ?: sizes.minByOrNull { it.width.toLong() * it.height }!!
    }

    private val stateCallback = object : CameraDevice.StateCallback() {
        override fun onOpened(cam: CameraDevice) {
            camera = cam
            startPreview()
        }
        override fun onDisconnected(cam: CameraDevice) { cam.close(); camera = null }
        override fun onError(cam: CameraDevice, error: Int) {
            cam.close(); camera = null
            Log.w(TAG, "camera error $error")
        }
    }

    private fun startPreview() {
        val cam = camera ?: return
        val texture = textureView.surfaceTexture ?: return
        texture.setDefaultBufferSize(previewSize.width, previewSize.height)
        val previewSurface = Surface(texture)
        val readerSurface = imageReader!!.surface
        try {
            @Suppress("DEPRECATION")
            cam.createCaptureSession(
                listOf(previewSurface, readerSurface),
                object : CameraCaptureSession.StateCallback() {
                    override fun onConfigured(s: CameraCaptureSession) {
                        session = s
                        val req = cam.createCaptureRequest(CameraDevice.TEMPLATE_PREVIEW).apply {
                            addTarget(previewSurface)
                            addTarget(readerSurface)
                            set(CaptureRequest.CONTROL_AF_MODE, CaptureRequest.CONTROL_AF_MODE_CONTINUOUS_PICTURE)
                        }.build()
                        try {
                            s.setRepeatingRequest(req, null, bgHandler)
                        } catch (e: Exception) {
                            Log.w(TAG, "setRepeatingRequest failed: ${e.message}")
                        }
                    }
                    override fun onConfigureFailed(s: CameraCaptureSession) {
                        Log.w(TAG, "capture session config failed")
                    }
                },
                bgHandler,
            )
        } catch (e: Exception) {
            Log.w(TAG, "createCaptureSession failed: ${e.message}")
        }
    }

    private val onFrame = ImageReader.OnImageAvailableListener { r ->
        val image = r.acquireLatestImage() ?: return@OnImageAvailableListener
        if (delivered || decoding) { image.close(); return@OnImageAvailableListener }
        decoding = true
        try {
            val plane = image.planes[0]
            val w = image.width
            val h = image.height
            val rowStride = plane.rowStride
            val buffer = plane.buffer
            // Copy the Y (luminance) plane row-by-row into a tightly-packed w*h array,
            // dropping any row padding so ZXing sees a clean grayscale image.
            val data = ByteArray(w * h)
            val row = ByteArray(rowStride)
            var pos = 0
            for (y in 0 until h) {
                buffer.position(y * rowStride)
                val len = minOf(rowStride, buffer.remaining())
                buffer.get(row, 0, len)
                System.arraycopy(row, 0, data, pos, w)
                pos += w
            }
            decode(data, w, h)
        } catch (e: Exception) {
            Log.w(TAG, "frame decode error: ${e.message}")
        } finally {
            image.close()
            decoding = false
        }
    }

    private fun decode(data: ByteArray, w: Int, h: Int) {
        val source = PlanarYUVLuminanceSource(data, w, h, 0, 0, w, h, false)
        val bitmap = BinaryBitmap(HybridBinarizer(source))
        val text = try {
            reader.decode(bitmap, hints).text
        } catch (_: Exception) {
            reader.reset()
            null
        } ?: return
        if (delivered) return
        delivered = true
        Log.i(TAG, "QR scanned: ${text.take(60)}")
        runOnUiThread { deliver(text) }
    }

    private fun deliver(text: String) {
        setResult(RESULT_OK, Intent().putExtra(RESULT_TEXT, text))
        finish()
    }

    private fun closeCamera() {
        try { session?.close() } catch (_: Exception) {}
        try { camera?.close() } catch (_: Exception) {}
        try { imageReader?.close() } catch (_: Exception) {}
        session = null
        camera = null
        imageReader = null
    }
}
