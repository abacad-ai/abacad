# abacad macOS agent — build, sign, notarize, and staple a distributable .app/.dmg.
#
# Run on a Mac (needs Swift/Xcode toolchain). This box (Linux) cannot build it.
#
# ── Signing & notarization ───────────────────────────────────────────────────
#   SIGN_IDENTITY  : codesign identity.
#                      "-" = ad-hoc (local dev only; NOT distributable —
#                            Gatekeeper blocks it and TCC grants are unstable).
#                    For distribution use the Developer ID Application cert:
#                      make release SIGN_IDENTITY="Developer ID Application: Beijing Xiaoyuanzhu Technology Co., Ltd. (R3845XW5FZ)"
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
SIGN_IDENTITY  ?= -
BUNDLE_ID      ?= ai.abacad.mac
TEAM_ID        ?= R3845XW5FZ
NOTARY_PROFILE ?= abacad-notary

APP     := build/abacad.app
CONF    := release
BINARY  := .build/$(CONF)/abacad
DMG     := build/abacad.dmg
VOLNAME := abacad
ICNS    := build/abacad.icns
ICONSET := build/abacad.iconset

# A real Developer ID identity (anything but ad-hoc "-") gets the hardened
# runtime + a secure timestamp — both are prerequisites for notarization.
ifeq ($(SIGN_IDENTITY),-)
CODESIGN_FLAGS :=
else
CODESIGN_FLAGS := --options runtime --timestamp
endif

.PHONY: all app build clean run dmg release icon _pkg-dmg _notarize-app _notarize-dmg

all: app

build:
	swift build -c $(CONF)

# App icon: rasterize AppIcon.svg into an .iconset and pack it with iconutil.
# Needs a Mac (iconutil) and an SVG rasterizer — rsvg-convert (`brew install librsvg`).
# NOT a dependency of `app`: run it once (or when AppIcon.svg changes) to (re)build
# $(ICNS); `app` bundles it if present, and builds fine without it (generic icon).
icon:
	@command -v rsvg-convert >/dev/null || { echo "ERROR: rsvg-convert not found — 'brew install librsvg'." >&2; exit 1; }
	rm -rf "$(ICONSET)"; mkdir -p "$(ICONSET)"
	@for sz in 16 32 128 256 512; do \
	  rsvg-convert -w $$sz            -h $$sz            AppIcon.svg -o "$(ICONSET)/icon_$${sz}x$${sz}.png"; \
	  rsvg-convert -w $$((sz*2))      -h $$((sz*2))      AppIcon.svg -o "$(ICONSET)/icon_$${sz}x$${sz}@2x.png"; \
	done
	iconutil -c icns "$(ICONSET)" -o "$(ICNS)"
	@echo "Built $(ICNS)"

# Build + sign the .app bundle. With the ad-hoc default this is dev-grade; with
# a Developer ID identity it is a distributable, hardened, timestamped signature.
app: build
	rm -rf "$(APP)"
	mkdir -p "$(APP)/Contents/MacOS" "$(APP)/Contents/Resources"
	cp "$(BINARY)" "$(APP)/Contents/MacOS/abacad"
	cp Info.plist "$(APP)/Contents/Info.plist"
	@if [ -f "$(ICNS)" ]; then cp "$(ICNS)" "$(APP)/Contents/Resources/AppIcon.icns"; echo "  + bundled AppIcon.icns"; \
	 else echo "  (no AppIcon.icns yet — run 'make icon' on a Mac with librsvg to generate it)"; fi
	codesign --force $(CODESIGN_FLAGS) --sign "$(SIGN_IDENTITY)" --identifier "$(BUNDLE_ID)" "$(APP)"
	codesign --verify --strict --verbose=2 "$(APP)"
	@echo "Built $(APP) (signed as $(SIGN_IDENTITY), id $(BUNDLE_ID))"

# Distributable disk image: the .app plus an /Applications symlink for
# drag-install. For a real release use `make release`, which notarizes + staples;
# a bare `make dmg` just packages whatever signature `make app` produced.
dmg: app _pkg-dmg
	@echo "Built $(DMG)"

# Full distributable pipeline: sign → notarize + staple the .app → package the
# dmg around the stapled app → notarize + staple the dmg. The app is signed
# exactly once (by the `app` dep); the helper targets below carry no rebuild
# dependency, so the stapled ticket is never clobbered by a re-sign.
#
#   make release SIGN_IDENTITY="Developer ID Application: … (R3845XW5FZ)"
#
# Needs a Developer ID identity and a valid NOTARY_PROFILE (see header).
release: app
	@test "$(SIGN_IDENTITY)" != "-" || { echo "ERROR: release needs a Developer ID SIGN_IDENTITY, not ad-hoc '-'." >&2; exit 1; }
	$(MAKE) _notarize-app
	$(MAKE) _pkg-dmg
	$(MAKE) _notarize-dmg
	@echo "== Gatekeeper assessment =="
	spctl -a -vv "$(APP)"
	@echo "Release ready: $(DMG)"

# ── internal helpers (operate on an already-built build/abacad.app; no rebuild) ─

_pkg-dmg:
	rm -rf build/dmg-staging "$(DMG)"
	mkdir -p build/dmg-staging
	cp -R "$(APP)" build/dmg-staging/
	ln -s /Applications build/dmg-staging/Applications
	hdiutil create -volname "$(VOLNAME)" -srcfolder build/dmg-staging -ov -format UDZO "$(DMG)"
	rm -rf build/dmg-staging
# Sign the dmg itself with Developer ID (+ timestamp; the hardened-runtime flag
# is for executables, not images) so Gatekeeper sees a usable signature on the
# download, not just the stapled ticket. Ad-hoc dev builds leave it unsigned.
ifneq ($(SIGN_IDENTITY),-)
	codesign --force --timestamp --sign "$(SIGN_IDENTITY)" "$(DMG)"
	codesign --verify --verbose=2 "$(DMG)"
endif
	@echo "Built $(DMG)"

# Submit the .app for notarization and staple the ticket into the bundle, so the
# app passes Gatekeeper offline even after it's copied out of the dmg.
_notarize-app:
	rm -f build/abacad-notarize.zip
	ditto -c -k --keepParent "$(APP)" build/abacad-notarize.zip
	xcrun notarytool submit build/abacad-notarize.zip --keychain-profile "$(NOTARY_PROFILE)" --wait
	xcrun stapler staple "$(APP)"
	rm -f build/abacad-notarize.zip
	@echo "Notarized + stapled $(APP)"

# Notarize + staple the dmg itself (the artifact users actually download).
_notarize-dmg:
	xcrun notarytool submit "$(DMG)" --keychain-profile "$(NOTARY_PROFILE)" --wait
	xcrun stapler staple "$(DMG)"
	@echo "Notarized + stapled $(DMG)"

# Convenience: build + launch. First launch will prompt for Accessibility and
# Screen Recording — grant both in System Settings, then relaunch.
run: app
	open "$(APP)"

clean:
	rm -rf .build build
