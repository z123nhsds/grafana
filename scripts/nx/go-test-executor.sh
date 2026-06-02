#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

PROJECT_ROOT="${1:-.}"
GO_RACE="${GO_RACE:-}"

GO_RACE_FLAG=""
[[ -n "${GO_RACE}" ]] && GO_RACE_FLAG="-race"

echo "Running Go tests for: ${PROJECT_ROOT}"

cd "${PROJECT_ROOT}" || exit 1

CACHE_DIR="${PROJECT_ROOT}/.nx-go-test-cache"
mkdir -p "${CACHE_DIR}"

TIMESTAMP=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

go test \
  ${GO_RACE_FLAG} \
  -short \
  -count=1 \
  -timeout=10m \
  -json \
  ./... 2>&1 | tee "${CACHE_DIR}/output.json"

EXIT_CODE=${PIPESTATUS[0]}

echo "{\"timestamp\": \"${TIMESTAMP}\", \"exit_code\": ${EXIT_CODE}}" > "${CACHE_DIR}/result.json"

exit ${EXIT_CODE}