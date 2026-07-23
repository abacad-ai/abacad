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

        // 64-bit only for now: the live-view target devices are arm64, and it
        // keeps the LibVNCServer build (Spike B) to a single ABI while we prove
        // the pipeline. Add armeabi-v7a here if a 32-bit device needs it.
        ndk {
            abiFilters += "arm64-v8a"
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
            isMinifyEnabled = false
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
