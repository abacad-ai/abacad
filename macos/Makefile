# Abacad macOS agent — build a signed .app bundle from the SwiftPM executable.
#
# Run on a Mac (needs Swift/Xcode toolchain). This box (Linux) cannot build it.
#
# ── PLACEHOLDERS — set these before shipping ─────────────────────────────────
#   SIGN_IDENTITY : codesign identity. "-" is ad-hoc (works for local dev, but
#                   TCC grants are less stable across rebuilds). For a durable
#                   identity use your cert, e.g.:
#                     make SIGN_IDENTITY="Apple Development: you@example.com (TEAMID)"
#   BUNDLE_ID     : must match CFBundleIdentifier in Info.plist.
# ─────────────────────────────────────────────────────────────────────────────
SIGN_IDENTITY ?= -
BUNDLE_ID     ?= com.example.abacad.agent

APP     := build/AbacadAgent.app
CONF    := release
BINARY  := .build/$(CONF)/AbacadAgent

.PHONY: all app build sign clean run

all: app

build:
	swift build -c $(CONF)

app: build
	rm -rf "$(APP)"
	mkdir -p "$(APP)/Contents/MacOS" "$(APP)/Contents/Resources"
	cp "$(BINARY)" "$(APP)/Contents/MacOS/AbacadAgent"
	cp Info.plist "$(APP)/Contents/Info.plist"
	codesign --force --sign "$(SIGN_IDENTITY)" --identifier "$(BUNDLE_ID)" "$(APP)"
	@echo "Built $(APP) (signed as $(SIGN_IDENTITY), id $(BUNDLE_ID))"

# Convenience: build + launch. First launch will prompt for Accessibility and
# Screen Recording — grant both in System Settings, then relaunch.
run: app
	open "$(APP)"

clean:
	rm -rf .build build
