package ai.abacad.android

import java.nio.ByteBuffer

/**
 * JNI bridge to the in-app RFB (VNC) server. The RFB core (LibVNCServer) is
 * compiled into our own APK ([rfb_jni.c] / CMakeLists.txt) and driven directly, so
 * live view needs no droidVNC-NG companion — nothing extra to install.
 *
 * [VncServer] owns the lifecycle: [nativeStart] boots a localhost RFB server for a
 * fixed-size ARGB surface, [nativePushFrame] copies each MediaProjection frame into
 * its framebuffer, and [nativeStop] tears it down. View-only in this pass (no viewer
 * input hooks; see rfb_jni.c).
 */
object RfbNative {
    init {
        System.loadLibrary("abacad_rfb")
    }

    /** Returns the native RFB layer's version string (smoke test). */
    external fun nativeVersion(): String

    /** Start a localhost RFB server for a [width] x [height] surface; returns an
     *  opaque handle, or 0 on failure. */
    external fun nativeStart(width: Int, height: Int): Long

    /** Push one captured frame. [buf] must be a direct ByteBuffer (ImageReader
     *  plane); [rowStride] may exceed width*4. */
    external fun nativePushFrame(handle: Long, buf: ByteBuffer, width: Int, height: Int, rowStride: Int)

    /** Stop the server and free it. No [nativePushFrame] may run concurrently. */
    external fun nativeStop(handle: Long)
}
