# abacad — Android device agent

A normal sideloaded app that turns the phone into something a remote agent can see and
control, from a **single accessibility grant** (no root, no ADB). It exposes a small set
of human-like primitives over an outbound WebSocket to the abacad server:

| Primitive | Android API |
|---|---|
| `screenshot(include_ui_tree)` | `AccessibilityService.takeScreenshot()` (consent-free on Android 11+) + `getRootInActiveWindow()` for the tree (text, ids, bounds, clickable) in the same call |
| `tap(x,y)` | `dispatchGesture()` — injected tap |
| `long_press(x,y)` | `dispatchGesture()` — injected press-and-hold |
| `swipe` | `dispatchGesture()` — injected drag (scroll/navigation) |
| `input_text(text)` | `ACTION_SET_TEXT` on the focused field |
| `back` / `home` / `recents` | `performGlobalAction()` — nav keys |

Waking a dark or locked screen is **automatic and invisible to the agent**: before any
command runs, the service brings the screen up and dismisses a non-secure keyguard via
`WakerActivity`. Sleeping is left to the phone's own display timeout — the agent never
manages power. The one catch: a **secure lock (PIN/pattern/biometric) can't be
auto-unlocked** — hands-off use needs a None/Swipe lock, and a locked-secure device
returns a clear error instead of a lockscreen. See
[`../docs/power-lockscreen.md`](../docs/power-lockscreen.md) for the full support matrix and the
setup checklist.

The primitives were verified on real hardware (see the earlier throwaway probe). This is the
**device half** of the loop; the agent talks to [`../server`](../server), which relays
commands here.

```
agent ──MCP──▶ server ──WebSocket (this app dials out)──▶ device
```

## Requirements
- **Android 11+ (API 30)** — `takeScreenshot()` doesn't exist below it.
- Server machine and this phone on the **same Wi-Fi** (v0 is LAN + cleartext `ws://`).

## Build & install
```bash
cd android
export ANDROID_HOME=$HOME/Library/Android/sdk   # or just open android/ in Android Studio
./gradlew installDebug
```
Needs a JDK 17+ — Android Studio bundles one:
`export JAVA_HOME="/Applications/Android Studio.app/Contents/jbr/Contents/Home"`.

## Release builds & signing
Android has no unsigned install path. Debug builds are auto-signed with
`~/.android/debug.keystore` and are fine on your own phone, but the APK people download
must be a **release** build — a debug build is `debuggable`, so anyone with ADB access to a
user's phone could attach to a service that reads the screen and injects taps.

```bash
make android-release      # -> app/build/outputs/apk/release/app-release.apk
make stage-android        # copied into the downloads dir as abacad-<version>-android-universal.apk
```

One APK, carrying both supported ABIs (`arm64-v8a` and `x86_64`), which is what
the published "universal" name claims. It hasn't always: the APK was arm64-only
for a while and so refused to install on anything else, emulators included.

**Per-ABI splits were built, measured, and rejected.** v0.4.2 release builds:

| APK | size |
|---|---:|
| `arm64-v8a` only | 2,240,684 B (2.14 MB) |
| `x86_64` only | 2,272,013 B (2.17 MB) |
| universal (shipped) | 2,501,465 B (2.39 MB) |

Splitting saves 260,781 B — 255 KB, ~10% — for the best case, an arm64 phone.
The native library is the only per-ABI content and it's ~224 KB; the rest is
shared dex and resources that every split carries in full anyway. That 255 KB
would have cost three release artifacts, three download buttons the user has to
choose between by knowing their own ABI, a per-ABI `versionCode` offset scheme,
and a 3x longer native CI build. Revisit only if the native side grows by an
order of magnitude.

The size win came from R8 instead — the release build is minified and
resource-shrunk, **18,754,025 B → 2,501,465 B (−87%)**. See
`app/proguard-rules.pro` for the short list of names that must survive
obfuscation, and why that list is short.

The release key is **permanent**: Android refuses to install an update signed by a
different key, so replacing it means every user must uninstall first (losing their
pairing). It therefore lives outside the repo and outside any build tree, at
`~/.abacad/android-release.jks`, with credentials in `~/.gradle/gradle.properties`:

```properties
abacadReleaseStoreFile=~/.abacad/android-release.jks
abacadReleaseStorePassword=...
abacadReleaseKeyAlias=abacad
abacadReleaseKeyPassword=...
```

**Back that keystore up.** To create one on a new machine (or if it's ever lost — which
strands every existing install):

```bash
keytool -genkeypair -keystore ~/.abacad/android-release.jks -storetype PKCS12 \
  -alias abacad -keyalg RSA -keysize 4096 -validity 10950 -dname "CN=abacad, O=abacad, C=US"
```

`assembleRelease` fails with a pointer here if the properties aren't set — it never falls
back to the debug key, because shipping one debug-signed release locks users out of every
properly signed update afterwards. Signatures are v2 + v3 (v3 carries proof-of-rotation,
the only path to ever changing the key without forcing uninstalls).

Sideloading caveats that signing does *not* remove: users still tap through "install from
unknown sources", and Play Protect still shows a scan warning on first install. Only Play
Store distribution clears those.

## Use
1. Start the server: `cd ../server && npm install && npm start` — note the machine's LAN IP.
2. Open **abacad**, enter `ws://<server-ip>:8848/device`, tap **Save & Connect**.
3. Enable **abacad** under Accessibility; accept the system warning.
   (`curl http://localhost:8848/health` should now show `deviceConnected:true`.)
4. Register the MCP endpoint with your agent, then drive it:
   ```bash
   claude mcp add --transport http abacad http://localhost:8848/mcp
   ```

Logs: `adb logcat -s ABACAD`.

## Not in v0
Cloud relay / NAT traversal, auth/pairing, approval gating, tap-by-node-id, `open_app`,
reboot self-heal / OEM battery survival — additive next steps behind the same contract.

On-battery Doze latency (a command during a Doze gap can be delayed, including the auto-wake
it triggers) needs a battery-optimization exemption and/or server-side queue-until-reconnect;
see `../docs/power-lockscreen.md`.
