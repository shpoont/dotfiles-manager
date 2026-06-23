#!/usr/bin/env bash
set -euo pipefail

HAS_GO_MOD="${1:-$(bash scripts/ci/check-go-module.sh)}"

GO_FILES=()
while IFS= read -r file; do
  GO_FILES+=("$file")
done < <(find . -type f -name '*.go' -not -path './vendor/*' -not -path './.git/*' | sort)

if ((${#GO_FILES[@]} > 0)); then
  echo "Running gofmt check..."
  UNFORMATTED="$(gofmt -l "${GO_FILES[@]}")"
  if [[ -n "$UNFORMATTED" ]]; then
    echo "Found unformatted Go files:" >&2
    echo "$UNFORMATTED" >&2
    exit 1
  fi
else
  echo "No Go files found; gofmt check skipped."
fi

echo "Checking v2 status/diff fixtures..."
scripts/ci/check-v2-status-diff-fixtures.sh

echo "Checking v2 smart-sync planning fixtures..."
scripts/ci/check-v2-smart-sync-planning-fixtures.sh

if [[ "$HAS_GO_MOD" != "true" ]]; then
  echo "No go.mod detected; skipping go vet/staticcheck/golangci-lint."
  exit 0
fi

export PATH="$(go env GOPATH)/bin:$PATH"

echo "Downloading Go modules..."
go mod download

echo "Running go vet..."
go vet ./...

echo "Running staticcheck..."
staticcheck ./...

echo "Running golangci-lint..."
golangci-lint run ./...
