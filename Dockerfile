# syntax=docker/dockerfile:1

# VERSION is stamped into the binary by the release workflow:
#   docker build --build-arg VERSION=v2.0.0 .
ARG VERSION=dev

FROM node:24-alpine AS frontend
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM golang:1.27-alpine AS build
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /src/internal/web/dist ./internal/web/dist
RUN CGO_ENABLED=0 go build -trimpath \
      -ldflags="-s -w -X main.version=${VERSION}" \
      -o /out/abacad ./cmd/abacad

FROM alpine:3.22
RUN addgroup -S -g 10001 abacad && adduser -S -D -H -u 10001 -G abacad abacad \
 && mkdir -p /config && chown abacad:abacad /config \
 && apk add --no-cache ca-certificates
COPY --from=build /out/abacad /usr/local/bin/abacad
USER abacad

# The image is configured entirely through the environment; see README.md.
ENV ABACAD_CONFIG=/config/config.yaml \
    ABACAD_LISTEN=0.0.0.0:8787
VOLUME ["/config"]
# One port carries everything: web UI, API, MCP and every device's VNC WebSocket.
EXPOSE 8787

# Follows ABACAD_LISTEN, so a different port keeps working. TLS deployments
# should pass --no-healthcheck (the probe speaks plain HTTP).
HEALTHCHECK --interval=30s --timeout=5s --start-period=3s --retries=3 \
  CMD wget -q -O /dev/null "http://127.0.0.1:${ABACAD_LISTEN##*:}/healthz" || exit 1
ENTRYPOINT ["/usr/local/bin/abacad"]
CMD ["serve"]
