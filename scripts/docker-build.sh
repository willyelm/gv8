#!/usr/bin/env bash
# Build the bundled runtime for a Linux target inside Docker.
# Usage: scripts/docker-build.sh linux/amd64|linux/arm64
#
# Prerequisites:
#   - Docker installed and running
#   - V8 source already fetched: make fetch
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PLATFORM="${1:-}"

case "${PLATFORM}" in
  linux/amd64|linux/arm64) ;;
  *)
    echo "usage: $0 linux/amd64|linux/arm64" >&2
    exit 1
    ;;
esac

IMAGE="gv8-builder"
case "${PLATFORM}" in
  linux/amd64)
    GV8_TARGET="linux/x86_64"
    DOCKER_PLATFORM="linux/amd64"
    ;;
  linux/arm64)
    GV8_TARGET="linux/arm64"
    # The Chromium Rust host toolchain in V8 DEPS is Linux x64-oriented.
    # Build the arm64 target from an amd64 Linux host environment.
    DOCKER_PLATFORM="linux/amd64"
    ;;
esac

echo "building Docker image ${IMAGE}..."
docker build --platform "${DOCKER_PLATFORM}" -t "${IMAGE}" "${ROOT}/scripts/docker"

echo "building V8 runtime for ${PLATFORM}..."
docker run --rm \
  --platform "${DOCKER_PLATFORM}" \
  -v "${ROOT}:/workspace" \
  -w /workspace \
  "${IMAGE}" \
  env GV8_TARGET="${GV8_TARGET}" NINJA_JOBS="${NINJA_JOBS:-2}" scripts/build-v8.sh
