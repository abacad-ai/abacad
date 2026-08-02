; abacad — per-user Windows installer (Inno Setup 6.3+).
;
; Built by `make windows-installer`, which passes the repo-root VERSION in on the
; ISCC command line. Do not hardcode a version here: every other client stamps
; itself from that one file and this must not be the exception that drifts.
;
;   iscc /DAppVersion=0.5.2 windows\installer\abacad.iss
;
; PER-USER, DELIBERATELY. PrivilegesRequired=lowest makes {autopf} resolve to
; %LOCALAPPDATA%\Programs, so this installs with no UAC prompt at all. That is
; the modern default (VS Code, Git for Windows) and it is also the only shape
; consistent with the app itself: app.manifest requests asInvoker, and the
; device credential in %APPDATA%\abacad is per-user via DPAPI CurrentUser. An
; elevated install would put program files somewhere the app's own user may not
; be able to update.

#define AppName "abacad"
#define AppExe "Abacad.exe"
#define AppPublisher "abacad"
#define AppURL "https://abacad.ai"

; The Makefile always passes this. The fallback exists so a developer poking at
; the script by hand gets an obvious placeholder rather than a silent "1.0". It
; must stay parseable as x.y.z — VersionInfoVersion below rejects "0.0.0-dev".
#ifndef AppVersion
  #define AppVersion "0.0.0"
#endif

; ArchitecturesAllowed=x64compatible below needs Inno Setup 6.3+. The assertion
; lives here rather than in the Makefile because there is no way to ask ISCC its
; version from outside: --version is not an option (it prints a banner and exits
; 1), the banner names the major version only, and ISCC.exe carries no PE version
; resource. ISPP's VER does know, and the preprocessor runs before any file is
; packaged — so an old compiler fails in milliseconds with this message instead
; of a confusing parse error pointing at the directive itself.
#if VER < EncodeVer(6,3,0)
  #error abacad.iss needs Inno Setup 6.3+ for ArchitecturesAllowed=x64compatible
#endif

[Setup]
; Fixed forever — this is what makes an install an UPGRADE rather than a second
; parallel entry in Add/Remove Programs. Derived reproducibly from the Win32
; identity in windows/app.manifest so it is not a magic number:
;   python3 -c "import uuid; print(uuid.uuid5(uuid.NAMESPACE_DNS, 'ai.abacad.win'))"
AppId={{383C79F3-E2D9-56BF-A1D3-C65130DD4794}
AppName={#AppName}
AppVersion={#AppVersion}
; Add/Remove Programs shows a stable "abacad" with the version in its own column,
; rather than rewriting the display name on every upgrade. (AppVerName would do
; the latter, and is redundant anyway once AppVersion is set.)
UninstallDisplayName={#AppName}
; Not derived from AppVersion — Inno maps AppVersion to the *text* product-version
; field only, so without this the setup binary ships a 0.0.0.0 file version.
VersionInfoVersion={#AppVersion}
AppPublisher={#AppPublisher}
AppPublisherURL={#AppURL}
AppSupportURL={#AppURL}
AppUpdatesURL={#AppURL}/downloads

; {autopf} resolves to {userpf} — %LOCALAPPDATA%\Programs — because
; PrivilegesRequired=lowest forces non-administrative mode. No DefaultGroupName:
; [Icons] writes to {autoprograms} directly, since a Start Menu folder holding one
; shortcut is a folder nobody wants.
DefaultDirName={autopf}\{#AppName}
DisableProgramGroupPage=yes

PrivilegesRequired=lowest
; x64compatible, not x64: the payload is win-x64 but runs fine under Arm64
; Windows 11's x64 emulation, and plain `x64` would lock those users out. This is
; the directive that requires Inno 6.3+, asserted by the #if VER check at the top
; of this file so an old compiler fails with a named error, not mid-compile.
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
MinVersion=10.0.17763

OutputDir=Output
OutputBaseFilename=abacad-setup
SetupIconFile=..\Abacad.ico
UninstallDisplayIcon={app}\{#AppExe}
WizardStyle=modern

; lzma2/max is already the default; stated explicitly because it is load-bearing
; for a ~244 MB payload and should not be quietly weakened. SolidCompression pays
; off across the many files WindowsAppSDKSelfContained emits (see [Files]).
Compression=lzma2/max
SolidCompression=yes

; abacad is a tray app that is *expected* to be running when someone installs an
; upgrade over it; without this the exe is locked and the install fails.
;
; Restart Manager DETECTS it fine — a running process holds a handle to its own
; image. CLOSING it is the weaker half: Inno calls RmShutdown without force, which
; asks top-level windows to close, and MainWindow.xaml.cs cancels its Closing
; event to hide-to-tray instead. If RM cannot close it the user gets an
; Abort/Retry/Ignore prompt. The durable fix is a named mutex in the app plus an
; AppMutex directive here; the app has no mutex today, so this is a known rough
; edge on upgrade, recorded in windows/README.md.
;
; RestartApplications=no is defensive only — RM can restart just those apps that
; call RegisterApplicationRestart, which this one does not.
CloseApplications=yes
RestartApplications=no

; Signing hook. Nothing is Authenticode-signed yet — see the known-limits note in
; windows/README.md. When a certificate exists, define the "abacad" SignTool in
; the ISCC invocation and uncomment these; signing the installer is what builds
; SmartScreen reputation on the certificate rather than on a per-release hash.
; SignTool=abacad
; SignedUninstaller=yes

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
; Unchecked, matching Inno's own convention — the Start Menu entry above always
; exists, so the app is discoverable without putting anything on the desktop.
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"; Flags: unchecked

; Checked by default, unlike the Linux systemd unit (which ships disabled and
; wants an explicit `systemctl --user enable`). The asymmetry is intentional: a
; device agent that does not survive a reboot silently stops being reachable,
; which reads as a broken relay rather than an unstarted service. A ticked box
; in a wizard is visible and refusable in a way a disabled unit file is not.
Name: "autostart"; Description: "Start {#AppName} when I sign in"

[Files]
; The whole publish directory, NOT just Abacad.exe.
;
; PublishSingleFile plus IncludeNativeLibrariesForSelfExtract folds managed and
; native code into the exe, but WindowsAppSDKSelfContained also emits *content* —
; resources.pri, Microsoft.UI.Xaml .xbf/.pri, .winmd — which neither flag bundles.
; `make stage-windows` used to ship exactly one file, so anything else publish
; produced was silently never delivered; that is the shape of the v0.5.1 bug and
; it is invisible until launch on a clean PC. An installer can carry a directory,
; so let it: this glob is correct whether publish emits one file or forty.
;
; .pdb is excluded — debug symbols for a release build are dead weight in a
; download this size.
Source: "..\publish\*"; DestDir: "{app}"; Excludes: "*.pdb"; Flags: ignoreversion recursesubdirs createallsubdirs

[Icons]
Name: "{autoprograms}\{#AppName}"; Filename: "{app}\{#AppExe}"
Name: "{autodesktop}\{#AppName}"; Filename: "{app}\{#AppExe}"; Tasks: desktopicon

[Registry]
; HKCU is the only correct hive for a non-elevated per-user install. The second
; line is not redundant: without it, unticking autostart during an UPGRADE would
; leave the previous install's Run value behind and the app would keep starting.
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Run"; ValueType: string; ValueName: "{#AppName}"; ValueData: """{app}\{#AppExe}"""; Flags: uninsdeletevalue; Tasks: autostart
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Run"; ValueType: none; ValueName: "{#AppName}"; Flags: deletevalue; Tasks: not autostart

[Run]
Filename: "{app}\{#AppExe}"; Description: "{cm:LaunchProgram,{#AppName}}"; Flags: nowait postinstall skipifsilent

; No [UninstallDelete] for %APPDATA%\abacad. It holds the DPAPI-encrypted
; device_id/device_token (windows/Prefs.cs), so leaving it means a reinstall
; reconnects as the same device instead of demanding a fresh claim code. The
; CurrentUser DPAPI scope already makes it unreadable to any other account, so
; there is nothing gained by shredding it on the way out. Revoke a device from
; the dashboard — that is the control that actually ends access.
