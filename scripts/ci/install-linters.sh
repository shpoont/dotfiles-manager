#!/usr/bin/env bash
set -euo pipefail

if [[ "$(bash scripts/ci/check-go-module.sh)" != "true" ]]; then
  echo "No go.mod detected; skipping linter installation."
  exit 0
fi

echo "Installing staticcheck and golangci-lint..."
go install honnef.co/go/tools/cmd/staticcheck@latest
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

if [[ -n "${GITHUB_PATH:-}" ]]; then
  echo "$(go env GOPATH)/bin" >> "$GITHUB_PATH"
fi

export PATH="$(go env GOPATH)/bin:$PATH"
staticcheck -version
golangci-lint --version
