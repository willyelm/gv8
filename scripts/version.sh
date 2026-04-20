#!/usr/bin/env bash
set -euo pipefail

cat "$(cd "$(dirname "$0")/.." && pwd)/internal/v8/VERSION"
