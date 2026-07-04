# Abacad — Android capability probe

A **throwaway** app that verifies the three capabilities Abacad's Android wedge
depends on, all from a **single Accessibility grant** (no root, no ADB privilege,
no MediaProjection):

| Test | API | Pass signal (Logcat tag `ABACAD_PROBE`) |
|------|-----|------------------------------------------|
| UI tree | `getRootInActiveWindow()` | `TREE: … nodes=N withText=… clickable=…` with real numbers |
| Tap inject | `dispatchGesture()` | `TAP: onCompleted` |
| Screenshot | `AccessibilityService.takeScreenshot()` | `SHOT: SUCCESS WxH nonBlack=true` **and no consent dialog ever shown** |

This is **not** the product. No relay, no MCP, no persistence. Delete after verifying.

## Requirements

- An **Android 11+ (API 30)** device. `takeScreenshot()` does not exist below API 30.
- No Google Play, no signing setup — a debug APK sideloads fine.

## Build

```bash
# from android/
./gradlew assembleDebug
# output: app/build/outputs/apk/debug/app-debug.apk
```

Needs JDK 17 and an Android SDK (`platform android-34`, `build-tools 34.0.0`).
Point Gradle at the SDK via `local.properties` (`sdk.dir=/path/to/sdk`) or the
`ANDROID_HOME` env var.

## Run & verify

1. Install: `adb install -r app-debug.apk` (or copy the APK to the phone and tap it).
2. Open **Abacad Probe** → tap **Open Accessibility Settings** → enable **Abacad Probe**.
3. Accept the system warning. The probe runs immediately and re-runs on each
   screen change.
4. Watch results:
   ```bash
   adb logcat -s ABACAD_PROBE
   ```
5. Re-run on demand (e.g. after switching to a different app):
   ```bash
   adb shell am broadcast -a dev.abacad.probe.RUN
   ```
6. Inspect the captured frame:
   ```bash
   adb pull /sdcard/Android/data/dev.abacad.probe/files/probe_shot.png
   ```

### Verdict

**PASS** if you see, from only the accessibility toggle:
`TREE` with real node counts, `SHOT: SUCCESS … nonBlack=true`, `TAP: onCompleted`
— **and no screen-capture consent dialog appeared at any point.**
