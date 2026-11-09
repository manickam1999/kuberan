#!/usr/bin/env bash
set -euo pipefail

if [ $# -ne 1 ]; then
  echo "Usage: $0 <module-directory>"
  echo "Example: $0 apps/api"
  exit 1
fi

MODULE_DIR="$1"

if [ ! -f "$MODULE_DIR/go.mod" ]; then
  echo "Error: $MODULE_DIR/go.mod not found. Is this a Go module?"
  exit 1
fi

cd "$MODULE_DIR"

echo "=== Checking $(head -1 go.mod | awk '{print $2}') ==="
echo ""

echo "=== Step 1/5: go build ==="
go build ./...

echo "=== Step 2/5: go vet ==="
go vet ./...

echo "=== Step 3/5: golangci-lint ==="
golangci-lint run ./...

echo "=== Step 4/5: go test ==="
go test ./... -count=1 -timeout 600s

echo "=== Step 5/5: go test -race ==="
go test ./... -race -count=1 -timeout 600s

echo ""
echo "All checks passed."
