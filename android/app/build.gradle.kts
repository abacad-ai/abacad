plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
}

// Release signing. Android has no unsigned install path, and the key that signs
// a release is permanent: an update must carry the same signature or users have
// to uninstall first (losing their pairing). So the keystore lives OUTSIDE the
// repo and outside any build tree, referenced from ~/.gradle/gradle.properties:
//
//   abacadReleaseStoreFile=~/.abacad/android-release.jks
//   abacadReleaseStorePassword=...
//   abacadReleaseKeyAlias=abacad
//   abacadReleaseKeyPassword=...
//
// See ../README.md for how to create it. Debug builds are unaffected — Gradle
// keeps auto-signing them with ~/.android/debug.keystore.
fun releaseProp(name: String) = (findProperty(name) as String?)?.takeIf { it.isNotBlank() }

// One monorepo version, from the repo-root VERSION file (one level above the
// android/ gradle root). versionName is that string verbatim; versionCode is
// derived from it as a monotonic integer (0.4.0 -> 400, 1.2.3 -> 10203) so it
// climbs on its own as the version climbs — no hand-maintained counter. Any
// pre-release suffix (0.4.0-rc1) is dropped for the numeric code.
val monorepoVersion = rootProject.file("../VERSION").readText().trim()
val monorepoVersionCode = monorepoVersion.substringBefore("-").substringBefore("+").split(".").let {
    val major = it.getOrNull(0)?.toIntOrNull() ?: 0
    val minor = it.getOrNull(1)?.toIntOrNull() ?: 0
    val patch = it.getOrNull(2)?.toIntOrNull() ?: 0
    major * 10000 + minor * 100 + patch
}

val releaseKeystore = releaseProp("abacadReleaseStoreFile")
    ?.replaceFirst(Regex("^~"), System.getProperty("user.home"))
    ?.let { path -> File(path) }
    ?.takeIf { it.isFile }

android {
    namespace = "ai.abacad.android"
    compileSdk = 34

    // NDK pinned to AGP 8.5.2's default so externalNativeBuild needs no toolchain
    // hunting. The in-app RFB server (LibVNCServer, C) is compiled here.
    ndkVersion = "26.1.10909125"

    defaultConfig {
        applicationId = "ai.abacad.android"
        minSdk = 30          // Android 11 — AccessibilityService.takeScreenshot() lives here
        targetSdk = 34
        versionCode = monorepoVersionCode
        versionName = monorepoVersion

        // One APK carrying every ABI we support — which is what the published
        // name has always claimed. It used to be arm64-v8a alone while shipping
        // as "…-android-universal.apk", so it refused to install on anything
        // else, emulators included; adding x86_64 makes the name true.
        //
        // ── Why one fat APK and NOT per-ABI splits ─────────────────────────
        // Splits were implemented and measured before being dropped. Actual
        // v0.4.2 release builds, R8 on:
        //
        //     arm64-v8a only     2,240,684 B   (2.14 MB)
        //     x86_64 only        2,272,013 B   (2.17 MB)
        //     universal (this)   2,501,465 B   (2.39 MB)
        //
        // So the best case for splitting — an arm64 phone — saves 260,781 B:
        // 255 KB, about 10% of a 2.4 MB download. The native library is the
        // only per-ABI content in the package, and it is just ~224 KB
        // (arm64-v8a) / ~255 KB (x86_64); everything else is shared dex and
        // resources, which every split would carry in full anyway.
        //
        // What that 255 KB would have cost: three release artifacts instead of
        // one, three download buttons where the user has to know their own ABI,
        // a per-ABI versionCode offset scheme (APKs sharing a versionCode are
        // not updates to each other, so moving between them breaks without
        // one), and a 3x longer native CI build. Not worth it.
        //
        // It is the arithmetic that makes splits wrong here, not the principle:
        // revisit if the native side ever grows by an order of magnitude.
        //
        // armeabi-v7a stays out: minSdk is 30, and a 32-bit-only Android 11
        // device is close to extinct. The CMake build is ABI-agnostic, so it is
        // one word here if one ever shows up.
        ndk {
            abiFilters += listOf("arm64-v8a", "x86_64")
        }
    }

    externalNativeBuild {
        cmake {
            path = file("src/main/cpp/CMakeLists.txt")
            version = "3.22.1"
        }
    }

    signingConfigs {
        if (releaseKeystore != null) {
            create("release") {
                storeFile = releaseKeystore
                storePassword = releaseProp("abacadReleaseStorePassword")
                keyAlias = releaseProp("abacadReleaseKeyAlias")
                keyPassword = releaseProp("abacadReleaseKeyPassword")
                // Both, explicitly: setting either flag turns off AGP's
                // automatic choice for the other. v2 is what Android 11 (our
                // minSdk) verifies; v3 is the scheme that carries
                // proof-of-rotation, i.e. the only way to ever move off this key
                // without forcing everyone to uninstall.
                enableV2Signing = true
                enableV3Signing = true
            }
        }
    }

    buildTypes {
        release {
            // R8: shrink + obfuscate the dex and drop unreferenced resources.
            //
            // This is the lever that actually moves APK size, and by a lot —
            // measured on v0.4.2:
            //
            //     minify off   18,754,025 B   (17.9 MB)
            //     minify on     2,501,465 B    (2.4 MB)   -87%
            //
            // which is why ABI splitting was not worth doing (see defaultConfig
            // above): almost none of this package is native code. It is dex and
            // resources, most of it Compose/Material3 that the app never calls.
            //
            // proguard-rules.pro documents every name that something outside
            // the Kotlin code (the JNI linker, the OS, our own crash reporting)
            // depends on. That list is short because all JSON here is
            // hand-rolled org.json with string-literal keys; read the header of
            // that file before adding a reflective library.
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(
                getDefaultProguardFile("proguard-android-optimize.txt"),
                "proguard-rules.pro",
            )
            // Deliberately no fallback to the debug key: a release APK signed
            // with a throwaway key is worse than no APK, because shipping it
            // once locks users out of every properly signed update.
            if (releaseKeystore != null) {
                signingConfig = signingConfigs.getByName("release")
            }
        }
    }

    // BuildConfig.VERSION_NAME is what the device client reports to the relay
    // (?version= on the dial). AGP 8 no longer generates BuildConfig unless asked.
    buildFeatures {
        buildConfig = true
        compose = true
    }

    // Compose compiler 1.5.14 pairs with Kotlin 1.9.24 (this repo's Kotlin plugin
    // version). Bump both together if either moves.
    composeOptions {
        kotlinCompilerExtensionVersion = "1.5.14"
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    kotlinOptions {
        jvmTarget = "17"
    }
}

dependencies {
    // OkHttp: outbound WebSocket to the abacad server. The device-control core
    // stays on framework APIs; only the UI (below) uses AndroidX.
    implementation("com.squareup.okhttp3:okhttp:4.12.0")
    // ZXing core: pure-Java QR decoder. Paired with the framework Camera2 API so
    // we can scan the connection QR without pulling in CameraX/ML Kit.
    implementation("com.google.zxing:core:3.5.3")

    // Jetpack Compose + Material 3 — the setup/awareness UI (MainActivity). The
    // BOM pins a mutually-compatible set; 2024.06 maps to Compose UI 1.6 /
    // Material3 1.2, which the 1.5.14 compiler (Kotlin 1.9.24) builds.
    implementation(platform("androidx.compose:compose-bom:2024.06.00"))
    implementation("androidx.compose.ui:ui")
    implementation("androidx.compose.ui:ui-graphics")
    implementation("androidx.compose.foundation:foundation")
    implementation("androidx.compose.material3:material3")
    implementation("androidx.activity:activity-compose:1.9.0")
    implementation("androidx.lifecycle:lifecycle-runtime-compose:2.8.2")
    implementation("androidx.compose.ui:ui-tooling-preview")
    debugImplementation("androidx.compose.ui:ui-tooling")

    // JVM unit tests. Enrollment is deliberately free of android.* imports so its
    // parsing and URL logic can be tested without a device — the build machine
    // can assemble an APK but never run one, so an untested path is an unverified
    // path. The real org.json shadows android.jar's stub, whose methods otherwise
    // throw "not mocked" under the JVM test runtime.
    testImplementation("junit:junit:4.13.2")
    testImplementation("org.json:json:20240303")
}

// Without a signing config AGP would quietly emit app-release-unsigned.apk,
// which no phone will install. Say so up front instead.
tasks.matching { it.name == "assembleRelease" }.configureEach {
    doFirst {
        if (releaseKeystore == null) {
            throw GradleException(
                "Release signing is not configured — see android/README.md " +
                    "(set abacadReleaseStoreFile & friends in ~/.gradle/gradle.properties). " +
                    "For a local build use ./gradlew assembleDebug."
            )
        }
    }
}
