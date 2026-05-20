#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
HEADER="${ROOT}/internal/v8/include/v8-version.h"

if [ ! -f "${HEADER}" ]; then
  echo "error: V8 headers not found - run 'make fetch' first" >&2
  exit 1
fi

MAJOR="$(awk '/^#define V8_MAJOR_VERSION/ {print $3}' "${HEADER}")"
MINOR="$(awk '/^#define V8_MINOR_VERSION/ {print $3}' "${HEADER}")"
BUILD_N="$(awk '/^#define V8_BUILD_NUMBER/ {print $3}' "${HEADER}")"
PATCH="$(awk '/^#define V8_PATCH_LEVEL/ {print $3}' "${HEADER}")"
echo "${MAJOR}.${MINOR}.${BUILD_N}.${PATCH}"
