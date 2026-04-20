GO ?= go
ROOT := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))

.PHONY: test fmt version fetch build build-darwin build-linux-x64 build-linux-arm64 install-v8 release

test:
	env -u GOROOT -u GOPATH GOCACHE=/tmp/gv8-go-cache $(GO) test ./...

fmt:
	$(GO) fmt ./...

version:
	$(ROOT)scripts/version.sh

# Fetch V8 source at the version pinned in internal/v8/VERSION.
fetch:
	$(ROOT)scripts/fetch-v8.sh

# Build the bundled archives natively for darwin/arm64.
build-darwin: build

# Build the bundled archives natively for the current platform.
build:
	$(ROOT)scripts/build-v8.sh

# Build the bundled archives for linux/amd64 via Docker.
build-linux-x64:
	$(ROOT)scripts/docker-build.sh linux/amd64

# Build the bundled archives for linux/arm64 via Docker.
build-linux-arm64:
	$(ROOT)scripts/docker-build.sh linux/arm64

install-v8:
	$(ROOT)scripts/install-v8.sh

release:
	GV8_TARGET=darwin/arm64 $(ROOT)scripts/package-v8.sh
	GV8_TARGET=linux/x86_64 $(ROOT)scripts/package-v8.sh
	GV8_TARGET=linux/arm64 $(ROOT)scripts/package-v8.sh
