GO      ?= go
TOOLS   := $(CURDIR)/.tools/bin
WAILS   := $(TOOLS)/wails
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/nabrahma/lathe/internal/version.Version=$(VERSION)

export CGO_ENABLED = 0

.PHONY: all tools deps build build-cli dev test test-race test-short lint fmt vet check boundary clean corpus

all: check build

tools:
	@mkdir -p $(TOOLS)
	GOBIN=$(TOOLS) $(GO) install github.com/wailsapp/wails/v2/cmd/wails@v2.15.0
	GOBIN=$(TOOLS) $(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest

deps:
	$(GO) mod download
	cd frontend && npm ci

build-cli:
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o bin/lathe ./cmd/lathe-cli

build:
	$(WAILS) build -trimpath -ldflags "$(LDFLAGS)"

dev:
	$(WAILS) dev

test:
	$(GO) test ./...

test-short:
	$(GO) test -short ./...

test-race:
	$(GO) test -race ./...

lint:
	golangci-lint run

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

boundary:
	$(GO) run ./scripts/boundary

corpus:
	$(GO) run ./scripts/gencorpus

check: fmt vet boundary test

clean:
	rm -rf bin build/bin frontend/dist testdata/corpus/generated
