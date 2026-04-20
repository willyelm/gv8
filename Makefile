GO ?= go
ROOT := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))

.PHONY: test fmt version fetch build build-darwin build-linux-x64 build-linux-arm64

test:
	env -u GOROOT -u GOPATH GOCACHE=/tmp/gv8-go-cache $(GO) test ./...

fmt:
	$(GO) fmt ./...

version:
	$(ROOT)scripts/version.sh

# Fetch V8 source at the version pinned in internal/v8/VERSION.
fetch:
	$(ROOT)scripts/fetch-v8.sh

# Build the bundled runtime natively for darwin/arm64.
build-darwin: build

# Build the bundled runtime natively for the current platform.
build:
	$(ROOT)scripts/build-v8.sh

# Build the bundled runtime for linux/amd64 via Docker.
build-linux-x64:
	$(ROOT)scripts/docker-build.sh linux/amd64

# Build the bundled runtime for linux/arm64 via Docker.
build-linux-arm64:
	$(ROOT)scripts/docker-build.sh linux/arm64
