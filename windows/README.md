# abacad Windows agent

The desktop counterpart to the macOS and Android apps: it dials the abacad relay
over a WebSocket and drives this PC on command — read the UI Automation tree,
capture the screen, and inject mouse/keyboard input. It speaks the same wire
contract as the phone plus the desktop-native verbs.

Two builds of one agent:

| | Project | Ships as | For |
|---|---|---|---|
| **App** | `Abacad.csproj` (WinUI 3) | `abacad-app-<v>-windows-x64.exe` (per-user installer) | a desktop session — tray icon, window, Pause button |
| **CLI** | `cli/Abacad.Cli.csproj` | `abacad-cli-<v>-windows-x64.zip` | Server boxes and remote sessions with nowhere to put a tray icon |

The CLI compiles the same sources minus four files (`Program`, `App.xaml`,
`MainWindow.xaml`, `TrayIcon`, and the two GDI helpers that only draw the tray
glyph), so there is one implementation of the agent, not two. It is a *full*
client — `abacad` with no arguments runs the agent in the foreground — because
Windows, unlike macOS, puts no bundle-identity condition on screen capture or
input injection.

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

The **app** needs a Windows build host: the Windows App SDK (WinUI 3) targets and
the XAML compiler resolve nowhere else. The **CLI** references none of that and
sets `EnableWindowsTargeting`, so it cross-builds from Linux or macOS — which is
why CI builds it on a cloud runner and only the app goes to the self-hosted box.
Both binaries run on Windows 10/11 only.

```sh
# from the repo root:
make windows-release       # app → windows/publish/Abacad.exe   (Windows host only)
make windows-installer     # app → windows/installer/Output/abacad-setup.exe (needs ISCC)
make windows-cli-release   # cli → windows/publish-cli/abacad.exe (any host)
```

`windows-installer` is what release actually publishes; `windows-release` only
produces its payload. It needs [Inno Setup](https://jrsoftware.org/isinfo.php)
6.3+ with `ISCC.exe` on PATH — on the self-hosted runner that means a
machine-scope install readable by `NT AUTHORITY\NETWORK SERVICE`, not a per-user
or winget-portable one.

Both are self-contained single files, so the target PC needs no .NET install — and
the app additionally sets `WindowsAppSDKSelfContained`, so it needs no Windows App
Runtime install either. Those are two separate runtimes: `--self-contained` bundles
only the .NET one, and v0.5.1 shipped without the second, so the app refused to
start with *"This application requires the Windows App Runtime Version 1.6"* on any
PC that didn't already have the framework MSIX.

> UI Automation comes from the `FlaUI.UIA3` package. An earlier revision pulled it
> in via `<UseWPF>true</UseWPF>` (WPF's client assemblies) and this note used to say
> so; the csproj no longer sets it. Nothing here draws WPF UI.

The app icon is `Abacad.ico`, embedded in the exe via `<ApplicationIcon>` and used
by the installer for its own icon and the shortcuts it creates. It is generated and
**committed**, like `macos/AppIcon.icns` — regenerate only if the artwork changes:

```sh
python3 scripts/gen-win-ico.py   # assets/icon-rounded-1024.png → windows/Abacad.ico
```

## Run

Run the downloaded installer. It installs per-user to
`%LOCALAPPDATA%\Programs\abacad` with **no UAC prompt**, adds a Start Menu entry
(desktop shortcut optional), and offers to start abacad when you sign in — ticked
by default, because a device agent that doesn't survive a reboot just goes quietly
unreachable. Uninstall from Settings → Apps.

Then launch **abacad**. A relay-mark icon appears in the notification area (its hub
turns **green** when connected). Double-click it — or right-click → **Settings…** —
to open the panel.

Uninstalling removes `%APPDATA%\abacad`, which holds this PC's device credential —
so a reinstall starts over at the setup screen and needs a fresh claim code. The
credential is what the client reads as "someone set this PC up", and keeping it
would let a reinstall skip the setup gate and go straight to controllable. An
*upgrade* installs over the old version without uninstalling, so it keeps the
credential and reconnects as the same device. Either way, the device row stays on
the relay until you revoke it from the dashboard.

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
- **Single-file publish was leaking native DLLs** — `PublishSingleFile` bundles
  managed assemblies but leaves native ones (`wpfgfx_cor3`, `PresentationNative_cor3`,
  `D3DCompiler_47_cor3`, `PenImc_cor3`, `vcruntime140_cor3`) beside the exe, and
  `make stage-windows` ships only the exe — so those files never reached anyone.
  Both projects now set `IncludeNativeLibrariesForSelfExtract`, verified on the CLI
  (five loose DLLs → none). **The app's fix is still unverified at runtime**: no
  host here can launch a Windows binary. v0.5.1 got as far as the Windows App SDK
  bootstrapper on a real PC before failing for an unrelated reason (see below), so
  single-file extraction is *not* disproven — but nothing past startup is tested.
  The installer removes the *structural* half of this risk: its `[Files]` entry
  globs the whole publish directory, so anything the two self-contained flags do
  not fold into the exe (`resources.pri`, `.winmd`, XAML `.xbf`) now reaches the
  user instead of being dropped by a one-file copy. A bare-exe download could not
  have carried them at all.
- **Windows App Runtime was not bundled (fixed, unverified)** — `--self-contained`
  bundles the .NET runtime only. An unpackaged WinUI 3 app resolves the *Windows
  App Runtime* from a framework MSIX on the machine, so v0.5.1 died at launch with
  "This application requires the Windows App Runtime Version 1.6". The csproj now
  sets `WindowsAppSDKSelfContained`. That combination — WinAppSDK self-contained
  *plus* `PublishSingleFile` — is the one to actually launch on the NUC before the
  next release is announced; it is the same untested assumption that produced this
  bug in the first place.
- **Unsigned download** — release publishes `abacad-app-<version>-windows-x64.exe`
  (a per-user Inno installer) and the downloads page lists it automatically from
  the manifest, but it is **not** Authenticode-signed yet, so SmartScreen warns on
  download and shows an unknown publisher on first run. The installer half of the
  gap is closed; the certificate is not. Hook: the commented `SignTool=` directive
  in `windows/installer/abacad.iss`. Sign the *installer*, not the payload exe —
  SmartScreen reputation then accrues to the certificate and carries across
  releases, instead of resetting with every new file hash. A non-EV certificate
  still needs download volume before the warning fades; only EV skips that wait.
- **No auto-update** — Inno installs and uninstalls; it does not update. Upgrading
  means downloading the new installer. A real update channel would mean a different
  framework (Velopack/Squirrel), not a change here.
- **Upgrading over a *running* app is the untested path** — `CloseApplications=yes`
  lets Restart Manager close the tray app before overwriting it. Detection is
  reliable (the running process locks its own exe), but closing is not: Inno calls
  `RmShutdown` *without* force, which politely asks top-level windows to close, and
  `MainWindow.xaml.cs` cancels its `Closing` event so the window hides to tray
  instead of exiting. If RM can't close it, the user gets an Abort/Retry/Ignore
  prompt and "Ignore" then hits a sharing violation. The fix is a named mutex in
  the app plus an `AppMutex` directive in the `.iss` — worth doing, deliberately
  not done blind, since it touches the startup path that already broke twice.
  **Exit abacad from the tray before upgrading** until that lands.
- **The bundled VNC path cannot work as written** — `VncPipe.cs` resolves
  `AppContext.BaseDirectory\vnc\winvnc.exe`, but nothing in the repo packages a
  `vnc/` folder, and under `PublishSingleFile` `BaseDirectory` is the *temp
  extraction directory*, not the install directory — so a `vnc\` laid down beside
  the installed exe would not be found either. The installer does not fix this and
  does not pretend to; set `ABACAD_VNC_SERVER` or fix the resolution to use the
  exe's real directory.
