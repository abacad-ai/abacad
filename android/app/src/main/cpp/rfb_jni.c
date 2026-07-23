// JNI shim for the in-app RFB server.
//
// Spike B: prove LibVNCServer compiles + links into our .so and is callable at
// runtime. nativeVersion() reports the linked LibVNCServer version and exercises
// a real server call (rfbGetScreen/rfbScreenCleanup) so the linker must pull in
// the library, not just its headers.
#include <jni.h>
#include <android/log.h>
#include <stdio.h>
#include <rfb/rfb.h>
#include <rfb/rfbconfig.h>

#define TAG "abacad_rfb"

JNIEXPORT jstring JNICALL
Java_ai_abacad_android_RfbNative_nativeVersion(JNIEnv *env, jobject thiz) {
    // Force a real LibVNCServer call so the link is genuine.
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
