# abacad dev tasks. Every target lives here — there is no per-platform Makefile.

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
# "abacad-downloads" (ABACAD_DOWNLOADS overrides it), so this is that directory.
DOWNLOADS ?= server/backend/abacad-downloads

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

.PHONY: build build-debug build-release debug release \
        dev tokens bump-version version android android-release \
        linux linux-release linux-run linux-test \
        macos macos-icon macos-dmg macos-release macos-trust-reset macos-clean \
        windows windows-debug windows-release \
        publish publish-macos publish-android \
        _mac-pkg-dmg _mac-notarize-app _mac-notarize-dmg

# Build every client platform, both variants. Run this on macOS, the only host
# that can build the macOS client (the others build there with their own
# toolchains). A trailing word selects one variant:
#
#   make build          → debug + release for all four platforms
#   make build debug    → debug/dev builds only (fast, local; macOS ad-hoc signed)
#   make build release  → publishable builds only (signed; macOS notarized dmg)
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

build-debug: android linux macos windows-debug
	@echo "Built debug clients: Android, Linux, macOS, Windows (v$(VERSION))"

build-release: android-release linux-release macos-release windows-release
	@echo "Built release clients: Android, Linux, macOS, Windows (v$(VERSION))"

# Start the Go backend and the Vite frontend together in the foreground.
# Open http://localhost:$(PORT). Ctrl-C stops both.
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
bump-version:
	@test -n "$(V)" || { echo "usage: make bump-version V=x.y.z" >&2; exit 1; }
	@printf '%s\n' "$(V)" > VERSION
	@for f in server/package.json server/frontend/package.json; do \
	  awk -v v="$(V)" 'BEGIN{d=0} /"version":/ && !d {sub(/"version":[ \t]*"[^"]*"/, "\"version\": \"" v "\""); d=1} {print}' "$$f" > "$$f.tmp" && mv "$$f.tmp" "$$f"; \
	done
	@for f in server/package-lock.json server/frontend/package-lock.json; do \
	  [ -f "$$f" ] || continue; \
	  awk -v v="$(V)" 'BEGIN{d=0} /"version":/ && d<2 {sub(/"version":[ \t]*"[^"]*"/, "\"version\": \"" v "\""); d++} {print}' "$$f" > "$$f.tmp" && mv "$$f.tmp" "$$f"; \
	done
	@echo "Bumped abacad to $(V) (VERSION + package.json + package-lock.json). Rebuild to stamp clients/server."

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
	{ git add VERSION; for f in server/package.json server/frontend/package.json server/package-lock.json server/frontend/package-lock.json; do [ -f "$$f" ] && git add "$$f"; done; true; } && \
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
# Headless X11 device client (pure-Go, no cgo). Builds anywhere with a Go
# toolchain. Output: linux/build/abacad

# Build the daemon.
linux:
	cd linux && go build -ldflags "$(GO_LINUX_LDFLAGS)" -o build/abacad ./cmd/abacad

# Cross-compile the release binaries install.sh serves (pure-Go → CGO off, any
# host cross-compiles). Copy the outputs into the server's downloads dir to
# publish. Output: linux/build/abacad-linux-{amd64,arm64}
linux-release:
	cd linux && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "$(GO_LINUX_LDFLAGS)" -o build/abacad-linux-amd64 ./cmd/abacad
	cd linux && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags "$(GO_LINUX_LDFLAGS)" -o build/abacad-linux-arm64 ./cmd/abacad
	@echo "Built linux/build/abacad-linux-amd64 and -arm64 (v$(VERSION))"

# Build + run against a relay: make linux-run URL=wss://host/device?token=…
linux-run: linux
	./linux/build/abacad --server-url "$(URL)"

# Unit tests plus the headless end-to-end suite under Xvfb (skips if Xvfb absent).
linux-test:
	cd linux && go test ./... && go test -tags e2e -run TestXvfbE2E ./internal/e2e

# ── Windows ──────────────────────────────────────────────────────────────────
# Tray client targeting Windows 10/11. The .NET SDK can cross-build it from
# macOS or Linux. Output: windows/bin/<Debug|Release>/net8.0-windows/

# `windows` (bare) is the release build, for back-compat and `make windows`.
windows: windows-release

windows-debug:
	dotnet build windows/Abacad.csproj -c Debug

# Release config, but UNSIGNED: there's no Authenticode/code-signing cert path in
# the repo yet, so the .exe runs but SmartScreen warns on download. TODO: sign
# here once a cert is available.
windows-release:
	dotnet build windows/Abacad.csproj -c Release

# ── macOS ────────────────────────────────────────────────────────────────────
# Needs a Mac with the Swift/Xcode toolchain; these targets do not build elsewhere.

# Build + sign the .app bundle. With the ad-hoc default this is dev-grade; with
# a Developer ID identity it is a distributable, hardened, timestamped signature.
# Output: macos/build/abacad.app
macos:
	swift build --package-path macos -c $(MAC_CONF)
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
	xcrun notarytool submit macos/build/abacad-notarize.zip --keychain-profile "$(NOTARY_PROFILE)" --wait
	xcrun stapler staple "$(MAC_APP)"
	rm -f macos/build/abacad-notarize.zip
	@echo "Notarized + stapled $(MAC_APP)"

# Notarize + staple the dmg itself (the artifact users actually download).
_mac-notarize-dmg:
	xcrun notarytool submit "$(MAC_DMG)" --keychain-profile "$(NOTARY_PROFILE)" --wait
	xcrun stapler staple "$(MAC_DMG)"
	@echo "Notarized + stapled $(MAC_DMG)"

# ── Publishing ───────────────────────────────────────────────────────────────

# Publish the built clients to the downloads directory under the names the
# server serves at /downloads/ and lists on the public /downloads page:
# abacad-<platform>-latest.<ext>. Building a client leaves it in its own build
# tree; only this step makes it downloadable. In production the same thing
# happens by copying the artifact onto the deploy volume — no restart needed.
publish: publish-macos publish-android
	@ls -lh $(DOWNLOADS)

# The .app is what you run locally; the .dmg is what people download, so this
# copies one. It builds an unnotarized dmg only if none exists yet — depending on
# macos-dmg unconditionally would re-sign the .app and repackage, silently
# discarding the stapled ticket from a preceding `make macos-release`. So the
# release flow is `make macos-release && make publish`; to force a fresh dev dmg,
# `make macos-clean` (or `make macos-dmg`) first.
publish-macos:
	@test -f "$(MAC_DMG)" || $(MAKE) macos-dmg
	@mkdir -p $(DOWNLOADS)
	cp $(MAC_DMG) $(DOWNLOADS)/abacad-macos-latest.dmg

# The debug APK is debuggable — anyone with ADB access to a user's phone could
# attach to a service that reads the screen and injects taps. Publish the
# release build only.
publish-android: android-release
	@mkdir -p $(DOWNLOADS)
	cp android/app/build/outputs/apk/release/app-release.apk $(DOWNLOADS)/abacad-android-latest.apk
