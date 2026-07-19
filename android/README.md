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
