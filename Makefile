# Abacad dev tasks.

# Port the dashboard (Vite dev server) is served on — the URL you open in a browser.
PORT ?= 1419

# The Go backend listens here. The Vite dev proxy targets this port, so keep it
# in sync with server/frontend/vite.config.ts if you change it.
BACKEND_ADDR ?= :1213

.PHONY: dev typecheck android android-install

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

# Build the debug APK. Output: android/app/build/outputs/apk/debug/app-debug.apk
android:
	cd android && ./gradlew assembleDebug

# Build (via the android target) and install the debug APK onto the connected device.
android-install: android
	cd android && ./gradlew installDebug
