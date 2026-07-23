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

The bundle id is `ai.abacad.mac` (set in `Info.plist` and the root `Makefile`; keep
the two in sync if you ever change it). All targets live in the root `Makefile` and
run from the repo root — there is no Makefile in this directory.

```sh
# from the repo root; ad-hoc signing if no Developer ID cert is in the keychain:
make macos
open macos/build/abacad.app
```

> TCC (Accessibility, Screen Recording) grants are keyed to the signing identity +
> bundle id. With ad-hoc signing (`-`), a rebuild can invalidate the grant and
> re-prompt. The real Developer ID identity keeps the grant stable.

### Distribution build (signed + notarized)

`make macos-release` produces a Gatekeeper-clean `.dmg`: Developer ID Application
signature, hardened runtime, secure timestamp, notarized by Apple, and the
notarization ticket stapled onto both the `.app` and the `.dmg` (so it passes
offline, even after the app is copied out of the image).

```sh
make macos-release SIGN_IDENTITY="Developer ID Application: Beijing Xiaoyuanzhu Technology Co., Ltd. (R3845XW5FZ)"
# → macos/build/abacad.dmg   (signed, notarized, stapled)
```

Team `R3845XW5FZ`. Publishing the result is a separate step: `make stage-macos`
copies the dmg into the local downloads directory under its published name
`abacad-<version>-macos-arm64.dmg` (and `make stage` refreshes `manifest.json`);
in production, copy that file into the deploy directory's `downloads/` (infra
repo, `deployment/xyz-sg-1/abacad.ai/`) and run its `deploy.sh`.

**One-time notary credential setup.** `make macos-release` reads notary credentials
from a keychain profile named `abacad-notary` (override with `NOTARY_PROFILE`).
Create it once with an App Store Connect API key
(App Store Connect → Users and Access → Integrations → Keys):

```sh
xcrun notarytool store-credentials abacad-notary \
  --key AuthKey_XXXXXXXXXX.p8 --key-id XXXXXXXXXX \
  --issuer 35f46605-144b-4c02-bb13-5874363169a8
```

Verify a finished build with:

```sh
spctl -a -vv macos/build/abacad.app                                  # → accepted / Notarized Developer ID
spctl -a -t open --context context:primary-signature -vv macos/build/abacad.dmg
xcrun stapler validate macos/build/abacad.dmg                        # offline ticket check
```

## Grant permissions (one-time, requires a human — cannot be scripted)

On first launch, open the menu-bar panel and click **Grant** for each:

- **Accessibility** — required for the AX tree read *and* all CGEvent input.
- **Screen Recording** — required for `screenshot`.

Both open the relevant System Settings pane; flip the toggle for **abacad**,
then **quit and relaunch** the app so it re-reads its trust status. The panel's
green checkmarks confirm the grants (hit **Refresh** after toggling).

## Connect

The easy path — **`abacad connect`** (device-authorization grant, no copy-paste):

```
/Applications/abacad.app/Contents/MacOS/abacad connect
#   or, against a self-hosted server:
#   …/abacad connect --server https://my.host
```

It prints a URL and a short code; open the URL while signed in, approve, and the
issued credential is stored in your login Keychain. Launch the menu-bar app (or, if
it's already running, reopen the panel) and the dot turns green — it auto-connects
on every launch after that. This is the CLI peer of the Linux/Windows `abacad
connect`. (The `connect` binary is the same executable inside the app bundle;
running it needs no Screen Recording / Accessibility grant — those apply only when
the menu-bar app actually drives the Mac.)

Or provision manually:

1. Provision a macOS device on the server and copy its `wss://…/device?token=…`
   URL (`POST /api/devices {"name":"My Mac","platform":"macos"}`, or the
   dashboard's **macOS** add-device tile).
2. Paste the URL into the panel and click **Connect**. The dot turns green.

Either way, from your MCP client, target this device — desktop verbs (`click`,
`scroll`, `press_keys`, `composite`) now drive the Mac.

## Known limits (v0)

- **No multi-touch** — macOS has no public gesture-injection API, so `composite`
  is single-pointer (paths, modifier-fused clicks, and timing work; pinch/rotate
  do not).
- **US keyboard layout** — `press_keys` maps names/characters on a US layout.
- **Main display only** — capture and coordinates target the primary display.
- **App Sandbox must stay OFF** — cross-app accessibility control is incompatible
  with the sandbox.
