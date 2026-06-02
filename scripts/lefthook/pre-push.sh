#!/usr/bin/env bash
set -euo pipefail

BASE_REF="${1:-}"
if [ -z "$BASE_REF" ]; then
  BASE_REF=$(git merge-base HEAD origin/main 2>/dev/null || echo "HEAD~1")
fi

CHANGED_FILES=$(git diff --name-only "$BASE_REF" HEAD 2>/dev/null || git diff --name-only HEAD~1 HEAD)

if [ -z "$CHANGED_FILES" ]; then
  echo "No changes detected. Skipping pre-push checks."
  exit 0
fi

HAS_GO_CHANGES=false
HAS_TS_CHANGES=false
HAS_PKG_CHANGES=false
HAS_CUE_CHANGES=false

while IFS= read -r file; do
  case "$file" in
    *.go|go.mod|go.sum|go.work|go.work.sum)
      HAS_GO_CHANGES=true
      ;;
    *.ts|*.tsx|*.js|*.jsx|*.scss|*.css|*.mjs)
      HAS_TS_CHANGES=true
      ;;
    packages/*|public/app/plugins/*)
      HAS_PKG_CHANGES=true
      ;;
    *.cue|kinds/*)
      HAS_CUE_CHANGES=true
      ;;
  esac
done <<< "$CHANGED_FILES"

EXIT_CODE=0

if [ "$HAS_GO_CHANGES" = true ]; then
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo "  Go changes detected — running affected Go tests"
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

  GO_DIRS=$(echo "$CHANGED_FILES" | grep '\.go$' | xargs -r -n1 dirname | sort -u | sed 's,^,./,')

  if [ -n "$GO_DIRS" ]; then
    AFFECTED_PKGS=""
    for dir in $GO_DIRS; do
      if [ -f "${dir}/go.mod" ] || [ -f "${dir#./}/go.mod" ] || go list -f '{{.Dir}}' "$dir/..." >/dev/null 2>&1; then
        PKG=$(go list -f '{{.ImportPath}}' "$dir" 2>/dev/null || true)
        if [ -n "$PKG" ]; then
          AFFECTED_PKGS="$AFFECTED_PKGS $PKG"
        fi
      fi
    done

    if [ -n "$AFFECTED_PKGS" ]; then
      echo "Affected packages:$AFFECTED_PKGS"
      if ! go test -short -timeout=5m $AFFECTED_PKGS; then
        EXIT_CODE=1
        echo "✗ Go tests failed"
      else
        echo "✓ Go tests passed"
      fi
    else
      echo "No testable Go packages found in changed directories. Running quick compile check..."
      if ! go build ./...; then
        EXIT_CODE=1
        echo "✗ Go build failed"
      else
        echo "✓ Go build passed"
      fi
    fi
  fi

  echo "Running golangci-lint on changed files..."
  CHANGED_GO_FILES=$(echo "$CHANGED_FILES" | grep '\.go$' || true)
  if [ -n "$CHANGED_GO_FILES" ]; then
    GO_LINT_DIRS=$(echo "$CHANGED_GO_FILES" | xargs -r -n1 dirname | sort -u | sed 's,^,./,')
    if ! golangci-lint run --config .golangci.yml $GO_LINT_DIRS 2>/dev/null; then
      echo "⚠ golangci-lint found issues (non-blocking in pre-push)"
    fi
  fi
fi

if [ "$HAS_TS_CHANGES" = true ]; then
  echo ""
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo "  TypeScript changes detected — running affected TS tests"
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

  if [ "$HAS_PKG_CHANGES" = true ]; then
    echo "Running NX affected tests..."
    if ! npx nx affected --target=test --base="$BASE_REF" --head=HEAD 2>/dev/null; then
      EXIT_CODE=1
      echo "✗ NX affected tests failed"
    else
      echo "✓ NX affected tests passed"
    fi
  else
    CHANGED_TS_FILES=$(echo "$CHANGED_FILES" | grep -E '\.(test\.)?(ts|tsx)$' || true)
    if [ -n "$CHANGED_TS_FILES" ]; then
      echo "Running Jest for changed test files..."
      if ! yarn jest --no-watch --findRelatedTests $CHANGED_TS_FILES 2>/dev/null; then
        EXIT_CODE=1
        echo "✗ Jest tests failed"
      else
        echo "✓ Jest tests passed"
      fi
    else
      echo "Running quick typecheck on changed files..."
      if ! yarn tsc --noEmit 2>/dev/null; then
        echo "⚠ Typecheck found issues (non-blocking in pre-push)"
      fi
    fi
  fi
fi

if [ "$HAS_CUE_CHANGES" = true ]; then
  echo ""
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo "  CUE changes detected — running cue vet"
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  CHANGED_CUE_FILES=$(echo "$CHANGED_FILES" | grep '\.cue$' || true)
  if [ -n "$CHANGED_CUE_FILES" ]; then
    for cue_file in $CHANGED_CUE_FILES; do
      cue_dir=$(dirname "$cue_file")
      if [ -f "${cue_dir}/cue.mod/module.cue" ]; then
        if ! (cd "$cue_dir" && cue vet ./... 2>/dev/null); then
          echo "⚠ CUE vet found issues in $cue_dir (non-blocking in pre-push)"
        fi
      fi
    done
    echo "✓ CUE checks completed"
  fi
fi

echo ""
if [ $EXIT_CODE -eq 0 ]; then
  echo "✓ All pre-push checks passed"
else
  echo "✗ Pre-push checks failed — fix issues before pushing"
fi

exit $EXIT_CODE
