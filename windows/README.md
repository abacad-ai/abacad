# abacad Windows agent

The desktop counterpart to the macOS and Android apps: a notification-area (tray)
app that dials the abacad relay over a WebSocket and drives this PC on command —
read the UI Automation tree, capture the screen, and inject mouse/keyboard input.
It speaks the same wire contract as the phone plus the desktop-native verbs.

## What it implements

| Lane | Methods |
|------|---------|
| Command (JSON) | `screenshot` (+ UI tree), `input_text`, `tap`→click, `long_press`, `swipe`→drag, `click`, `right_click`, `drag`, `scroll`, `press_keys`, `composite` |
| Tunnel (binary) | `/connect` stream lane — dials arbitrary `host:port` and pipes TCP (RDP, SSH, VNC, …) |

`back` / `home` / `recents` return a clean "no desktop analogue" error (the tool
list is a global superset; the device rejects what it doesn't implement).

Backends: **UI Automation** (`System.Windows.Automation`, tree), **GDI `BitBlt`**
(capture), **`SendInput`** (input), **`System.Net.Sockets`** (tunnel). The process
is PerMonitorV2 DPI-aware, so it works in **physical pixels** — UIA bounds, the
screenshot, and click coordinates all share one space (1 screenshot pixel == 1
click unit, matching the other clients). The screenshot is JPEG; the wire field
stays `png_base64` for compatibility.

## Build (needs the .NET 8 SDK)

`dotnet` cross-builds this Windows-targeted project on any OS, but the app only
runs on Windows 10/11.

```sh
cd windows
dotnet build -c Release
# self-contained single exe (no .NET install needed on the target PC):
dotnet publish -c Release -r win-x64 --self-contained \
  -p:PublishSingleFile=true -o publish
# → publish/Abacad.exe
```

> UI Automation is pulled in via `<UseWPF>true</UseWPF>` (WPF's client assemblies);
> the app draws no WPF UI. If your SDK can't resolve `System.Windows.Automation`,
> confirm the Windows Desktop workload is installed (`dotnet workload` / the
> "Desktop development" component).

## Run

Launch `Abacad.exe`. A relay-mark icon appears in the notification area (its hub
turns **green** when connected). Double-click it — or right-click → **Settings…** —
to open the panel.

Windows needs no per-capability permission grant (unlike macOS TCC): a normal
process can already read the UIA tree, capture the screen, and inject input.

## Connect

The easy path — **`abacad connect`** (device-authorization grant, no copy-paste):

```
abacad connect                       # or: abacad connect --server https://my.host
```

It prints a URL and a short code; open the URL while signed in, approve, and the
issued credential is stored for you. Start abacad (the tray app) and the dot turns
green — it auto-connects on every launch after that. This is the console peer of
the Linux/macOS `abacad connect`.

Or provision manually:

1. Provision a Windows device on the server and copy its `wss://…/device?token=…`
   URL (`POST /api/devices {"name":"My PC","platform":"windows"}`, or the
   dashboard's **Windows** add-device tile).
2. Paste the URL into the tray settings panel and click **Connect**. The dot turns
   green.

Either way, from your MCP client, target this device — desktop verbs (`click`,
`scroll`, `press_keys`, `composite`) now drive the PC.

The URL carries the device token, so it is stored encrypted at rest with **DPAPI**
(only this Windows user account can decrypt it) and sent in the `Authorization`
header, never the URL query.

## Known limits (v0)

- **Elevated windows** — input into windows owned by an elevated (administrator)
  process is blocked by Windows UIPI unless abacad is itself run as administrator.
- **US keyboard layout** — `press_keys` maps names/characters on a US layout.
- **Primary display only** — capture and coordinates target the primary monitor.
- **Single pointer** — `composite` is single-pointer (paths, modifier-fused clicks,
  and timing work; multi-touch gestures do not).
- **No published installer yet** — build the exe yourself (above). A signed
  (Authenticode) installer dropped at `downloads/abacad-windows-latest.exe` — the
  analogue of the macOS notarized `.dmg` — is a follow-up; once it exists, add a
  `Windows` entry to `CLIENT_DOWNLOADS` in `server/frontend/src/lib/devices.ts`.
