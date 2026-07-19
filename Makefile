# Abacad dev tasks.

# Port the dashboard (Vite dev server) is served on — the URL you open in a browser.
PORT ?= 1419

# The Go backend listens here. The Vite dev proxy targets this port, so keep it
# in sync with server/frontend/vite.config.ts if you change it.
BACKEND_ADDR ?= :1213

.PHONY: dev typecheck tokens android android-install macos macos-run deploy

# Start the Go backend and the Vite frontend together in the foreground.
# Open http://localhost:$(PORT). Ctrl-C stops both.
dev:
	@cd server/frontend && npm install
	@trap 'kill 0' INT TERM EXIT; \
	  ( cd server/backend && ABACAD_ADDR=$(BACKEND_ADDR) go run ./cmd/abacad -dev-cors ) & \
	  ( cd server/frontend && npm run dev -- --port $(PORT) ) & \
	  wait

typecheck:
	cd server/backend && go build ./...
	cd server/frontend && npm run typecheck

# Regenerate the per-platform design tokens (tokens.css / Theme.kt / Theme.swift)
# from design/tokens.json. Commit the outputs together with the JSON change.
tokens:
	node design/generate.mjs

# Build the debug APK. Output: android/app/build/outputs/apk/debug/app-debug.apk
android:
	cd android && ./gradlew assembleDebug

# Build (via the android target) and install the debug APK onto the connected device.
android-install: android
	cd android && ./gradlew installDebug

# Build the signed macOS .app bundle. Output: macos/build/AbacadAgent.app
# Needs a Mac with the Swift/Xcode toolchain. See macos/Makefile for signing vars.
macos:
	cd macos && $(MAKE) app

# Build (via the macos target) and launch the .app. First launch prompts for
# Accessibility and Screen Recording — grant both, then relaunch.
macos-run:
	cd macos && $(MAKE) run

# Deploy to production (host: xyz-sg-1, override with DEPLOY_HOST=…): builds the
# server image + the macOS client dmg, ships both, restarts the server, and
# publishes the dmg at https://abacad.ai/downloads/abacad-macos-latest.dmg.
# See deploy.sh for the steps.
deploy:
	./deploy.sh
