#!/usr/bin/env bash
set -euo pipefail

RESULTS_DIR=".ci-fast-results"
rm -rf "$RESULTS_DIR"
mkdir -p "$RESULTS_DIR"

CI_FAST_PARALLEL="${CI_FAST_PARALLEL:-6}"
TIMESTAMP=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

run_task() {
  local name="$1"
  shift
  local start_ns
  start_ns=$(date +%s%N)
  local log_file="$RESULTS_DIR/${name}.log"
  local exit_code=0

  "$@" > "$log_file" 2>&1 || exit_code=$?

  local end_ns
  end_ns=$(date +%s%N)
  local duration_ms=$(( (end_ns - start_ns) / 1000000 ))

  jq -n \
    --arg name "$name" \
    --arg exit_code "$exit_code" \
    --arg duration_ms "$duration_ms" \
    --arg timestamp "$TIMESTAMP" \
    '{name: $name, exitCode: ($exit_code | tonumber), durationMs: ($duration_ms | tonumber), timestamp: $timestamp}' \
    > "$RESULTS_DIR/${name}.json"

  if [ "$exit_code" -ne 0 ]; then
    local truncated_log
    truncated_log=$(tail -c 5000 "$log_file" | jq -Rs .)
    jq --argjson log "$truncated_log" '. + {logs: $log}' "$RESULTS_DIR/${name}.json" > "$RESULTS_DIR/${name}.json.tmp"
    mv "$RESULTS_DIR/${name}.json.tmp" "$RESULTS_DIR/${name}.json"
  fi

  return "$exit_code"
}

export -f run_task
export RESULTS_DIR TIMESTAMP

declare -A TASK_NAMES=()
declare -A TASK_CMDS=()

TASK_NAMES[lint-go]="lint-go"
TASK_CMDS[lint-go]="make lint-go"

TASK_NAMES[lint-ts]="lint-ts"
TASK_CMDS[lint-ts]="yarn lint"

TASK_NAMES[typecheck]="typecheck"
TASK_CMDS[typecheck]="yarn typecheck"

TASK_NAMES[test-go-unit]="test-go-unit"
TASK_CMDS[test-go-unit]="make test-go-unit SHARD=1 SHARDS=1"

TASK_NAMES[test-js]="test-js"
TASK_CMDS[test-js]="yarn jest --no-watch --ci --maxWorkers=50%"

TASK_NAMES[knip]="knip"
TASK_CMDS[knip]="yarn knip"

PIDS=()
NAMES=()

for key in "${!TASK_NAMES[@]}"; do
  name="${TASK_NAMES[$key]}"
  cmd="${TASK_CMDS[$key]}"
  run_task "$name" bash -c "$cmd" &
  PIDS+=($!)
  NAMES+=("$name")
done

FAILED=0
FAILURES=()

for i in "${!PIDS[@]}"; do
  pid="${PIDS[$i]}"
  name="${NAMES[$i]}"
  if ! wait "$pid"; then
    FAILED=$((FAILED + 1))
    FAILURES+=("$name")
  fi
done

ALL_JSON=$(jq -s '.' "$RESULTS_DIR"/*.json)

if [ "$FAILED" -gt 0 ]; then
  FAILURE_JSON=$(echo "$ALL_JSON" | jq -c 'map(select(.exitCode != 0))')
  OUTPUT=$(jq -n \
    --arg status "failed" \
    --argjson total "${#TASK_NAMES[@]}" \
    --argjson failed "$FAILED" \
    --argjson failures "$FAILURE_JSON" \
    --argjson results "$ALL_JSON" \
    --arg timestamp "$TIMESTAMP" \
    '{status: $status, totalTasks: $total, failedCount: $failed, failures: $failures, results: $results, timestamp: $timestamp}')

  echo "$OUTPUT" | jq . > "$RESULTS_DIR/report.json"
  echo ""
  echo "=========================================="
  echo "  ci-fast FAILED ($FAILED/${#TASK_NAMES[@]} tasks)"
  echo "=========================================="
  for name in "${FAILURES[@]}"; do
    echo "  ✗ $name"
  done
  echo ""
  echo "Structured JSON report: $RESULTS_DIR/report.json"
  echo "Individual task logs: $RESULTS_DIR/<task>.log"
  echo ""
  cat "$RESULTS_DIR/report.json"
  exit 1
else
  OUTPUT=$(jq -n \
    --arg status "passed" \
    --argjson total "${#TASK_NAMES[@]}" \
    --argjson failed "0" \
    --argjson results "$ALL_JSON" \
    --arg timestamp "$TIMESTAMP" \
    '{status: $status, totalTasks: $total, failedCount: $failed, results: $results, timestamp: $timestamp}')

  echo "$OUTPUT" | jq . > "$RESULTS_DIR/report.json"
  echo ""
  echo "=========================================="
  echo "  ci-fast PASSED (${#TASK_NAMES[@]}/${#TASK_NAMES[@]} tasks)"
  echo "=========================================="
  for key in "${!TASK_NAMES[@]}"; do
    echo "  ✓ ${TASK_NAMES[$key]}"
  done
  echo ""
  echo "Report: $RESULTS_DIR/report.json"
  exit 0
fi
