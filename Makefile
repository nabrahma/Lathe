GO      ?= go
TOOLS   := $(CURDIR)/.tools/bin
WAILS   := $(TOOLS)/wails
LINT    := $(TOOLS)/golangci-lint
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/nabrahma/lathe/internal/version.Version=$(VERSION)

export CGO_ENABLED = 0

.PHONY: all tools deps build build-cli dev test test-race test-short lint fmt vet check boundary clean corpus appicon

all: check build

tools:
	@mkdir -p "$(TOOLS)"
	GOBIN="$(TOOLS)" $(GO) install github.com/wailsapp/wails/v2/cmd/wails@v2.15.0
	GOBIN="$(TOOLS)" $(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest

deps:
	$(GO) mod download
	cd frontend && npm ci

build-cli:
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o bin/lathe ./cmd/lathe-cli

# The desktop build needs cgo on macOS and Linux, where the webview is a C
# library. Windows does not, because Wails loads WebView2 through pure Go, but
# setting it everywhere keeps one command working on all three.
build:
	CGO_ENABLED=1 "$(WAILS)" build -trimpath -ldflags "$(LDFLAGS)"

dev:
	CGO_ENABLED=1 "$(WAILS)" dev

test:
	$(GO) test ./...

test-short:
	$(GO) test -short ./...

test-race:
	$(GO) test -race ./...

lint:
	"$(LINT)" run

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

boundary:
	$(GO) run ./scripts/boundary

appicon:
	go run ./scripts/appicon

corpus:
	$(GO) run ./scripts/gencorpus

check: fmt vet boundary test

clean:
	rm -rf bin build/bin frontend/dist testdata/corpus/generated
