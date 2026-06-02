#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
OUTPUT_DIR="${REPO_ROOT}/.ci-fast-results"
REPORT_FILE="${OUTPUT_DIR}/report.json"
SUMMARY_FILE="${OUTPUT_DIR}/summary.txt"
TIMESTAMP=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

JOBS="${CI_FAST_JOBS:-0}"
KEEP_OUTPUT="${CI_FAST_KEEP_OUTPUT:-false}"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

declare -A job_names
declare -A job_cmds
declare -A job_pids
declare -A job_statuses
declare -A job_outputs
declare -A job_durations
declare -A job_start_times

mkdir -p "${OUTPUT_DIR}"

register_job() {
  local name="$1"
  local cmd="$2"
  job_names["${name}"]="${name}"
  job_cmds["${name}"]="${cmd}"
}

run_job() {
  local name="$1"
  local log_file="${OUTPUT_DIR}/${name}.log"
  local start_time
  start_time=$(date +%s)

  job_start_times["${name}"]="${start_time}"

  if eval "${job_cmds[${name}]}" > "${log_file}" 2>&1; then
    job_statuses["${name}"]="pass"
  else
    job_statuses["${name}"]="fail"
  fi

  local end_time
  end_time=$(date +%s)
  job_durations["${name}"]=$((end_time - start_time))
  job_outputs["${name}"]="${log_file}"
}

print_status_line() {
  local name="$1"
  local status="${job_statuses[${name}]}"
  local duration="${job_durations[${name}]}"
  local icon=""

  case "${status}" in
    pass) icon="${GREEN}✓${NC}" ;;
    fail) icon="${RED}✗${NC}" ;;
    *)    icon="${YELLOW}?${NC}" ;;
  esac

  printf "  %s %-30s %ss\n" "${icon}" "${name}" "${duration}"
}

generate_json_report() {
  local json="${REPORT_FILE}"
  local overall="pass"

  printf '{\n' > "${json}"
  printf '  "timestamp": "%s",\n' "${TIMESTAMP}" >> "${json}"
  printf '  "branch": "%s",\n' "$(git -C "${REPO_ROOT}" rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown")" >> "${json}"
  printf '  "commit": "%s",\n' "$(git -C "${REPO_ROOT}" rev-parse --short HEAD 2>/dev/null || echo "unknown")" >> "${json}"

  for name in "${!job_names[@]}"; do
    if [[ "${job_statuses[${name}]}" == "fail" ]]; then
      overall="fail"
    fi
  done

  printf '  "overall": "%s",\n' "${overall}" >> "${json}"
  printf '  "jobs": [\n' >> "${json}"

  local first=true
  for name in "${!job_names[@]}"; do
    if [[ "${first}" == "true" ]]; then
      first=false
    else
      printf ',\n' >> "${json}"
    fi

    local status="${job_statuses[${name}]}"
    local duration="${job_durations[${name}]}"
    local log_file="${job_outputs[${name}]}"

    local error_summary=""
    if [[ "${status}" == "fail" ]]; then
      error_summary=$(tail -20 "${log_file}" | sed 's/"/\\"/g' | sed ':a;N;$!ba;s/\n/\\n/g')
    fi

    printf '    {\n' >> "${json}"
    printf '      "name": "%s",\n' "${name}" >> "${json}"
    printf '      "status": "%s",\n' "${status}" >> "${json}"
    printf '      "duration_seconds": %s,\n' "${duration}" >> "${json}"
    printf '      "log_file": "%s",\n' "${log_file}" >> "${json}"
    printf '      "error_summary": "%s"\n' "${error_summary}" >> "${json}"
    printf '    }' >> "${json}"
  done

  printf '\n  ]\n' >> "${json}"
  printf '}\n' >> "${json}"
}

print_summary() {
  echo ""
  echo "============================================"
  echo "  CI Fast Results"
  echo "============================================"

  local fail_count=0
  local pass_count=0

  for name in "${!job_names[@]}"; do
    print_status_line "${name}"
    if [[ "${job_statuses[${name}]}" == "fail" ]]; then
      ((fail_count++))
    else
      ((pass_count++))
    fi
  done

  echo "--------------------------------------------"
  echo "  Total: ${#job_names[@]} | ${GREEN}Passed: ${pass_count}${NC} | ${RED}Failed: ${fail_count}${NC}"
  echo "--------------------------------------------"

  if [[ "${fail_count}" -gt 0 ]]; then
    echo ""
    echo "${RED}Failed jobs error summaries:${NC}"
    for name in "${!job_names[@]}"; do
      if [[ "${job_statuses[${name}]}" == "fail" ]]; then
        echo ""
        echo "  --- ${name} (${job_outputs[${name}]}) ---"
        tail -15 "${job_outputs[${name}]}"
      fi
    done
    echo ""
    echo "Report: ${REPORT_FILE}"
  fi
}

register_job "lint-go" "
  cd '${REPO_ROOT}' &&
  make lint-go
"

register_job "lint-ts" "
  cd '${REPO_ROOT}' &&
  yarn run lint
"

register_job "typecheck" "
  cd '${REPO_ROOT}' &&
  yarn run typecheck
"

register_job "test-go-unit" "
  cd '${REPO_ROOT}' &&
  make test-go-unit
"

register_job "test-js" "
  cd '${REPO_ROOT}' &&
  yarn run test:ci
"

register_job "knip" "
  cd '${REPO_ROOT}' &&
  yarn run knip
"

echo "Starting CI Fast checks at ${TIMESTAMP}..."
echo "Parallel jobs: ${JOBS} (0 = unlimited)"
echo ""

if [[ "${JOBS}" -gt 0 ]]; then
  running=0
  for name in "${!job_names[@]}"; do
    while [[ "${running}" -ge "${JOBS}" ]]; do
      for n in "${!job_pids[@]}"; do
        if ! kill -0 "${job_pids[${n}]}" 2>/dev/null; then
          wait "${job_pids[${n}]}" || true
          unset "job_pids[${n}]"
          ((running--))
        fi
      done
      sleep 0.5
    done
    run_job "${name}" &
    job_pids["${name}"]=$!
    ((running++))
  done
else
  for name in "${!job_names[@]}"; do
    run_job "${name}" &
    job_pids["${name}"]=$!
  done
fi

for name in "${!job_pids[@]}"; do
  wait "${job_pids[${name}]}" || true
done

generate_json_report
print_summary

if [[ "${KEEP_OUTPUT}" != "true" ]]; then
  find "${OUTPUT_DIR}" -name "*.log" -mtime +7 -delete 2>/dev/null || true
fi

for name in "${!job_names[@]}"; do
  if [[ "${job_statuses[${name}]}" == "fail" ]]; then
    exit 1
  fi
done

exit 0