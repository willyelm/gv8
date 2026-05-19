GO ?= go
ROOT := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))
TEST_CACHE_ROOT ?= /tmp/gv8-go
GO_TEST_ENV = env -u GOROOT -u GOPATH GOCACHE=$(TEST_CACHE_ROOT)/cache GOMODCACHE=$(TEST_CACHE_ROOT)/mod GOTMPDIR=$(TEST_CACHE_ROOT)/tmp

.PHONY: test test-leak test-stress ci-test fmt version fetch build build-darwin build-linux-x64 build-linux-arm64

test:
	mkdir -p $(TEST_CACHE_ROOT)/cache $(TEST_CACHE_ROOT)/mod $(TEST_CACHE_ROOT)/tmp
	$(GO_TEST_ENV) $(GO) test ./...

test-leak:
	mkdir -p $(TEST_CACHE_ROOT)/cache $(TEST_CACHE_ROOT)/mod $(TEST_CACHE_ROOT)/tmp
	$(GO_TEST_ENV) $(GO) test -count=10 -run 'TestContextCloseReleasesHostFunctions|TestContextCloseClearsRegistryAndResolver|TestRepeatedContextLifecycleDoesNotLeakRegistrations' ./...

test-stress:
	mkdir -p $(TEST_CACHE_ROOT)/cache $(TEST_CACHE_ROOT)/mod $(TEST_CACHE_ROOT)/tmp
	$(GO_TEST_ENV) $(GO) test -count=10 -run 'TestRepeatedRunAndCloseCycle|TestConcurrentAccessRequiresExternalSynchronization|TestConcurrentObjectAccessRequiresExternalSynchronization|TestServerRuntimeEssentials' ./...

ci-test: test test-leak test-stress

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
