#!/usr/bin/env bash
set -euo pipefail

if [[ -f "go.mod" ]]; then
  echo "true"
else
  echo "false"
fi
