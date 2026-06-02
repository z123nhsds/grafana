#!/usr/bin/env bash
set -euo pipefail

PROJECT_DIR="${1:-.}"
shift || true

EXTRA_ARGS=("$@")

cd "$PROJECT_DIR"

if [ -f "go.mod" ]; then
  go test -short -timeout=5m ./... "${EXTRA_ARGS[@]}"
else
  echo "No go.mod found in $PROJECT_DIR, skipping."
  exit 0
fi
