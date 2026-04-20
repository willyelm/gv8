#!/usr/bin/env bash
# Download and install the bundled libgv8 runtime for the current or requested target.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
VERSION="$(cat "${ROOT}/internal/v8/VERSION")"
GV8_TARGET="${GV8_TARGET:-${1:-}}"
DIST_VERSION="${GV8_DIST_VERSION:-${VERSION}}"
BASE_URL="${GV8_DIST_BASE_URL:-https://github.com/willyelm/gv8/releases/download/v8-${DIST_VERSION}}"

if [ -z "${GV8_TARGET}" ]; then
  case "$(uname -s)/$(uname -m)" in
    Darwin/arm64) GV8_TARGET="darwin/arm64" ;;
    Linux/x86_64) GV8_TARGET="linux/x86_64" ;;
    Linux/aarch64|Linux/arm64) GV8_TARGET="linux/arm64" ;;
    *)
      echo "unsupported host: $(uname -s)/$(uname -m)" >&2
      exit 1
      ;;
  esac
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

ASSET_NAME="gv8-v8-${DIST_VERSION}-${TARGET_DIR}.tar.gz"
URL="${BASE_URL}/${ASSET_NAME}"
INSTALL_DIR="${ROOT}/internal/v8/${TARGET_DIR}"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "${TMP_DIR}"' EXIT

mkdir -p "${INSTALL_DIR}"
echo "downloading ${URL}"
curl -fL "${URL}" -o "${TMP_DIR}/${ASSET_NAME}"
tar -C "${INSTALL_DIR}" -xzf "${TMP_DIR}/${ASSET_NAME}"
case "${GV8_TARGET}" in
  darwin/arm64) LIB_NAME="libgv8.dylib" ;;
  linux/x86_64|linux/arm64) LIB_NAME="libgv8.so" ;;
esac
echo "installed: ${INSTALL_DIR}/${LIB_NAME}"
