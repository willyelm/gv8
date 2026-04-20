#!/usr/bin/env bash
# Package the built libgv8 shared library for a target as a release asset.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
VERSION="$(cat "${ROOT}/internal/v8/VERSION")"
GV8_TARGET="${GV8_TARGET:-${1:-}}"
DIST_DIR="${ROOT}/dist"

if [ -z "${GV8_TARGET}" ]; then
  echo "usage: GV8_TARGET=os/arch $0" >&2
  echo "example: GV8_TARGET=darwin/arm64 $0" >&2
  exit 2
fi

case "${GV8_TARGET}" in
  darwin/arm64) TARGET_DIR="darwin_arm64" ;;
  linux/x86_64) TARGET_DIR="linux_x86_64" ;;
  linux/arm64) TARGET_DIR="linux_arm64" ;;
  *)
    echo "unsupported target: ${GV8_TARGET}" >&2
    exit 1
    ;;
esac

SOURCE_DIR="${ROOT}/internal/v8/${TARGET_DIR}"
ASSET_NAME="gv8-v8-${VERSION}-${TARGET_DIR}.tar.gz"

case "${GV8_TARGET}" in
  darwin/arm64) LIB_NAME="libgv8.dylib" ;;
  linux/x86_64|linux/arm64) LIB_NAME="libgv8.so" ;;
esac

SOURCE_FILE="${SOURCE_DIR}/${LIB_NAME}"
if [ ! -f "${SOURCE_FILE}" ]; then
  echo "error: missing ${SOURCE_FILE}; run the platform build first" >&2
  exit 1
fi

mkdir -p "${DIST_DIR}"
if [ -f "${SOURCE_DIR}/icudtl.dat" ]; then
  tar -C "${SOURCE_DIR}" -czf "${DIST_DIR}/${ASSET_NAME}" "${LIB_NAME}" icudtl.dat
else
  tar -C "${SOURCE_DIR}" -czf "${DIST_DIR}/${ASSET_NAME}" "${LIB_NAME}"
fi
echo "done: ${DIST_DIR}/${ASSET_NAME}"
