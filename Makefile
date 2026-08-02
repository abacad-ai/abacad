# abacad dev tasks. Every target you invoke lives here. The one exception is the
# Linux GUI (linux/Makefile), whose recipes need a toolchain the other targets
# deliberately avoid — `linux-gui` / `linux-deb` below are passthroughs to it.

# ── Version ───────────────────────────────────────────────────────────────────
# One number for the whole monorepo (server + every client). The VERSION file at
# the repo root is the single source of truth; builds stamp it in (Go via ldflags,
# macOS via an Info.plist substitution, Android/Windows/frontend read the file
# directly). `make bump-version V=x.y.z` moves everything at once — even components
# with no code change, on purpose (a release bumps the whole repo together).
VERSION            := $(shell cat VERSION)
GO_SERVER_LDFLAGS  := -X abacad/internal/version.Version=$(VERSION)
GO_LINUX_LDFLAGS   := -X abacad-linux/internal/version.Version=$(VERSION)

# Port the dashboard (Vite dev server) is served on — the URL you open in a browser.
PORT ?= 1419

# The Go backend listens here. The Vite dev proxy targets this port, so keep it
# in sync with server/frontend/vite.config.ts if you change it.
BACKEND_ADDR ?= :1213

# Where the server looks for public release artifacts. The backend runs with
# server/backend as its working directory and defaults to a relative
# "downloads" (ABACAD_DOWNLOADS overrides it), so this is that directory.
DOWNLOADS ?= server/backend/downloads

# ── macOS signing & notarization ─────────────────────────────────────────────
#   SIGN_IDENTITY  : codesign identity. Auto-detected: the team's Developer ID
#                    cert if it is in the keychain, else ad-hoc "-".
#                    Prefer the real identity even for local dev — ad-hoc's
#                    designated requirement is a bare cdhash, so every rebuild
#                    looks like a brand-new app to TCC and the Accessibility /
#                    Screen Recording grants silently stop applying (the old row
#                    lingers in System Settings, still checked, pointing at the
#                    dead hash). Developer ID's requirement is identifier + team,
#                    which survives rebuilds, so you grant once and never again.
#   BUNDLE_ID      : reverse-DNS id; must match CFBundleIdentifier in Info.plist.
#   TEAM_ID        : Apple Developer team id (R3845XW5FZ).
#   NOTARY_PROFILE : name of a keychain profile holding notary credentials,
#                    created once with (App Store Connect API key — recommended):
#                      xcrun notarytool store-credentials abacad-notary \
#                        --key AuthKey_XXXX.p8 --key-id <KEY_ID> --issuer <ISSUER_UUID>
#                    or with an Apple ID + app-specific password:
#                      xcrun notarytool store-credentials abacad-notary \
#                        --apple-id you@example.com --team-id R3845XW5FZ \
#                        --password <app-specific-password>
# ─────────────────────────────────────────────────────────────────────────────
BUNDLE_ID      ?= ai.abacad.mac
TEAM_ID        ?= R3845XW5FZ
NOTARY_PROFILE ?= abacad-notary

# Which keychain holds that profile. notarytool reads the DEFAULT (login)
# keychain unless told otherwise, but on the CI Mac the profile lives in
# ci.keychain alongside the Developer ID identity — and that box has no desktop
# session, so the login keychain is locked and notarytool dies with
# `keychainLocked(keychainName: "default")` even though signing just succeeded.
# Point it at ci.keychain when that keychain exists; on a dev Mac that has no
# ci.keychain the flag is omitted and the default keychain is used as before.
CI_KEYCHAIN := $(HOME)/Library/Keychains/ci.keychain-db
ifneq ($(wildcard $(CI_KEYCHAIN)),)
NOTARY_KEYCHAIN := --keychain "$(CI_KEYCHAIN)"
else
NOTARY_KEYCHAIN :=
endif

DEV_ID := Developer ID Application: Beijing Xiaoyuanzhu Technology Co., Ltd. ($(TEAM_ID))
ifeq ($(shell security find-identity -v -p codesigning 2>/dev/null | grep -c "$(TEAM_ID)"),0)
SIGN_IDENTITY ?= -
else
SIGN_IDENTITY ?= $(DEV_ID)
endif

# Paths are relative to this file (the repo root) — recipes never cd into macos/.
MAC_CONF    := release
MAC_BINARY  := macos/.build/$(MAC_CONF)/abacad
MAC_APP     := macos/build/abacad.app
MAC_DMG     := macos/build/abacad.dmg
MAC_VOLNAME := abacad
MAC_ICNS    := macos/AppIcon.icns
MAC_ICONSET := macos/build/AppIcon.iconset

# A real Developer ID identity (anything but ad-hoc "-") gets the hardened
# runtime + a secure timestamp — both are prerequisites for notarization.
ifeq ($(SIGN_IDENTITY),-)
CODESIGN_FLAGS :=
else
CODESIGN_FLAGS := --options runtime --timestamp
endif

# The .deb's architecture is whatever this host is, because the GUI links cgo
# against the host's own GTK and so never cross-compiles. Releases build on an
# amd64 runner and publish amd64; an arm64 Linux dev box gets a correctly labelled
# arm64 package instead of a mislabelled one. Non-Debian hosts have no dpkg, and
# there the .deb simply isn't built — the fallback only keeps the name well-formed.
DEB_ARCH := $(shell dpkg --print-architecture 2>/dev/null || echo amd64)

# ── Published artifact names ──────────────────────────────────────────────────
# One convention for every platform:
#
#     abacad-<kind>-<version>-<platform>-<arch>.<suffix>
#
# `kind` is app or cli, and it is part of the name because most platforms ship
# both: the app is the one with a window (menu bar, tray, GTK, launcher), the cli
# is the one you drive from a terminal. It is also load-bearing for the manifest —
# gen-manifest.mjs keeps the newest build per kind+platform+arch, so without the
# kind a .deb and a .tar.gz for the same linux-amd64 would collide and one would
# silently vanish from the downloads page.
#
# The `stage` target below copies each build to these names in $(DOWNLOADS) and
# regenerates manifest.json; nothing carries a "latest" name anymore — the
# manifest is what points at the current version.
#
# Arch is whatever that platform's own users call it, not one repo-wide spelling:
# Microsoft's downloads say x64, Apple says Apple silicon, and Debian's package
# architecture really is amd64 (it is the value in the .deb's own control file).
# Someone downloading a build reads the name to answer "is this the one for my
# machine", and a Go-flavoured `amd64` makes a Windows user guess. Matching
# sibling filenames only helps whoever greps the downloads dir, which is nobody.
# The Android APK carries every ABI the app supports (arm64-v8a + x86_64 — see
# android/app/build.gradle.kts), so it's "universal".
PKG_MACOS           := abacad-app-$(VERSION)-macos-apple-silicon.dmg
PKG_MACOS_CLI       := abacad-cli-$(VERSION)-macos-apple-silicon.tar.gz
PKG_ANDROID         := abacad-app-$(VERSION)-android-universal.apk
PKG_WINDOWS         := abacad-app-$(VERSION)-windows-x64.exe
PKG_WINDOWS_CLI     := abacad-cli-$(VERSION)-windows-x64.zip
PKG_LINUX_DEB       := abacad-app-$(VERSION)-linux-$(DEB_ARCH).deb
PKG_LINUX_CLI_AMD64 := abacad-cli-$(VERSION)-linux-amd64.tar.gz
PKG_LINUX_CLI_ARM64 := abacad-cli-$(VERSION)-linux-arm64.tar.gz

# Where each platform's release build leaves its artifact (staged from here).
# dpkg-deb names the package itself (abacad_<version>_<arch>.deb), so the .deb is
# renamed to the repo convention on the way into $(DOWNLOADS) like everything else.
APK_RELEASE := android/app/build/outputs/apk/release/app-release.apk
WIN_EXE     := windows/publish/Abacad.exe
WIN_CLI_EXE := windows/publish-cli/abacad.exe
# WIN_EXE is the installer's INPUT, not a published artifact — what ships as
# PKG_WINDOWS is WIN_SETUP, the same way Linux ships the .deb rather than the
# bare binary. Inno fixes the leaf name via OutputBaseFilename in the .iss, and
# stage-windows renames it to the repo convention on the way into $(DOWNLOADS).
WIN_SETUP   := windows/installer/Output/abacad-setup.exe
LINUX_DEB   := linux/build/abacad_$(VERSION)_$(DEB_ARCH).deb
MAC_CLI_BIN := macos/build/abacad-cli/abacad

.PHONY: build build-debug build-release debug release \
        dev server tokens bump-version version android android-release \
        linux linux-release linux-run linux-test linux-gui linux-deb linux-app \
        macos macos-icon macos-dmg macos-release macos-cli macos-cli-release macos-trust-reset macos-clean \
        windows windows-debug windows-release windows-cli-release windows-installer \
        publish stage stage-macos stage-android stage-linux stage-windows manifest \
        _mac-pkg-dmg _mac-notarize-app _mac-notarize-dmg

# Build every client platform, both variants. Run this on macOS, the only host
# that can build the macOS client (the others build there with their own
# toolchains). A trailing word selects one variant:
#
#   make build          → debug + release for all four platforms
#   make build debug    → debug/dev builds only (fast, local; macOS ad-hoc signed)
#   make build release  → publishable builds only (signed; macOS notarized dmg)
#
# A platform that fails to build is skipped, not fatal: the build moves on to the
# rest, and the release variant still stages whatever did build. So one broken
# toolchain — e.g. Windows/WinUI 3, which can't cross-build from macOS — doesn't
# block shipping the others. (The per-platform CI jobs in release.yml invoke the
# platform targets directly and still fail hard on their own, so a genuine
# release regression is never masked.)
#
# `debug`/`release` after `build` are parsed as the variant here, not run as
# separate goals. The release half needs the macOS signing/notary setup (see
# macos-release) and emits an UNSIGNED Windows .exe — there's no Authenticode
# cert path yet.
_BUILD_MODE := $(filter debug release,$(MAKECMDGOALS))

build:
ifeq ($(_BUILD_MODE),)
	@$(MAKE) build-debug
	@$(MAKE) build-release
else
	@$(MAKE) build-$(_BUILD_MODE)
endif

# No-op stubs so `make build debug` / `make build release` don't fail on an
# unknown goal; `build` above consumes the variant word.
debug release:
	@:

# Platforms per variant. These are looped over inside the recipe (not declared as
# prerequisites) on purpose: a prerequisite that fails aborts the whole build,
# whereas a loop lets a failed platform be skipped so the rest still build. The
# run exits 0 even when a platform is skipped — a partial local build is a success
# here, not an error — but prints a loud SKIPPED line so it's never silent.
# Both kinds per platform, since a release publishes both: linux-deb and the two
# CLIs sit alongside the app targets rather than under them, so a failure to build
# (say) the GTK app still ships the Linux CLI.
DEBUG_PLATFORMS   := android linux macos windows-debug
RELEASE_PLATFORMS := android-release linux-release linux-deb macos-release macos-cli-release \
                     windows-installer windows-cli-release

build-debug:
	@failed=""; \
	for t in $(DEBUG_PLATFORMS); do \
	  echo "==== build $$t ===="; \
	  $(MAKE) $$t || { failed="$$failed $$t"; echo "  (skipped $$t — build failed)"; }; \
	done; \
	if [ -n "$$failed" ]; then echo "Built debug clients (v$(VERSION)) — SKIPPED failed:$$failed"; \
	else echo "Built debug clients: Android, Linux, macOS, Windows (v$(VERSION))"; fi

build-release:
	@failed=""; \
	for t in $(RELEASE_PLATFORMS); do \
	  echo "==== build $$t ===="; \
	  $(MAKE) $$t || { failed="$$failed $$t"; echo "  (skipped $$t — build failed)"; }; \
	done; \
	$(MAKE) stage; \
	if [ -n "$$failed" ]; then echo "Built + staged release clients (v$(VERSION)) — SKIPPED failed:$$failed"; \
	else echo "Built + staged release clients: Android, Linux, macOS, Windows (v$(VERSION))"; fi

# Build the deployable server binary: dashboard SPA + docs site, embedded into
# the Go backend. The embedded copies (server/backend/internal/web/{dist,docs-dist})
# are generated here, not committed — only a .gitkeep stub is tracked in each, so
# the Go package still compiles on a fresh clone (go:embed needs the directory to
# exist at compile time). Until you run this, the backend serves a placeholder
# page for the dashboard and 404s /docs; the API and device channels work either way.
# Needs node + go. Output: server/backend/abacad
server:
	./server/build.sh

# Start the Go backend and the Vite frontend together in the foreground.
# Open http://localhost:$(PORT). Ctrl-C stops both. No embedded build is needed:
# the dashboard is served by Vite, and the backend's placeholder page is never hit.
dev:
	@cd server/frontend && npm install
	@trap 'kill 0' INT TERM EXIT; \
	  ( cd server/backend && ABACAD_ADDR=$(BACKEND_ADDR) go run -ldflags "$(GO_SERVER_LDFLAGS)" ./cmd/abacad -dev-cors ) & \
	  ( cd server/frontend && npm run dev -- --port $(PORT) ) & \
	  wait

# Regenerate the per-platform design tokens (tokens.css / Theme.kt / Theme.swift)
# from design/tokens.json. Commit the outputs together with the JSON change.
tokens:
	node design/generate.mjs

# Move the whole monorepo to a new version. Writes the VERSION file (the single
# source Go/macOS/Android/Windows/frontend builds all read at build time) and syncs
# the spots that can't read it then — the npm package.json + package-lock.json
# versions. Everything else picks the number up on its next build. Only the root
# "version" is touched (first line in each package.json; first two in each lock —
# the package + packages[""] entries), so dependency versions are left alone; a
# lock left stale would make `npm ci` reject the tree.
#
#   make bump-version V=0.5.0
#
# Then rebuild the clients/server to stamp it in, and commit VERSION + the json/lock.
#
# Every file the bump rewrites, grouped by how it has to be rewritten. `version`
# stages exactly VERSIONED_FILES, so the two targets can no longer drift — they
# did once, and v0.5.1 was tagged with a stale AbacadVersion because the Swift
# file was added to the bump but not to the release commit's `git add` list.
VERSION_PKG_JSON := server/package.json server/frontend/package.json
VERSION_PKG_LOCK := server/package-lock.json server/frontend/package-lock.json
VERSION_SWIFT    := macos/Sources/AbacadKit/Version.swift
VERSIONED_FILES  := VERSION $(VERSION_PKG_JSON) $(VERSION_PKG_LOCK) $(VERSION_SWIFT)

bump-version:
	@test -n "$(V)" || { echo "usage: make bump-version V=x.y.z" >&2; exit 1; }
	@printf '%s\n' "$(V)" > VERSION
	@for f in $(VERSION_PKG_JSON); do \
	  awk -v v="$(V)" 'BEGIN{d=0} /"version":/ && !d {sub(/"version":[ \t]*"[^"]*"/, "\"version\": \"" v "\""); d=1} {print}' "$$f" > "$$f.tmp" && mv "$$f.tmp" "$$f"; \
	done
	@for f in $(VERSION_PKG_LOCK); do \
	  [ -f "$$f" ] || continue; \
	  awk -v v="$(V)" 'BEGIN{d=0} /"version":/ && d<2 {sub(/"version":[ \t]*"[^"]*"/, "\"version\": \"" v "\""); d++} {print}' "$$f" > "$$f.tmp" && mv "$$f.tmp" "$$f"; \
	done
	@# The macOS CLI is the other spot that can't read VERSION at build time:
	@# SwiftPM has no build-time substitution and a bare binary has no Info.plist,
	@# so the number is committed in AbacadVersion (see macos/.../Version.swift).
	@f=$(VERSION_SWIFT); \
	  awk -v v="$(V)" '/public static let current =/ {sub(/"[^"]*"/, "\"" v "\"")} {print}' "$$f" > "$$f.tmp" && mv "$$f.tmp" "$$f"
	@echo "Bumped abacad to $(V) (VERSION + package.json + package-lock.json + AbacadVersion). Rebuild to stamp clients/server."

# Cut a release. Prints the current version, proposes a patch bump x.y.(z+1),
# and lets you accept it with Enter or type a different one. Then it bumps the
# version (via bump-version), commits, tags v<version>, and pushes both — which
# fires .github/workflows/release.yml to build, sign, notarize, and publish
# every client to a GitHub Release. Run from an up-to-date main.
version:
	@cur=$$(cat VERSION); \
	major=$${cur%%.*}; rest=$${cur#*.}; minor=$${rest%%.*}; patch=$${rest##*.}; \
	def="$$major.$$minor.$$((patch + 1))"; \
	printf 'Current version: %s\n' "$$cur"; \
	printf 'New version [%s]: ' "$$def"; \
	read v; v=$${v:-$$def}; \
	case "$$v" in [0-9]*.[0-9]*.[0-9]*) ;; *) echo "error: not an x.y.z version: $$v" >&2; exit 1;; esac; \
	if git rev-parse -q --verify "refs/tags/v$$v" >/dev/null 2>&1; then echo "error: tag v$$v already exists" >&2; exit 1; fi; \
	"$${MAKE:-make}" --no-print-directory bump-version V="$$v" && \
	{ for f in $(VERSIONED_FILES); do [ -f "$$f" ] && git add "$$f"; done; true; } && \
	git commit -m "release v$$v" && \
	git tag "v$$v" && \
	git push origin HEAD && \
	git push origin "v$$v" && \
	echo "Pushed v$$v — release workflow: https://github.com/abacad-ai/abacad/actions/workflows/release.yml"

# ── Android ──────────────────────────────────────────────────────────────────

# Build the debug APK — what you install on your own phone while developing.
# Output: android/app/build/outputs/apk/debug/app-debug.apk
android:
	cd android && ./gradlew assembleDebug

# Build the signed release APK — what other people download. Needs the release
# keystore configured in ~/.gradle/gradle.properties (see android/README.md);
# the build fails loudly rather than emitting an unsigned or debug-signed APK.
# Output: android/app/build/outputs/apk/release/app-release.apk
android-release:
	cd android && ./gradlew assembleRelease

# ── Linux ────────────────────────────────────────────────────────────────────
# One X11 device client, built two ways from the same tree:
#
#   cli — pure-Go, no cgo, no GTK. The headless daemon plus `abacad connect` /
#         `abacad capabilities`. Cross-compiles anywhere with a Go toolchain, and
#         is what install.sh serves. Output: linux/build/abacad
#   app — the same binary built `-tags gui` (cgo + GTK4 + libadwaita), so
#         `abacad --gui` opens the desktop window. Shipped as a .deb with a
#         launcher and a systemd user service. Needs a Linux host with the GTK
#         dev packages; it does not cross-compile.
#
# The GUI recipes live in linux/Makefile (they need the gui-only toolchain);
# these are passthroughs so `make linux-deb` works from the repo root like every
# other platform's release target.

# Build the daemon.
linux:
	cd linux && go build -ldflags "$(GO_LINUX_LDFLAGS)" -o build/abacad ./cmd/abacad

# Build the GTK4/libadwaita desktop binary (cgo). Needs libgtk-4-dev and
# libadwaita-1-dev (>= 1.4). Output: linux/build/abacad-gui
linux-gui:
	$(MAKE) -C linux gui GO_LDFLAGS="$(GO_LINUX_LDFLAGS)"

# Package that binary as a Debian/Ubuntu .deb — the Linux app download.
# Output: linux/build/abacad_<version>_amd64.deb, staged as $(PKG_LINUX_DEB)
linux-deb linux-app:
	$(MAKE) -C linux deb GO_LDFLAGS="$(GO_LINUX_LDFLAGS)"

# Cross-compile the CLI tarballs install.sh serves (pure-Go → CGO off, any host
# cross-compiles). Each holds a single `abacad` binary, so install.sh just untars
# and moves it. `make stage` copies these into the downloads dir.
# Output: linux/build/abacad-cli-<version>-linux-{amd64,arm64}.tar.gz
linux-release:
	cd linux && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "$(GO_LINUX_LDFLAGS)" -o build/abacad ./cmd/abacad
	tar -czf linux/build/$(PKG_LINUX_CLI_AMD64) -C linux/build abacad
	cd linux && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags "$(GO_LINUX_LDFLAGS)" -o build/abacad ./cmd/abacad
	tar -czf linux/build/$(PKG_LINUX_CLI_ARM64) -C linux/build abacad
	rm -f linux/build/abacad
	@echo "Built linux/build/$(PKG_LINUX_CLI_AMD64) and $(PKG_LINUX_CLI_ARM64) (v$(VERSION))"

# Build + run against a relay: make linux-run URL=wss://host/device?token=…
linux-run: linux
	./linux/build/abacad --server-url "$(URL)"

# Unit tests plus the headless end-to-end suite under Xvfb (skips if Xvfb absent).
linux-test:
	cd linux && go test ./... && go test -tags e2e -run TestXvfbE2E ./internal/e2e

# ── Windows ──────────────────────────────────────────────────────────────────
# Tray client targeting Windows 10/11. Needs a Windows host with the .NET 8 SDK:
# the WinUI 3 (Windows App SDK) targets and the XAML compiler do not resolve on
# macOS or Linux, so this no longer cross-builds — CI runs it on the self-hosted
# NUC. Debug: windows/bin/Debug/net8.0-windows/. Release: a self-contained
# single exe at windows/publish/Abacad.exe.

# `windows` (bare) is the release build, for back-compat and `make windows`.
windows: windows-release

windows-debug:
	dotnet build windows/Abacad.csproj -c Debug

# Self-contained single-file exe (bundles the .NET runtime, so it runs on a clean
# Windows box). This is the installer's payload, not a shipped artifact — see
# windows-installer below. Output: windows/publish/Abacad.exe
windows-release:
	dotnet publish windows/Abacad.csproj -c Release -r win-x64 --self-contained -p:PublishSingleFile=true -o windows/publish

# Wrap the published exe in a per-user Inno Setup installer — the artifact that
# actually ships, the analogue of the macOS .dmg and the Linux .deb. Installs to
# %LOCALAPPDATA%\Programs with no UAC prompt, creates a Start Menu entry, and
# offers run-at-login; see windows/installer/abacad.iss for the reasoning.
#
# Needs Inno Setup 6.3+ with ISCC.exe on PATH (machine-scope, and readable by
# NT AUTHORITY\NETWORK SERVICE on the self-hosted runner — same provisioning
# trap that release.yml documents for make and dotnet).
#
# Only PRESENCE is asserted here. ISCC has no --version flag (it prints a banner
# and exits 1), its banner carries the major version only — "Inno Setup 6" — and
# the exe ships no PE version resource (0.0.0.0), so there is nothing to parse
# before a compile begins. The 6.3+ assertion therefore lives in abacad.iss as an
# ISPP #if on VER, which the preprocessor evaluates before packaging any file.
#
# Still UNSIGNED: there is no Authenticode certificate in the repo yet, so
# SmartScreen warns on download. The hook is the commented SignTool directive in
# the .iss — sign the installer there, not the payload exe, so reputation accrues
# to the certificate instead of to a per-release file hash.
#
# -D, not /D, and not //D either. A lone /DAppVersion=... is rewritten by MSYS
# into a Windows path (C:/Program Files/Git/DAppVersion=...) before ISCC sees it.
# The usual // escape is NOT a fix here: it only collapses back to / when an MSYS
# shell spawns the program itself, and make (native Win32) does not, so ISCC gets
# a literal //DAppVersion=... and rejects it with "Unknown option". ISCC accepts
# -D for every /-option, which has no leading slash and so is immune to both.
# Output: windows/installer/Output/abacad-setup.exe
windows-installer: windows-release
	@command -v ISCC.exe >/dev/null 2>&1 || \
	   { echo "error: ISCC.exe not on PATH — install Inno Setup 6.3+ (machine scope)" >&2; exit 1; }
	ISCC.exe -DAppVersion=$(VERSION) windows/installer/abacad.iss

# The console build of the same agent — no WinUI, no tray, so unlike the app it
# cross-compiles from any host with the .NET 8 SDK (EnableWindowsTargeting in the
# csproj). Also unsigned; same missing-cert story as above.
# Output: windows/publish-cli/abacad.exe
windows-cli-release:
	dotnet publish windows/cli/Abacad.Cli.csproj -c Release -r win-x64 --self-contained -p:PublishSingleFile=true -o windows/publish-cli

# ── macOS ────────────────────────────────────────────────────────────────────
# Needs a Mac with the Swift/Xcode toolchain; these targets do not build elsewhere.

# Build + sign the .app bundle. With the ad-hoc default this is dev-grade; with
# a Developer ID identity it is a distributable, hardened, timestamped signature.
# Output: macos/build/abacad.app
macos:
	swift build --package-path macos -c $(MAC_CONF) --product abacad
	rm -rf "$(MAC_APP)"
	mkdir -p "$(MAC_APP)/Contents/MacOS" "$(MAC_APP)/Contents/Resources"
	cp "$(MAC_BINARY)" "$(MAC_APP)/Contents/MacOS/abacad"
	cp macos/Info.plist "$(MAC_APP)/Contents/Info.plist"
	plutil -replace CFBundleShortVersionString -string "$(VERSION)" "$(MAC_APP)/Contents/Info.plist"
	@if [ -f "$(MAC_ICNS)" ]; then cp "$(MAC_ICNS)" "$(MAC_APP)/Contents/Resources/AppIcon.icns"; echo "  + bundled AppIcon.icns"; \
	 else echo "  (no AppIcon.icns — run 'make macos-icon' to generate it)"; fi
	codesign --force $(CODESIGN_FLAGS) --sign "$(SIGN_IDENTITY)" --identifier "$(BUNDLE_ID)" "$(MAC_APP)"
	codesign --verify --strict --verbose=2 "$(MAC_APP)"
	@echo "Built $(MAC_APP) (signed as $(SIGN_IDENTITY), id $(BUNDLE_ID))"

# App icon: render the mark to an .iconset with AppKit (macos/Tools/GenAppIcon.swift)
# and pack it with iconutil. Needs only the Swift toolchain + iconutil (no rasterizer).
# NOT a dependency of `macos`: the resulting $(MAC_ICNS) is committed, so `macos` just
# bundles it. Re-run this after changing the mark, then commit the refreshed .icns.
macos-icon:
	rm -rf "$(MAC_ICONSET)"
	swift macos/Tools/GenAppIcon.swift "$(MAC_ICONSET)"
	iconutil -c icns "$(MAC_ICONSET)" -o "$(MAC_ICNS)"
	@echo "Built $(MAC_ICNS)"

# Distributable disk image: the .app plus an /Applications symlink for
# drag-install. For a real release use `make macos-release`, which notarizes +
# staples; a bare `make macos-dmg` just packages whatever signature `macos` produced.
macos-dmg: macos _mac-pkg-dmg
	@echo "Built $(MAC_DMG)"

# Full distributable pipeline: sign → notarize + staple the .app → package the
# dmg around the stapled app → notarize + staple the dmg. The app is signed
# exactly once (by the `macos` dep); the helper targets below carry no rebuild
# dependency, so the stapled ticket is never clobbered by a re-sign.
#
#   make macos-release SIGN_IDENTITY="Developer ID Application: … (R3845XW5FZ)"
#
# Needs a Developer ID identity and a valid NOTARY_PROFILE (see header).
macos-release: macos
	@test "$(SIGN_IDENTITY)" != "-" || { echo "ERROR: macos-release needs a Developer ID SIGN_IDENTITY, not ad-hoc '-'." >&2; exit 1; }
	$(MAKE) _mac-notarize-app
	$(MAKE) _mac-pkg-dmg
	$(MAKE) _mac-notarize-dmg
	@echo "== Gatekeeper assessment =="
	spctl -a -vv "$(MAC_APP)"
	@echo "Release ready: $(MAC_DMG)"

# Forget every TCC grant for this bundle id. Needed once after switching off
# ad-hoc signing: the rows left behind in System Settings are keyed to dead
# cdhashes, and a stale row shadows the new signature instead of being replaced.
# Quit the app first; re-grant on the next launch and the grant then sticks.
macos-trust-reset:
	-tccutil reset ScreenCapture $(BUNDLE_ID)
	-tccutil reset Accessibility $(BUNDLE_ID)
	@echo "Cleared TCC grants for $(BUNDLE_ID) — relaunch and grant once more."

# ── macOS command line ───────────────────────────────────────────────────────
# `abacad connect` / `capabilities` / `status` as a standalone binary, for a Mac
# you administer over ssh. It configures; the .app still does the driving (TCC
# keys Screen Recording and Accessibility to the bundle identity, so a bare
# binary cannot hold those grants) — see macos/Sources/abacad-cli/main.swift.
#
# Signed with its own identifier: it is a separate distributable from the app,
# and sharing $(BUNDLE_ID) would make two different binaries claim one identity.
# Output: $(MAC_CLI_BIN)
MAC_CLI_BUILT := macos/.build/$(MAC_CONF)/abacad-cli
CLI_BUNDLE_ID ?= ai.abacad.cli

macos-cli:
	swift build --package-path macos -c $(MAC_CONF) --product abacad-cli
	mkdir -p "$(dir $(MAC_CLI_BIN))"
	cp "$(MAC_CLI_BUILT)" "$(MAC_CLI_BIN)"
	codesign --force $(CODESIGN_FLAGS) --sign "$(SIGN_IDENTITY)" --identifier "$(CLI_BUNDLE_ID)" "$(MAC_CLI_BIN)"
	codesign --verify --strict --verbose=2 "$(MAC_CLI_BIN)"
	@echo "Built $(MAC_CLI_BIN) (signed as $(SIGN_IDENTITY), id $(CLI_BUNDLE_ID))"

# Notarize the CLI. There is no stapling step: a ticket can only be stapled to a
# bundle, dmg or pkg, never to a bare Mach-O, so the notarization lives on
# Apple's side and Gatekeeper checks it online. That is fine for how this is
# installed — `curl | tar` sets no quarantine attribute, so Gatekeeper is not in
# the path at all; notarizing is for the person who downloads it in a browser.
macos-cli-release: macos-cli
	@test "$(SIGN_IDENTITY)" != "-" || { echo "ERROR: macos-cli-release needs a Developer ID SIGN_IDENTITY, not ad-hoc '-'." >&2; exit 1; }
	rm -f macos/build/abacad-cli-notarize.zip
	ditto -c -k "$(MAC_CLI_BIN)" macos/build/abacad-cli-notarize.zip
	xcrun notarytool submit macos/build/abacad-cli-notarize.zip --keychain-profile "$(NOTARY_PROFILE)" $(NOTARY_KEYCHAIN) --wait
	rm -f macos/build/abacad-cli-notarize.zip
	@echo "Notarized $(MAC_CLI_BIN)"

macos-clean:
	rm -rf macos/.build macos/build

# ── internal macOS helpers (operate on an already-built .app; no rebuild) ─────

_mac-pkg-dmg:
	rm -rf macos/build/dmg-staging "$(MAC_DMG)"
	mkdir -p macos/build/dmg-staging
	cp -R "$(MAC_APP)" macos/build/dmg-staging/
	ln -s /Applications macos/build/dmg-staging/Applications
	hdiutil create -volname "$(MAC_VOLNAME)" -srcfolder macos/build/dmg-staging -ov -format UDZO "$(MAC_DMG)"
	rm -rf macos/build/dmg-staging
# Sign the dmg itself with Developer ID (+ timestamp; the hardened-runtime flag
# is for executables, not images) so Gatekeeper sees a usable signature on the
# download, not just the stapled ticket. Ad-hoc dev builds leave it unsigned.
ifneq ($(SIGN_IDENTITY),-)
	codesign --force --timestamp --sign "$(SIGN_IDENTITY)" "$(MAC_DMG)"
	codesign --verify --verbose=2 "$(MAC_DMG)"
endif
	@echo "Built $(MAC_DMG)"

# Submit the .app for notarization and staple the ticket into the bundle, so the
# app passes Gatekeeper offline even after it's copied out of the dmg.
_mac-notarize-app:
	rm -f macos/build/abacad-notarize.zip
	ditto -c -k --keepParent "$(MAC_APP)" macos/build/abacad-notarize.zip
	xcrun notarytool submit macos/build/abacad-notarize.zip --keychain-profile "$(NOTARY_PROFILE)" $(NOTARY_KEYCHAIN) --wait
	xcrun stapler staple "$(MAC_APP)"
	rm -f macos/build/abacad-notarize.zip
	@echo "Notarized + stapled $(MAC_APP)"

# Notarize + staple the dmg itself (the artifact users actually download).
_mac-notarize-dmg:
	xcrun notarytool submit "$(MAC_DMG)" --keychain-profile "$(NOTARY_PROFILE)" $(NOTARY_KEYCHAIN) --wait
	xcrun stapler staple "$(MAC_DMG)"
	@echo "Notarized + stapled $(MAC_DMG)"

# ── Staging & the manifest ───────────────────────────────────────────────────

# Copy the built RELEASE artifacts into the downloads dir under their published
# names and (re)generate manifest.json — the single file every consumer reads:
# the /downloads page renders from it, install.sh greps the Linux tarball URL out
# of it, and a future in-app auto-updater can diff against it. Building a client
# leaves it in its own build tree; staging is what makes it downloadable, and it
# travels with a fresh manifest so there's nothing to keep in sync by hand.
#
#   make build release   → builds all four, then stages + writes the manifest, so
#                          a following `make dev` serves a working downloads page.
#   make stage           → (re)stage whatever is already built, no rebuild.
#
# Each stage-* copies only if its artifact exists, so a partial build (say just
# `make linux-release && make stage`) stages a partial set and the manifest lists
# exactly what's there. In production the same thing happens by copying the release
# assets + manifest.json onto the deploy volume — no restart needed.
stage: stage-macos stage-android stage-linux stage-windows
	node scripts/gen-manifest.mjs "$(DOWNLOADS)"
	@ls -lh "$(DOWNLOADS)"

# Back-compat alias — infra and docs say `make publish`.
publish: stage

# Regenerate manifest.json alone (after hand-dropping a file into $(DOWNLOADS)).
manifest:
	node scripts/gen-manifest.mjs "$(DOWNLOADS)"

# The .app is what you run locally; the .dmg is what people download. Copy the
# dmg exactly as macos-release left it — never rebuild here, or a re-sign/repack
# would silently discard the stapled notarization ticket. No dmg yet ⇒ skip with
# a hint rather than shipping an unsigned one by surprise.
stage-macos:
	@mkdir -p "$(DOWNLOADS)"
	@if [ -f "$(MAC_DMG)" ]; then cp "$(MAC_DMG)" "$(DOWNLOADS)/$(PKG_MACOS)"; echo "  staged $(PKG_MACOS)"; \
	 else echo "  (skip macos app: no $(MAC_DMG) — run 'make macos-release')"; fi
	@# COPYFILE_DISABLE stops bsdtar writing AppleDouble ._ sidecars for any xattrs
	@# on the binary — they would extract next to `abacad` and confuse anyone
	@# untarring it. The Developer ID signature is embedded in the Mach-O itself,
	@# so nothing is lost by dropping them.
	@if [ -f "$(MAC_CLI_BIN)" ]; then COPYFILE_DISABLE=1 tar -czf "$(DOWNLOADS)/$(PKG_MACOS_CLI)" -C "$(dir $(MAC_CLI_BIN))" abacad; echo "  staged $(PKG_MACOS_CLI)"; \
	 else echo "  (skip macos cli: no $(MAC_CLI_BIN) — run 'make macos-cli-release')"; fi

# The debug APK is debuggable — anyone with ADB access to a user's phone could
# attach to a service that reads the screen and injects taps. Stage the release
# build only (release-only staging means the debug APK never reaches here).
stage-android:
	@mkdir -p "$(DOWNLOADS)"
	@if [ -f "$(APK_RELEASE)" ]; then cp "$(APK_RELEASE)" "$(DOWNLOADS)/$(PKG_ANDROID)"; echo "  staged $(PKG_ANDROID)"; \
	 else echo "  (skip android: no release APK — run 'make android-release')"; fi

# Linux ships both kinds: the cgo-free CLI tarballs (both arches) and the GTK
# desktop .deb (amd64 only — it needs cgo, so it doesn't cross-compile).
stage-linux:
	@mkdir -p "$(DOWNLOADS)"
	@for f in "$(PKG_LINUX_CLI_AMD64)" "$(PKG_LINUX_CLI_ARM64)"; do \
	  if [ -f "linux/build/$$f" ]; then cp "linux/build/$$f" "$(DOWNLOADS)/$$f"; echo "  staged $$f"; \
	  else echo "  (skip linux cli: no linux/build/$$f — run 'make linux-release')"; fi; \
	done
	@if [ -f "$(LINUX_DEB)" ]; then cp "$(LINUX_DEB)" "$(DOWNLOADS)/$(PKG_LINUX_DEB)"; echo "  staged $(PKG_LINUX_DEB)"; \
	 else echo "  (skip linux app: no $(LINUX_DEB) — run 'make linux-deb')"; fi

stage-windows:
	@mkdir -p "$(DOWNLOADS)"
	@if [ -f "$(WIN_SETUP)" ]; then cp "$(WIN_SETUP)" "$(DOWNLOADS)/$(PKG_WINDOWS)"; echo "  staged $(PKG_WINDOWS)"; \
	 else echo "  (skip windows app: no $(WIN_SETUP) — run 'make windows-installer')"; fi
	@# .zip rather than .tar.gz: Windows opens one without extra tooling. -j junks
	@# the path so the archive holds a bare abacad.exe.
	@if [ ! -f "$(WIN_CLI_EXE)" ]; then echo "  (skip windows cli: no $(WIN_CLI_EXE) — run 'make windows-cli-release')"; \
	 elif ! command -v zip >/dev/null 2>&1; then echo "  (skip windows cli: no zip on PATH — install it to stage $(PKG_WINDOWS_CLI))"; \
	 else rm -f "$(DOWNLOADS)/$(PKG_WINDOWS_CLI)"; \
	   zip -qj "$(DOWNLOADS)/$(PKG_WINDOWS_CLI)" "$(WIN_CLI_EXE)"; \
	   echo "  staged $(PKG_WINDOWS_CLI)"; fi
