# abacad Linux client

The Linux counterpart to the macOS and Android clients: it dials the abacad relay
over a WebSocket and drives this machine on command — capture the screen and
inject mouse/keyboard input. It speaks the same wire contract as the phone plus
the desktop-native verbs.

One tree, built two ways:

| | Build | Ships as | For |
|---|---|---|---|
| **CLI** | `make linux` (cgo-free) | `abacad-cli-<v>-linux-<arch>.tar.gz`, via `install.sh` | servers, containers, CI, anything you reach over ssh |
| **App** | `make linux-deb` (cgo + GTK4) | `abacad-app-<v>-linux-amd64.deb` | a desktop session — window, live status, Pause button, app launcher |

The `.deb` carries the GTK build plus a systemd **user** service, so the same
binary runs the window (`abacad --gui`) and the background connection. Config
comes from flags / env / a config file either way.

> **Disclosure.** While this client is connected, the machine can be **viewed and
> controlled remotely by an agent** (screen capture + input injection). The app
> shows that live in its window. The CLI has no on-screen indicator — it logs
> `device online — this machine can now be viewed and controlled remotely by an
> agent` on connect, and `abacad capabilities` is its control surface. Only run it
> on machines you are authorized to operate, and make sure anyone who uses the
> machine knows it is remotely controllable.

## What it implements

| Lane | Methods |
|------|---------|
| Command (JSON) | `screenshot` (+ UI tree), `input_text`, `tap`→click, `long_press`, `swipe`→drag, `click`, `right_click`, `drag`, `scroll`, `press_keys`, `composite` |
| Tunnel (binary) | `/connect` stream lane — dials arbitrary `host:port` and pipes TCP (ssh, VNC, …) |

`back` / `home` / `recents` return a clean "no desktop analogue" error (the tool
list is a global superset; the device rejects what it doesn't implement).

Backends: **XGB** (`GetImage` screen capture) and **XTEST** (`FakeInput` mouse /
keyboard) over a pure-Go X11 connection — no cgo, no libX11 dev headers, no
`xdotool`. Coordinates are root-window pixels; the screenshot is captured in that
same space so a pixel maps straight to a click point.

## Build

The CLI builds anywhere with a Go toolchain (this is the one client that also
builds on the Linux CI box). The app additionally needs the GTK4 and libadwaita
dev packages, and links cgo against them — so it builds on Linux only, for the
host's own architecture:

```sh
# from the repo root:
make linux            # CLI          → linux/build/abacad
make linux-gui        # app binary   → linux/build/abacad-gui
make linux-deb        # app package  → linux/build/abacad_<version>_amd64.deb

# on Debian/Ubuntu, once:
sudo apt install libgtk-4-dev libadwaita-1-dev   # libadwaita >= 1.4
```

The gotk4 bindings are pinned in `go.mod` like any other dependency; `make -C
linux deps-gui` bumps them.

## Provision + connect

1. Provision a Linux device on the server and copy its `wss://…/device?token=…`
   URL. Set the platform tag so `list_devices` shows it as a desktop:
   `POST /api/devices {"name":"My Linux box","platform":"linux"}`.
2. Run the daemon (config precedence: flags > env > `~/.config/abacad/config`):

   ```sh
   # flag:
   linux/build/abacad --server-url 'wss://host/device?token=…'
   # or split the token out of the URL:
   ABACAD_SERVER_URL=wss://host/device ABACAD_TOKEN=… linux/build/abacad
   # or ~/.config/abacad/config:
   #   server_url = wss://host/device
   #   token      = …
   ```

   The token is carried in the `Authorization: Bearer` header, never in the URL
   it dials, so it stays out of logs. Plaintext `ws://` is refused to anything but
   loopback — a cleartext control channel to this host would be a full MITM.
3. From your MCP client, target this device — the desktop verbs now drive it.

## Verify (headless)

```sh
make linux-test       # unit tests + Xvfb end-to-end suite
```

The E2E suite (`internal/e2e`, build tag `e2e`) spins up a virtual X server and a
mock `/device` relay, drives every verb, confirms the screenshot is a real JPEG
of the framebuffer, that injected motion actually warps the pointer, and that the
binary tunnel round-trips. Skips automatically if `Xvfb` isn't on `PATH`.

## Known limits (v1)

- **X11 only.** Wayland capture/input (portals, `uinput`) is out of scope for v1;
  run under an X session (or XWayland).
- **Accessibility tree is stubbed.** `screenshot` returns `{pkg:"", nodes:[]}` for
  now — the pixel rung works fully; the AT-SPI (D-Bus) semantic tree is the next
  build. See `TODO(atspi)` in `internal/agent/tree.go`.
- **`input_text` types, it doesn't replace.** Without AT-SPI there's no reliable
  focused-field API, so `input_text` types into the focused element (click the
  field first). Replace semantics arrive with the tree.
- **US keyboard layout** for named keys / characters, matching the other clients.
  Arbitrary Unicode still types via a temporarily remapped spare keycode.
- **Primary screen only** for capture and coordinates.
