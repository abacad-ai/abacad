# R8 rules for the release build.
#
# This app turns out to be unusually R8-friendly, and it is worth writing down
# why so nobody weakens it by accident: every JSON field name on the wire is a
# STRING LITERAL in a hand-rolled org.json call (DeviceClient.kt,
# AbacadAccessibilityService.kt, ScreenRecorder.kt, BlobClient.kt). Nothing maps
# a Kotlin property name to a protocol key, so obfuscation physically cannot
# rename a field the server reads. There is also no Class.forName, no
# getDeclaredMethod, and no reflective serializer anywhere in the tree.
#
# If you ever swap the hand-rolled JSON for Gson/Moshi/kotlinx.serialization,
# that guarantee dies and every model class needs a keep rule. Don't do it
# casually.
#
# What follows is only the handful of places where something OUTSIDE the Kotlin
# code holds onto a name: the JNI linker, the Android system, and our own
# failure reporting. Each one fails at RUNTIME rather than at build time, which
# is exactly why they are enumerated here rather than discovered later.

# ── JNI: the LibVNCServer bridge ─────────────────────────────────────────────
# libabacad_rfb.so resolves these by mangled symbol name
# (Java_ai_abacad_android_RfbNative_nativeStart, …). rfb_jni.c contains no
# RegisterNatives call, so the mangled name IS the contract: rename the class or
# any method and you get UnsatisfiedLinkError. It throws the first time live
# view starts — not at launch — so a build that boots fine can still be broken.
-keep class ai.abacad.android.RfbNative { *; }
-keepclasseswithmembernames class * {
    native <methods>;
}

# ── Components the SYSTEM instantiates from a manifest string ────────────────
# proguard-android-optimize.txt already keeps Activity/Service subclasses, so
# this is belt-and-braces. It is here anyway because the failure mode is the
# worst one in the app: the accessibility service is bound by the OS using the
# name in AndroidManifest.xml, and a failed binding produces no crash at all —
# just a toggle in system Settings that silently does nothing, on a product
# whose entire function is that service.
-keep class ai.abacad.android.AbacadAccessibilityService { *; }

# ── Keep exception class names ───────────────────────────────────────────────
# DeviceClient.kt falls back to `t.javaClass.simpleName` when a Throwable has no
# message, and that string is not just logged — it is sent to the relay as the
# connection-failure reason. Without this, the field that tells you why devices
# are dropping degrades to "a" / "b" in exactly the situation where you need it.
-keepnames class * extends java.lang.Throwable

# ── Readable stack traces ────────────────────────────────────────────────────
# Keep line numbers and hide the original source file name; combined with the
# mapping.txt that R8 writes to app/build/outputs/mapping/release/, this keeps
# obfuscated crashes deobfuscatable. Archive that mapping file per release — it
# is the only thing that can decode a stack trace from a shipped APK.
-keepattributes SourceFile,LineNumberTable
-renamesourcefileattribute SourceFile
