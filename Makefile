# dso — build, test, and codegen targets.
#
# Go is expected on PATH. In this dev environment it lives outside the default
# PATH; see the project notes. Example:
#   export PATH="$HOME/sdk/go/bin:$HOME/go/bin:$PATH"

GO      ?= go
BUF     ?= buf
PKG     ?= ./...

.PHONY: all build test vet fmt proto proto-check tidy clean

all: build

build:
	$(GO) build $(PKG)

test:
	$(GO) test $(PKG)

vet:
	$(GO) vet ./internal/... ./cmd/... 2>/dev/null || $(GO) vet ./internal/...

fmt:
	$(GO) fmt ./internal/... ./cmd/... 2>/dev/null || $(GO) fmt ./internal/...

# Regenerate protobuf Go code from proto/ using buf managed mode.
proto:
	$(BUF) generate

# CI guard: regeneration must produce no diff.
proto-check: proto
	git diff --exit-code -- internal/proto

tidy:
	$(GO) mod tidy

clean:
	rm -rf bin dist
