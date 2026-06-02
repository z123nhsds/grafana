#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
GO_WORK="${REPO_ROOT}/go.work"
NX_PROJECTS_DIR="${REPO_ROOT}/.nx/go-modules"

GO_WORK_TOOL="${REPO_ROOT}/scripts/go-workspace/main.go"

mkdir -p "${NX_PROJECTS_DIR}"

list_go_modules() {
  go run "${GO_WORK_TOOL}" list-submodules --path "${GO_WORK}" --delimiter $'\n'
}

get_module_name() {
  local mod_path="$1"
  head -1 "${REPO_ROOT}/${mod_path}/go.mod" | sed 's/^module //'
}

read_go_deps() {
  local mod_path="$1"
  local go_mod="${REPO_ROOT}/${mod_path}/go.mod"

  grep 'require' -A 1000 "${go_mod}" 2>/dev/null | \
    grep -v 'require' | \
    grep -v '^$' | \
    grep -v '^//' | \
    grep -v 'indirect' | \
    sed 's/^\s*//' | \
    awk '{print $1}' | \
    grep 'github.com/grafana/grafana' || true
}

generate_nx_project() {
  local mod_path="$1"
  local mod_name
  mod_name=$(get_module_name "${mod_path}")

  local project_name
  project_name=$(echo "${mod_path}" | sed 's|^\./||' | sed 's|/|-|g')
  [[ "${project_name}" == "." ]] && project_name="grafana-root"

  local deps_json="[]"
  local deps
  deps=$(read_go_deps "${mod_path}")

  if [[ -n "${deps}" ]]; then
    local dep_list=""
    while IFS= read -r dep; do
      local dep_path
      dep_path=$(echo "${dep}" | sed 's|github.com/grafana/grafana/||')
      local dep_name
      dep_name=$(echo "${dep_path}" | sed 's|/|-|g')
      if [[ -n "${dep_list}" ]]; then
        dep_list="${dep_list}, "
      fi
      dep_list="${dep_list}\"${dep_name}\""
    done <<< "${deps}"
    deps_json="[${dep_list}]"
  fi

  cat > "${NX_PROJECTS_DIR}/${project_name}.json" <<EOF
{
  "name": "${project_name}",
  "\$schema": "../../node_modules/nx/schemas/project-schema.json",
  "projectType": "library",
  "sourceRoot": "${mod_path}",
  "tags": ["scope:go-module", "type:backend"],
  "targets": {
    "test": {
      "executor": "nx:run-commands",
      "inputs": [
        "{projectRoot}/**/*.go",
        "{projectRoot}/go.mod",
        "{projectRoot}/go.sum",
        "{workspaceRoot}/go.work",
        "{workspaceRoot}/go.work.sum"
      ],
      "outputs": [
        "{projectRoot}/.nx-go-test-cache"
      ],
      "cache": true,
      "options": {
        "cwd": "{projectRoot}",
        "command": "go test -short -count=1 -timeout=10m ./... 2>&1 | tee {projectRoot}/.nx-go-test-cache/output.txt; exit \${PIPESTATUS[0]}"
      }
    },
    "lint": {
      "executor": "nx:run-commands",
      "inputs": [
        "{projectRoot}/**/*.go",
        "{projectRoot}/go.mod",
        "{projectRoot}/go.sum"
      ],
      "cache": true,
      "options": {
        "cwd": "{workspaceRoot}",
        "command": "make lint-go GO_LINT_FILES=${mod_path}/..."
      }
    },
    "build": {
      "executor": "nx:run-commands",
      "inputs": [
        "{projectRoot}/**/*.go",
        "{projectRoot}/go.mod",
        "{projectRoot}/go.sum",
        "{workspaceRoot}/go.work",
        "{workspaceRoot}/go.work.sum"
      ],
      "outputs": [
        "{projectRoot}/bin"
      ],
      "cache": true,
      "options": {
        "cwd": "{projectRoot}",
        "command": "go build ./..."
      }
    }
  },
  "implicitDependencies": {
    "go.work": "*",
    "go.work.sum": "*"
  }
}
EOF

  echo "  Generated: ${project_name} (${mod_path})"
}

main() {
  echo "Syncing go.work modules to NX project configurations..."
  echo ""

  rm -rf "${NX_PROJECTS_DIR}"
  mkdir -p "${NX_PROJECTS_DIR}"

  local modules
  modules=$(list_go_modules)

  while IFS= read -r mod_path; do
    [[ -z "${mod_path}" ]] && continue
    generate_nx_project "${mod_path}"
  done <<< "${modules}"

  echo ""
  echo "Done! Generated $(ls "${NX_PROJECTS_DIR}" | wc -l) NX project configurations."
  echo "Location: ${NX_PROJECTS_DIR}"
  echo ""
  echo "To use: npx nx run-many -t test --projects='tag:scope:go-module'"
  echo "Or:     npx nx affected -t test --projects='tag:scope:go-module'"
}

main "$@"