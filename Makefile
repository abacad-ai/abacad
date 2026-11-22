.PHONY: build test vet fmt run clean frontend-deps frontend-build

BIN := bin/abacad

build: frontend-build
	go build -trimpath -o $(BIN) ./cmd/abacad

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

clean:
	rm -rf bin
	find internal/web/dist -mindepth 1 ! -name .gitkeep -delete
