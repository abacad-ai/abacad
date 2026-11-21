.PHONY: build test vet fmt run clean

BIN := bin/abacad

build:
	go build -trimpath -o $(BIN) ./cmd/abacad

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './internal/web/assets/*')

run:
	go run ./cmd/abacad serve --listen 127.0.0.1:8080 --no-mdns

clean:
	rm -rf bin
