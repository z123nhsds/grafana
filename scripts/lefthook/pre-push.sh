#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
GO_WORK="${REPO_ROOT}/go.work"

REMOTE="${1:-origin}"
URL="${2:-}"

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

git_cmd() {
  git -C "${REPO_ROOT}" "$@"
}

get_base_ref() {
  local default_branch
  default_branch=$(git_cmd symbolic-ref refs/remotes/origin/HEAD 2>/dev/null | sed 's@^refs/remotes/origin/@@' || echo "main")
  echo "origin/${default_branch}"
}

get_changed_files() {
  local base_ref
  base_ref=$(get_base_ref)

  git_cmd diff --name-only "${base_ref}...HEAD" 2>/dev/null || git_cmd diff --name-only HEAD~1 2>/dev/null || echo ""
}

get_go_module_path() {
  local file="$1"

  while [[ "${file}" != "." && "${file}" != "/" ]]; do
    local dir
    dir=$(dirname "${file}")
    if [[ -f "${REPO_ROOT}/${dir}/go.mod" ]]; then
      echo "${dir}"
      return 0
    fi
    file="${dir}"
  done

  if [[ -f "${REPO_ROOT}/go.mod" ]]; then
    echo "."
    return 0
  fi

  return 1
}

get_affected_go_packages() {
  local changed_files="$1"
  local packages=()
  local seen=()

  while IFS= read -r file; do
    [[ -z "${file}" ]] && continue
    [[ ! "${file}" =~ \.go$ ]] && continue

    local mod_path
    mod_path=$(get_go_module_path "${file}" 2>/dev/null) || continue

    if [[ -z "${seen[${mod_path}]:-}" ]]; then
      seen["${mod_path}"]=1
      packages+=("${mod_path}")
    fi
  done <<< "${changed_files}"

  (IFS=$'\n'; echo "${packages[*]}")
}

get_affected_nx_projects() {
  local changed_files="$1"
  local ts_files=()

  while IFS= read -r file; do
    [[ -z "${file}" ]] && continue
    if [[ "${file}" =~ \.(ts|tsx|js|jsx|json|scss|css)$ ]] && \
       [[ ! "${file}" =~ ^\.github/ ]] && \
       [[ ! "${file}" =~ \.gen\.ts$ ]]; then
      ts_files+=("${file}")
    fi
  done <<< "${changed_files}"

  if [[ ${#ts_files[@]} -eq 0 ]]; then
    echo ""
    return 0
  fi

  cd "${REPO_ROOT}" && npx nx show projects --affected --base="$(get_base_ref)" --head=HEAD 2>/dev/null || echo ""
}

run_affected_go_tests() {
  local packages="$1"
  local failed=0

  while IFS= read -r pkg; do
    [[ -z "${pkg}" ]] && continue

    local go_pkg_path="./${pkg}/..."
    if [[ "${pkg}" == "." ]]; then
      go_pkg_path="./pkg/..."
    fi

    echo -e "${YELLOW}Running Go tests for: ${go_pkg_path}${NC}"

    if ! go test -short -timeout=10m -count=1 "${go_pkg_path}" 2>&1; then
      failed=1
    fi
  done <<< "${packages}"

  return "${failed}"
}

run_affected_js_tests() {
  local projects="$1"

  if [[ -z "${projects}" ]]; then
    echo -e "${GREEN}No affected JS projects detected.${NC}"
    return 0
  fi

  echo -e "${YELLOW}Running tests for affected JS projects:${NC}"
  echo "${projects}"

  local failed=0
  while IFS= read -r project; do
    [[ -z "${project}" ]] && continue
    echo -e "${YELLOW}  Testing: ${project}${NC}"
    if ! npx nx test "${project}" --skip-nx-cache 2>&1; then
      failed=1
    fi
  done <<< "${projects}"

  return "${failed}"
}

main() {
  echo ""
  echo "============================================"
  echo "  Pre-push: Detecting changed packages..."
  echo "============================================"

  local changed_files
  changed_files=$(get_changed_files)

  if [[ -z "${changed_files}" ]]; then
    echo -e "${GREEN}No changes detected. Skipping pre-push checks.${NC}"
    exit 0
  fi

  echo ""
  echo "Changed files:"
  echo "${changed_files}" | head -20
  local total_changed
  total_changed=$(echo "${changed_files}" | wc -l)
  if [[ "${total_changed}" -gt 20 ]]; then
    echo "  ... and $((total_changed - 20)) more files"
  fi

  local go_packages
  go_packages=$(get_affected_go_packages "${changed_files}")
  local nx_projects
  nx_projects=$(get_affected_nx_projects "${changed_files}")

  echo ""
  echo "Affected Go modules:"
  if [[ -z "${go_packages}" ]]; then
    echo "  (none)"
  else
    echo "${go_packages}" | while IFS= read -r p; do [[ -n "${p}" ]] && echo "  ${p}"; done
  fi

  echo ""
  echo "Affected NX projects:"
  if [[ -z "${nx_projects}" ]]; then
    echo "  (none)"
  else
    echo "${nx_projects}" | while IFS= read -r p; do [[ -n "${p}" ]] && echo "  ${p}"; done
  fi

  local overall_failed=0

  if [[ -n "${go_packages}" ]]; then
    echo ""
    echo "============================================"
    echo "  Running affected Go tests..."
    echo "============================================"
    if ! run_affected_go_tests "${go_packages}"; then
      overall_failed=1
    fi
  fi

  if [[ -n "${nx_projects}" ]]; then
    echo ""
    echo "============================================"
    echo "  Running affected JS tests..."
    echo "============================================"
    if ! run_affected_js_tests "${nx_projects}"; then
      overall_failed=1
    fi
  fi

  echo ""
  if [[ "${overall_failed}" -eq 0 ]]; then
    echo -e "${GREEN}All affected tests passed!${NC}"
    exit 0
  else
    echo -e "${RED}Some affected tests failed. Push aborted.${NC}"
    echo ""
    echo "Tip: Use 'make ci-fast' to run the full CI suite locally."
    exit 1
  fi
}

main "$@"