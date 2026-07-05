# Abacad — Android device agent

A normal sideloaded app that turns the phone into something a remote agent can see and
control, from a **single accessibility grant** (no root, no ADB). It exposes three
primitives over an outbound WebSocket to the Abacad server:

| Primitive | Android API |
|---|---|
| `ui_tree` | `getRootInActiveWindow()` — structured tree (text, ids, bounds, clickable) |
| `tap(x,y)` | `dispatchGesture()` — injected tap |
| `screenshot` | `AccessibilityService.takeScreenshot()` — consent-free on Android 11+ |

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
Cloud relay / NAT traversal, auth/pairing, approval gating, `type`/`swipe`, tap-by-node-id,
reboot self-heal / OEM battery survival — additive next steps behind the same contract.
