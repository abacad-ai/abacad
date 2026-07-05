# Abacad — Android device agent

A normal sideloaded app that turns the phone into something a remote agent can see and
control, from a **single accessibility grant** (no root, no ADB). It exposes three
primitives over an outbound WebSocket to the Abacad server:

| Primitive | Android API |
|---|---|
| `ui_tree` | `getRootInActiveWindow()` — structured tree (text, ids, bounds, clickable) |
| `tap(x,y)` | `dispatchGesture()` — injected tap |
| `swipe` | `dispatchGesture()` — injected drag (scroll/navigation) |
| `screenshot` | `AccessibilityService.takeScreenshot()` — consent-free on Android 11+ |
| `wake` | `WakerActivity` — turn screen on + dismiss a non-secure keyguard |
| `sleep` | device-admin `lockNow()` — turn the screen off between tasks |

`wake`/`sleep` exist so the phone can idle with the **screen off** (battery/lifespan) and only
light up when the agent calls. The catch: a **secure lock (PIN/pattern/biometric) can't be
auto-unlocked** — hands-off use needs a None/Swipe lock. See
[`../docs/power-lockscreen.md`](../docs/power-lockscreen.md) for the full support matrix and the
setup checklist.

All three were verified on real hardware (see the earlier throwaway probe). This is the
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

## Use
1. Start the server: `cd ../server && npm install && npm start` — note the machine's LAN IP.
2. Open **Abacad Probe**, enter `ws://<server-ip>:8848/device`, tap **Save & Connect**.
3. Enable **Abacad Probe** under Accessibility; accept the system warning.
   (`curl http://localhost:8848/health` should now show `deviceConnected:true`.)
4. Register the MCP endpoint with your agent, then drive it:
   ```bash
   claude mcp add --transport http abacad http://localhost:8848/mcp
   ```

Logs: `adb logcat -s ABACAD`.

## Not in v0
Cloud relay / NAT traversal, auth/pairing, approval gating, `type`, tap-by-node-id,
reboot self-heal / OEM battery survival — additive next steps behind the same contract.

On-battery Doze latency (a `wake` during a Doze gap can be delayed) needs a battery-optimization
exemption and/or server-side queue-until-reconnect; see `../docs/power-lockscreen.md`.
