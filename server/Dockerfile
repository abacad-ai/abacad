# syntax=docker/dockerfile:1
#
# Multi-stage build of the abacad relay: compile the dashboard SPA, embed it into
# the Go backend, and ship a single static binary. Mirrors server/build.sh.
#
# Build (context is server/):   docker build -t abacad -f server/Dockerfile server
# Run:                          docker run -p 8848:8848 -v abacad-data:/data abacad
#
# The binary is pure-Go (modernc.org/sqlite — no cgo), so CGO stays off and we
# cross-compile for the requested TARGETARCH without QEMU.

# --- frontend: build the Vite dist (arch-independent) ---
FROM --platform=$BUILDPLATFORM node:24-alpine AS frontend
WORKDIR /app/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# --- backend: embed dist + build the static binary ---
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS backend
WORKDIR /app/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
# embed can't reach outside the module; drop the fresh dist where //go:embed expects it.
COPY --from=frontend /app/frontend/dist ./internal/web/dist
ARG TARGETOS TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /abacad ./cmd/abacad

# --- runtime: minimal, non-root, data on a volume ---
FROM alpine:3.20
RUN apk add --no-cache ca-certificates wget \
 && adduser -D -u 10001 abacad \
 && mkdir -p /data && chown abacad:abacad /data
COPY --from=backend /abacad /usr/local/bin/abacad
USER abacad
WORKDIR /data
# DB, blob store, and the SSH jump host key live under /data so a single volume
# persists all state (a stable host key keeps clients' known_hosts valid).
ENV ABACAD_ADDR=:8848 \
    ABACAD_DB=/data/abacad.db \
    ABACAD_BLOBS=/data/abacad-blobs \
    ABACAD_DOWNLOADS=/data/abacad-downloads \
    ABACAD_SSH_HOST_KEY=/data/ssh_host_ed25519_key
# The SSH jump host is opt-in: run with -e ABACAD_SSH_ADDR=:2222 to enable it.
# The container is non-root, so it can't bind :22 directly — bind :2222 inside and
# publish the host's :22 to it:  docker run -p 22:2222 -e ABACAD_SSH_ADDR=:2222 ...
EXPOSE 8848 2222
VOLUME /data
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8848/health || exit 1
ENTRYPOINT ["/usr/local/bin/abacad"]
