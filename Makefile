# abacad dev tasks.

# Port the dashboard (Vite dev server) is served on — the URL you open in a browser.
PORT ?= 1419

# The Go backend listens here. The Vite dev proxy targets this port, so keep it
# in sync with server/frontend/vite.config.ts if you change it.
BACKEND_ADDR ?= :1213

# Where the server looks for public release artifacts. The backend runs with
# server/backend as its working directory and defaults to a relative
# "abacad-downloads" (ABACAD_DOWNLOADS overrides it), so this is that directory.
DOWNLOADS ?= server/backend/abacad-downloads

.PHONY: dev typecheck tokens android android-install macos macos-run publish publish-macos publish-android

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

# Build the signed macOS .app bundle. Output: macos/build/abacad.app
# Needs a Mac with the Swift/Xcode toolchain. See macos/Makefile for signing vars.
macos:
	cd macos && $(MAKE) app

# Build (via the macos target) and launch the .app. First launch prompts for
# Accessibility and Screen Recording — grant both, then relaunch.
macos-run:
	cd macos && $(MAKE) run

# Publish the built clients to the downloads directory under the names the
# server serves at /downloads/ and lists on the public /downloads page:
# abacad-<platform>-latest.<ext>. Building a client leaves it in its own build
# tree; only this step makes it downloadable. In production the same thing
# happens by copying the artifact onto the deploy volume — no restart needed.
publish: publish-macos publish-android
	@ls -lh $(DOWNLOADS)

# The .app is what you run locally; the .dmg is what people download, so this
# packages one (macos/Makefile's dmg target = app + hdiutil). For a signed,
# notarized artifact run `cd macos && make release` first, then this copies it.
publish-macos:
	cd macos && $(MAKE) dmg
	@mkdir -p $(DOWNLOADS)
	cp macos/build/abacad.dmg $(DOWNLOADS)/abacad-macos-latest.dmg

publish-android: android
	@mkdir -p $(DOWNLOADS)
	cp android/app/build/outputs/apk/debug/app-debug.apk $(DOWNLOADS)/abacad-android-latest.apk
