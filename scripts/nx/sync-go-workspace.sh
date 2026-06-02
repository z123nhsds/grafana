#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT=$(cd "$(dirname "$0")/../.." && pwd)
GO_WORK_FILE="$REPO_ROOT/go.work"

if [ ! -f "$GO_WORK_FILE" ]; then
  echo "Error: go.work not found at $GO_WORK_FILE"
  exit 1
fi

MODULE_PATHS=$(go run "$REPO_ROOT/scripts/go-workspace/main.go" list-submodules --path "$GO_WORK_FILE")

NX_CACHE_DIR="$REPO_ROOT/.nx/cache"
mkdir -p "$NX_CACHE_DIR"

GO_WORK_HASH_FILE="$NX_CACHE_DIR/go-work-hash"
CURRENT_HASH=""
if [ -f "$GO_WORK_HASH_FILE" ]; then
  CURRENT_HASH=$(cat "$GO_WORK_HASH_FILE")
fi

NEW_HASH=$(cat "$GO_WORK_FILE" "$REPO_ROOT/go.work.sum" 2>/dev/null | sha256sum | cut -d' ' -f1)

if [ "$CURRENT_HASH" = "$NEW_HASH" ]; then
  echo "go.work unchanged — NX project configs are up to date."
  exit 0
fi

echo "Syncing go.work modules to NX project configurations..."

for module_path in $MODULE_PATHS; do
  FULL_PATH="$REPO_ROOT/$module_path"
  PROJECT_JSON="$FULL_PATH/project.json"

  if [ ! -d "$FULL_PATH" ]; then
    continue
  fi

  MODULE_NAME=""
  if [ -f "$FULL_PATH/go.mod" ]; then
    MODULE_NAME=$(head -1 "$FULL_PATH/go.mod" | awk '{print $2}')
  fi

  RELATIVE_PATH="$module_path"
  PROJECT_NAME=$(echo "$RELATIVE_PATH" | tr '/' '-')

  if [ -f "$PROJECT_JSON" ]; then
    EXISTING_TAGS=$(jq -r '.tags // [] | if type == "array" then join(",") else . end' "$PROJECT_JSON" 2>/dev/null || echo "")
    if echo "$EXISTING_TAGS" | grep -q "scope:go-module"; then
      echo "  ✓ $module_path — already tagged as go-module"
      continue
    fi

    UPDATED_TAGS=$(echo "$EXISTING_TAGS" | jq -R -s '
      split(",") | map(select(length > 0)) + ["scope:go-module"] | unique
    ' 2>/dev/null || echo '["scope:go-module"]')

    jq --argjson tags "$UPDATED_TAGS" '.tags = $tags' "$PROJECT_JSON" > "$PROJECT_JSON.tmp" && mv "$PROJECT_JSON.tmp" "$PROJECT_JSON"
    echo "  ✓ $module_path — added scope:go-module tag"
  else
    cat > "$PROJECT_JSON" << EOF
{
  "name": "$PROJECT_NAME",
  "root": "$module_path",
  "tags": ["scope:go-module"],
  "targets": {
    "test": {
      "executor": "nx:run-commands",
      "options": {
        "command": "cd $module_path && go test -short -timeout=5m ./..."
      },
      "inputs": ["default", "^default", "goSources"],
      "cache": true
    },
    "lint": {
      "executor": "nx:run-commands",
      "options": {
        "command": "golangci-lint run --config {workspaceRoot}/.golangci.yml $module_path/..."
      },
      "inputs": ["default", "goSources", "{workspaceRoot}/.golangci.yml"],
      "cache": true
    }
  }
}
EOF
    echo "  ✓ $module_path — created project.json with go-module targets"
  fi
done

echo "$NEW_HASH" > "$GO_WORK_HASH_FILE"
echo "NX project sync complete. Hash: $NEW_HASH"
