#!/usr/bin/env bash
set -euo pipefail

if [[ -f "go.mod" && -f ".goreleaser.yml" ]]; then
  echo "true"
else
  echo "false"
fi
