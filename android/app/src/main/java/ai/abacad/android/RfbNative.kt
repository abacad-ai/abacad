package ai.abacad.android

/**
 * JNI bridge to the in-app RFB (VNC) server. Replaces the droidVNC-NG companion:
 * the RFB core (LibVNCServer) is compiled into our own APK and driven directly,
 * so live view needs no second app installed.
 *
 * Spike A: only [nativeVersion] is wired, to prove the native lib builds and loads.
 */
object RfbNative {
    init {
        System.loadLibrary("abacad_rfb")
    }

    /** Returns the native RFB layer's version string. */
    external fun nativeVersion(): String
}
