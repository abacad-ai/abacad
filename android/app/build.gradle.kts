plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
}

android {
    namespace = "dev.abacad.probe"
    compileSdk = 34

    defaultConfig {
        applicationId = "dev.abacad.probe"
        minSdk = 30          // Android 11 — AccessibilityService.takeScreenshot() lives here
        targetSdk = 34
        versionCode = 3
        versionName = "0.3-swipe"
    }

    buildTypes {
        release {
            isMinifyEnabled = false
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
    // OkHttp: outbound WebSocket to the Abacad server. Everything else stays on
    // framework APIs (no AndroidX).
    implementation("com.squareup.okhttp3:okhttp:4.12.0")
    // ZXing core: pure-Java QR decoder. Paired with the framework Camera2 API so
    // we can scan the connection QR without pulling in CameraX/ML Kit (AndroidX).
    implementation("com.google.zxing:core:3.5.3")
}
