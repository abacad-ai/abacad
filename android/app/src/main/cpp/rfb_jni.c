// JNI shim for the in-app RFB (VNC) server.
//
// This IS the live channel's device-side RFB server: LibVNCServer, compiled into
// our own .so (see CMakeLists.txt) and driven directly over JNI — no droidVNC-NG
// companion, nothing extra to install. Kotlin (VncServer.kt) captures the screen
// with MediaProjection, pushes each frame in via nativePushFrame, and bridges the
// server's localhost socket to the reverse-connect WebSocket.
//
// The server binds 127.0.0.1 ONLY: the framebuffer is never reachable off-device;
// only our own in-process WS<->TCP bridge connects to it.
//
// View-only in this pass: no ptr/kbd hooks are installed, so LibVNCServer silently
// drops viewer input (matches live.mode = "view"; interactive control is a
// documented follow-on that wires the hooks back into the accessibility injectors).
#include <jni.h>
#include <android/log.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <stdint.h>
#include <arpa/inet.h> // htonl, INADDR_LOOPBACK
#include <rfb/rfb.h>
#include <rfb/rfbconfig.h>

#define TAG "abacad_rfb"

// Fixed localhost port for the in-app RFB server. Only our own bridge dials it and
// it is loopback-bound, so a fixed port is safe. Must match LOCAL_PORT in VncServer.kt.
#define RFB_PORT 5901

JNIEXPORT jstring JNICALL
Java_ai_abacad_android_RfbNative_nativeVersion(JNIEnv *env, jobject thiz) {
    // Force a real LibVNCServer call so the link is genuine (kept as a smoke test).
    rfbScreenInfoPtr screen = rfbGetScreen(NULL, NULL, 32, 24, 8, 3, 4);
    int ok = (screen != NULL);
    if (screen) rfbScreenCleanup(screen);

    char buf[128];
    snprintf(buf, sizeof(buf), "LibVNCServer %s.%s.%s (rfbGetScreen: %s)",
             LIBVNCSERVER_VERSION_MAJOR,
             LIBVNCSERVER_VERSION_MINOR,
             LIBVNCSERVER_VERSION_PATCHLEVEL,
             ok ? "ok" : "FAILED");
    __android_log_print(ANDROID_LOG_INFO, TAG, "nativeVersion() -> %s", buf);
    return (*env)->NewStringUTF(env, buf);
}

// Start a localhost RFB server for a width x height ARGB surface. Returns an opaque
// handle (the rfbScreenInfoPtr) or 0 on failure.
JNIEXPORT jlong JNICALL
Java_ai_abacad_android_RfbNative_nativeStart(JNIEnv *env, jobject thiz, jint width, jint height) {
    if (width <= 0 || height <= 0) return 0;

    // 32bpp true-colour: bitsPerSample=8, samplesPerPixel=3, bytesPerPixel=4. On a
    // little-endian device this lays the framebuffer out as bytes R,G,B,X — exactly
    // Android's ImageReader PixelFormat.RGBA_8888, so frames copy in with no swizzle.
    rfbScreenInfoPtr screen = rfbGetScreen(NULL, NULL, width, height, 8, 3, 4);
    if (!screen) {
        __android_log_print(ANDROID_LOG_ERROR, TAG, "rfbGetScreen failed");
        return 0;
    }
    screen->frameBuffer = (char *)calloc((size_t)width * height * 4, 1);
    if (!screen->frameBuffer) {
        rfbScreenCleanup(screen);
        return 0;
    }
    screen->desktopName = "abacad";
    screen->alwaysShared = TRUE;

    // Localhost only: bind the IPv4 listener to 127.0.0.1 and disable the IPv6
    // listener. The screen is never exposed on the LAN.
    screen->listenInterface = htonl(INADDR_LOOPBACK);
    screen->port = RFB_PORT;
    screen->ipv6port = 0; // 0 => don't open an IPv6 listen socket

    rfbInitServer(screen);
    // Run the RFB event loop on a background thread (WITH_THREADS=ON in CMake).
    rfbRunEventLoop(screen, -1, TRUE);

    __android_log_print(ANDROID_LOG_INFO, TAG,
                        "nativeStart %dx%d on 127.0.0.1:%d", width, height, RFB_PORT);
    return (jlong)(intptr_t)screen;
}

// Copy one captured frame into the framebuffer and mark it dirty. `buf` must be a
// direct ByteBuffer (ImageReader plane). rowStride may exceed width*4 (padding).
JNIEXPORT void JNICALL
Java_ai_abacad_android_RfbNative_nativePushFrame(JNIEnv *env, jobject thiz,
                                                 jlong handle, jobject buf,
                                                 jint width, jint height, jint rowStride) {
    rfbScreenInfoPtr screen = (rfbScreenInfoPtr)(intptr_t)handle;
    if (!screen || !screen->frameBuffer) return;
    // Ignore frames whose size no longer matches (e.g. a mid-session rotation); a
    // live resize is a follow-up (rfbNewFramebuffer).
    if (width != screen->width || height != screen->height) return;

    const uint8_t *src = (const uint8_t *)(*env)->GetDirectBufferAddress(env, buf);
    if (!src) return;
    uint8_t *dst = (uint8_t *)screen->frameBuffer;
    const int rowBytes = width * 4;
    if (rowStride == rowBytes) {
        memcpy(dst, src, (size_t)rowBytes * height);
    } else {
        for (int y = 0; y < height; y++) {
            memcpy(dst + (size_t)y * rowBytes, src + (size_t)y * rowStride, (size_t)rowBytes);
        }
    }
    rfbMarkRectAsModified(screen, 0, 0, width, height);

    // Frame-flow instrumentation: prove capture is actually delivering pixels (the
    // black-screen bug looked like a live view but was really a never-updated
    // framebuffer). Log the first frame — with a cheap non-black sanity sample so a
    // solid-black capture is distinguishable from a genuine screen — then rarely.
    static unsigned long frames = 0;
    if (frames == 0 || (frames % 300) == 0) {
        // Sample the centre pixel so an all-zero (black) capture is visible in logs.
        const uint8_t *px = dst + ((size_t)(height / 2) * rowBytes) + ((size_t)(width / 2) * 4);
        __android_log_print(ANDROID_LOG_INFO, TAG,
                            "pushFrame #%lu %dx%d stride=%d centre=rgb(%u,%u,%u)",
                            frames, width, height, rowStride, px[0], px[1], px[2]);
    }
    frames++;
}

// Re-mark the whole framebuffer dirty without a new capture. VncServer calls this on
// a light timer so a STATIC screen (no ImageReader callbacks) still gets its current
// contents pushed to a viewer that connected after the last real frame — otherwise a
// motionless screen shows the stale/black initial framebuffer forever.
JNIEXPORT void JNICALL
Java_ai_abacad_android_RfbNative_nativeRefresh(JNIEnv *env, jobject thiz, jlong handle) {
    rfbScreenInfoPtr screen = (rfbScreenInfoPtr)(intptr_t)handle;
    if (!screen || !screen->frameBuffer) return;
    rfbMarkRectAsModified(screen, 0, 0, screen->width, screen->height);
}

// Stop the server and free everything. The caller must guarantee no nativePushFrame
// runs concurrently (VncServer.kt serializes push/stop on a lock and stops the
// frame source first).
JNIEXPORT void JNICALL
Java_ai_abacad_android_RfbNative_nativeStop(JNIEnv *env, jobject thiz, jlong handle) {
    rfbScreenInfoPtr screen = (rfbScreenInfoPtr)(intptr_t)handle;
    if (!screen) return;
    rfbShutdownServer(screen, TRUE); // stops the background event loop, drops clients
    char *fb = (char *)screen->frameBuffer;
    screen->frameBuffer = NULL;
    rfbScreenCleanup(screen);
    free(fb);
    __android_log_print(ANDROID_LOG_INFO, TAG, "nativeStop");
}
