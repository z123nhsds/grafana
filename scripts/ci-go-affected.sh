#!/usr/bin/env bash
set -e

# Extract directories for changed Go files
# Using grep to filter .go files and dirname to get unique directories
DIRS=$(for file in "$@"; do echo "$file"; done | grep '\.go$' | xargs -r -n1 dirname | sort -u)

if [ -z "$DIRS" ]; then
  echo "No Go files changed. Skipping Go tests."
  exit 0
fi

echo "Affected Go directories:"
echo "$DIRS"

# For each directory, run tests
for dir in $DIRS; do
  if [ -d "$dir" ]; then
    echo "Running go test for $dir..."
    go test "./$dir/..." || exit 1
  fi
done
