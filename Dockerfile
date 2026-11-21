# syntax=docker/dockerfile:1
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/abacad ./cmd/abacad

FROM alpine:3.22
RUN addgroup -S -g 10001 abacad && adduser -S -D -H -u 10001 -G abacad abacad \
 && mkdir -p /config && chown abacad:abacad /config \
 && apk add --no-cache ca-certificates
COPY --from=build /out/abacad /usr/local/bin/abacad
USER abacad
EXPOSE 8787
# Raw VNC listeners are allocated from 5901-5999 at runtime. With
# --network host they need no EXPOSE; with bridge networking, publish the range.
VOLUME ["/config"]
ENTRYPOINT ["/usr/local/bin/abacad"]
CMD ["serve", "--config", "/config/config.yaml", "--listen", "0.0.0.0:8787"]
