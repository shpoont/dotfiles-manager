#!/usr/bin/env bash
set -euo pipefail

SHARD="${1:?usage: docker-shard.sh <shard> [has_go_mod]}"
HAS_GO_MOD="${2:-$(bash scripts/ci/check-go-module.sh)}"

docker run --rm \
  -u "$(id -u):$(id -g)" \
  -v "$PWD:/workspace" \
  -w /workspace \
  -e PATH="/usr/local/go/bin:/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin" \
  -e HOME="/workspace/.ci-home" \
  -e GOCACHE="/workspace/.ci-home/.cache/go-build" \
  -e GOMODCACHE="/workspace/.ci-home/pkg/mod" \
  -e CI=true \
  -e HAS_GO_MOD="$HAS_GO_MOD" \
  golang:1.22 \
  bash -lc "mkdir -p \"\$HOME\" \"\$GOCACHE\" \"\$GOMODCACHE\" && bash scripts/ci/run-tests.sh '$SHARD' linux '$HAS_GO_MOD'"
