# Abacad dev tasks.

# Dev server port. 121314 is NOT a valid port (max is 65535); using 1213.
PORT ?= 1213

.PHONY: dev typecheck

# Start the server in the foreground with hot reload. Ctrl-C stops it.
dev:
	cd server && PORT=$(PORT) npx tsx --watch src/index.ts

typecheck:
	cd server && npm run typecheck
