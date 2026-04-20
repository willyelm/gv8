#!/usr/bin/env bash
# Fetch V8 source at the version pinned in internal/v8/VERSION and update
# bundled headers/version data.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
if [ ! -f "${ROOT}/internal/v8/VERSION" ]; then
  echo "error: missing ${ROOT}/internal/v8/VERSION" >&2
  exit 1
fi
REF="$(tr -d '[:space:]' < "${ROOT}/internal/v8/VERSION")"

if [ -z "${REF}" ]; then
  echo "error: internal/v8/VERSION is empty" >&2
  exit 2
fi

WORKROOT="${ROOT}/.v8"
WORKTREE="${WORKROOT}/src/v8"

mkdir -p "$(dirname "${WORKTREE}")"

cd "$(dirname "${WORKTREE}")"

ensure_checkout() {
  rm -rf "${WORKTREE}"
  git clone --filter=blob:none https://chromium.googlesource.com/v8/v8.git "${WORKTREE}"
}

ensure_checkout

git -C "${WORKTREE}" checkout --detach "${REF}"

COMMIT="$(git -C "${WORKTREE}" rev-parse HEAD)"
HEADER="${WORKTREE}/include/v8-version.h"
MAJOR="$(awk '/^#define V8_MAJOR_VERSION/ {print $3}' "${HEADER}")"
MINOR="$(awk '/^#define V8_MINOR_VERSION/ {print $3}' "${HEADER}")"
BUILD_N="$(awk '/^#define V8_BUILD_NUMBER/ {print $3}' "${HEADER}")"
PATCH="$(awk '/^#define V8_PATCH_LEVEL/ {print $3}' "${HEADER}")"
VERSION="${MAJOR}.${MINOR}.${BUILD_N}.${PATCH}"

rm -rf "${ROOT}/internal/v8/include"
cp -R "${WORKTREE}/include" "${ROOT}/internal/v8/include"
printf '%s\n' "${VERSION}" > "${ROOT}/internal/v8/VERSION"
printf '%s\n' "${COMMIT}"  > "${ROOT}/internal/v8/SOURCE_COMMIT"

echo "fetched V8 v${VERSION} @ ${COMMIT:0:12}"
echo "next: make build"
