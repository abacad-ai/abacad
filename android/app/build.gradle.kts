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

val releaseKeystore = releaseProp("abacadReleaseStoreFile")
    ?.replaceFirst(Regex("^~"), System.getProperty("user.home"))
    ?.let { path -> File(path) }
    ?.takeIf { it.isFile }

android {
    namespace = "ai.abacad.android"
    compileSdk = 34

    defaultConfig {
        applicationId = "ai.abacad.android"
        minSdk = 30          // Android 11 — AccessibilityService.takeScreenshot() lives here
        targetSdk = 34
        versionCode = 3
        versionName = "0.3-swipe"
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

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    kotlinOptions {
        jvmTarget = "17"
    }
}

dependencies {
    // OkHttp: outbound WebSocket to the abacad server. Everything else stays on
    // framework APIs (no AndroidX).
    implementation("com.squareup.okhttp3:okhttp:4.12.0")
    // ZXing core: pure-Java QR decoder. Paired with the framework Camera2 API so
    // we can scan the connection QR without pulling in CameraX/ML Kit (AndroidX).
    implementation("com.google.zxing:core:3.5.3")
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
