.PHONY: build test vet fmt run clean version docker frontend-deps frontend-build

BIN := bin/abacad
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X main.version=$(VERSION)

build: frontend-build
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/abacad

frontend-deps:
	cd frontend && npm ci

frontend-build: frontend-deps
	cd frontend && npm run build
	touch internal/web/dist/.gitkeep

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './internal/web/assets/*' -not -path './frontend/*')

run:
	go run ./cmd/abacad serve --listen 127.0.0.1:8787 --no-mdns

version:
	@echo $(VERSION)

# Local image for testing the same input the release workflow uses.
docker:
	docker build --build-arg VERSION=$(VERSION) -t abacad:local .

clean:
	rm -rf bin
	find internal/web/dist -mindepth 1 ! -name .gitkeep -delete
