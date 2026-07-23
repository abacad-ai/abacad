# abacad Linux client

The Linux counterpart to the macOS and Android clients: a **headless daemon** that
dials the abacad relay over a WebSocket and drives this machine on command —
capture the screen and inject mouse/keyboard input. It speaks the same wire
contract as the phone plus the desktop-native verbs.

Unlike the macOS menu-bar app, this is a background process with no GUI: config
comes from flags / env / a config file, so it runs equally on a desktop session
or a headless box (systemd, container, CI).

> **Disclosure.** While this daemon is connected, the machine can be **viewed and
> controlled remotely by an agent** (screen capture + input injection). With no GUI
> there is no on-screen indicator — it logs `device online — this machine can now be
> viewed and controlled remotely by an agent` on connect. Only run it on machines you
> are authorized to operate, and make sure anyone who uses the machine knows it is
> remotely controllable.

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

Builds anywhere with a Go toolchain (this is the one client that also builds on
the Linux CI box):

```sh
# from the repo root:
make linux            # → linux/build/abacad
```

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
