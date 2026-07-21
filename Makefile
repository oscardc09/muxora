VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(BUILD_DATE)

.PHONY: build test vet check run install install-local fmt clean version

build:
	go build -ldflags "$(LDFLAGS)" -o bin/muxora ./cmd/muxora

test:
	go test ./...

vet:
	go vet ./...

check: test vet build

run:
	go run ./cmd/muxora

install:
	go install -ldflags "$(LDFLAGS)" ./cmd/muxora

# Instala sin depender del GOPATH. ~/.local/bin debe estar incluido en PATH.
install-local: build
	mkdir -p "$(HOME)/.local/bin"
	install -m 0755 bin/muxora "$(HOME)/.local/bin/muxora"

fmt:
	gofmt -w cmd internal

clean:
	rm -f bin/muxora

version: build
	./bin/muxora --version
