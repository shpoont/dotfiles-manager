#!/usr/bin/env bash
set -euo pipefail

SHARD="${1:?usage: docker-shard.sh <shard> [has_go_mod]}"
HAS_GO_MOD="${2:-$(bash scripts/ci/check-go-module.sh)}"

docker run --rm \
  -u "$(id -u):$(id -g)" \
  -v "$PWD:/workspace" \
  -w /workspace \
  -e CI=true \
  -e HAS_GO_MOD="$HAS_GO_MOD" \
  golang:1.22 \
  bash -lc "bash scripts/ci/run-tests.sh '$SHARD' linux '$HAS_GO_MOD'"
