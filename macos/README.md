# abacad macOS agent

The desktop counterpart to the Android app: a menu-bar app that dials the abacad
relay over a WebSocket and drives this Mac on command — read the accessibility
tree, capture the screen, and inject mouse/keyboard input. It speaks the same wire
contract as the phone plus the desktop-native verbs.

## What it implements

| Lane | Methods |
|------|---------|
| Command (JSON) | `screenshot` (+ UI tree), `input_text`, `tap`→click, `long_press`, `swipe`→drag, `click`, `right_click`, `drag`, `scroll`, `press_keys`, `composite` |
| Tunnel (binary) | `/connect` stream lane — dials arbitrary `host:port` and pipes TCP (ssh, VNC, …) |

`back` / `home` / `recents` return a clean "no desktop analogue" error (the tool
list is a global superset; the device rejects what it doesn't implement).

Backends: **AXUIElement** (tree), **ScreenCaptureKit** (capture, macOS 14+),
**CGEvent** (input), **Network.framework** (tunnel). Coordinates are global
top-left points; the screenshot is scaled to point size so tree bounds map
directly to click points.

## Build (on a Mac — needs Swift/Xcode; a Linux box cannot build this)

The bundle id is `ai.abacad.mac` (set in `Info.plist` and the `Makefile`; keep the
two in sync if you ever change it). For distribution, set `SIGN_IDENTITY` to your
Developer cert:

```sh
cd macos
# ad-hoc signing (fine for local dev):
make
# or with your Developer identity (more stable TCC grants across rebuilds):
make SIGN_IDENTITY="Apple Development: you@example.com (TEAMID)"
open build/abacad.app
```

> TCC (Accessibility, Screen Recording) grants are keyed to the signing identity +
> bundle id. With ad-hoc signing (`-`), a rebuild can invalidate the grant and
> re-prompt. A real Developer identity keeps the grant stable.

## Grant permissions (one-time, requires a human — cannot be scripted)

On first launch, open the menu-bar panel and click **Grant** for each:

- **Accessibility** — required for the AX tree read *and* all CGEvent input.
- **Screen Recording** — required for `screenshot`.

Both open the relevant System Settings pane; flip the toggle for **abacad**,
then **quit and relaunch** the app so it re-reads its trust status. The panel's
green checkmarks confirm the grants (hit **Refresh** after toggling).

## Connect

1. Provision a macOS device on the server and copy its `wss://…/device?token=…`
   URL. (The server now stores a `platform` tag; provision with
   `POST /api/devices {"name":"My Mac","platform":"macos"}` so `list_devices`
   shows it as a desktop.)
2. Paste the URL into the panel and click **Connect**. The dot turns green.
3. From your MCP client, target this device — desktop verbs (`click`, `scroll`,
   `press_keys`, `composite`) now drive the Mac.

## Known limits (v0)

- **No multi-touch** — macOS has no public gesture-injection API, so `composite`
  is single-pointer (paths, modifier-fused clicks, and timing work; pinch/rotate
  do not).
- **US keyboard layout** — `press_keys` maps names/characters on a US layout.
- **Main display only** — capture and coordinates target the primary display.
- **App Sandbox must stay OFF** — cross-app accessibility control is incompatible
  with the sandbox.
