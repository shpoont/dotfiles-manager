#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CASE_ROOT="$ROOT/cases"
CASES=(
  issue-quality-v2-specs
  happy-path-ux-v2-cli
  recipe-explain-clarity
  native-safety-review
)

fail() {
  echo "validate failed: $*" >&2
  exit 1
}

require_file() {
  local path="$1"
  [[ -f "$path" ]] || fail "missing file: $path"
}

require_readme_text() {
  local path="$1"
  local needle="$2"
  grep -Fq -- "$needle" "$path" || fail "README missing '$needle': $path"
}

find_toml_python() {
  local candidates=()
  if [[ -n "${PYTHON:-}" ]]; then
    candidates+=("$PYTHON")
  fi
  candidates+=("$HOME/.asdf/shims/python" python3 python)

  local candidate
  for candidate in "${candidates[@]}"; do
    if command -v "$candidate" >/dev/null 2>&1; then
      if "$candidate" - <<'TOMLPROBE' >/dev/null 2>&1; then
try:
    import tomllib
except ModuleNotFoundError:
    import tomli as tomllib
TOMLPROBE
        printf '%s\n' "$candidate"
        return 0
      fi
    fi
  done
  return 1
}

parse_toml() {
  local path="$1"
  local py
  py="$(find_toml_python)" || fail "Python with tomllib/tomli is required to parse TOML: $path"
  "$py" - "$path" <<'TOMLPY'
import sys
try:
    import tomllib
except ModuleNotFoundError:
    import tomli as tomllib
path = sys.argv[1]
with open(path, 'rb') as f:
    tomllib.load(f)
TOMLPY
}

check_no_local_private_artifacts() {
  local found=()
  local path

  while IFS= read -r path; do
    found+=("$path")
  done < <(
    find "$ROOT" \
      \( \
        \( -type d \( \
            -name jobs -o \
            -name all-jobs -o \
            -name results -o \
            -name dashboard -o \
            -name output -o \
            -name tmp -o \
            -name local-auth -o \
            -name local-build -o \
            -name local-output \
          \) \) -o \
        \( -type f \( \
            -name auth.json -o \
            -name .harbor-auth-prepared -o \
            -path '*/environment/codex-auth/config.toml' -o \
            -name '*.jsonl' -o \
            -iname 'transcript*' \
          \) \) \
      \) \
      -print | sort
  )

  if ((${#found[@]} > 0)); then
    echo "local-private/generated Harbor artifacts are present:" >&2
    printf '  %s\n' "${found[@]#$ROOT/}" >&2
    fail "clean generated Harbor outputs and copied auth before validation/commit"
  fi
}

[[ -d "$CASE_ROOT" ]] || fail "missing cases directory: $CASE_ROOT"
check_no_local_private_artifacts

for case in "${CASES[@]}"; do
  dir="$CASE_ROOT/$case"
  [[ -d "$dir" ]] || fail "missing case directory: $dir"

  for rel in \
    README.md \
    instruction.md \
    task.toml \
    fixture/context.md \
    fixture/sample-pass.md \
    fixture/sample-fail.md \
    environment/Dockerfile \
    environment/.dockerignore \
    environment/docker-compose.yaml \
    environment/codex-auth/README.md \
    environment/codex-auth/config.example.toml \
    environment/context/context.md \
    tests/objective.sh \
    tests/test.sh \
    tests/review.toml
  do
    require_file "$dir/$rel"
  done

  require_readme_text "$dir/README.md" "## Relevant v2 specs"
  require_readme_text "$dir/README.md" "## User or reviewer risk evaluated"
  require_readme_text "$dir/README.md" "## Passing answer includes"
  require_readme_text "$dir/README.md" "## Failing answer looks like"
  require_readme_text "$dir/README.md" "## Deterministic tests complemented"
  require_readme_text "$dir/README.md" "## Out of scope"

  bash -n "$dir/tests/objective.sh"
  bash -n "$dir/tests/test.sh"
  parse_toml "$dir/task.toml"
  parse_toml "$dir/tests/review.toml"
  grep -Fq "Committed files in this directory are non-secret" "$dir/environment/codex-auth/README.md" || \
    fail "codex-auth/README.md must document committed files as non-secret for $case"
  grep -Fq "real \`auth.json\` and generated \`config.toml\`" "$dir/environment/codex-auth/README.md" || \
    fail "codex-auth/README.md must document generated local-only auth/config handling for $case"
  grep -Fq "Non-secret Codex defaults" "$dir/environment/codex-auth/config.example.toml" || \
    fail "codex-auth/config.example.toml must document that it is non-secret for $case"
  grep -Fq "must never be committed or baked into Docker images" "$dir/environment/codex-auth/config.example.toml" || \
    fail "codex-auth/config.example.toml must document local-only auth/config handling for $case"
  cmp -s "$dir/fixture/context.md" "$dir/environment/context/context.md" || \
    fail "environment context does not match fixture context for $case"
  grep -Fq "codex-auth/auth.json" "$dir/environment/.dockerignore" || \
    fail "environment .dockerignore must exclude codex-auth/auth.json for $case"
  grep -Fq "codex-auth/" "$dir/environment/.dockerignore" || \
    fail "environment .dockerignore must exclude the full codex-auth source for $case"
  if grep -Eq "COPY +codex-auth|ADD +codex-auth|auth\.json" "$dir/environment/Dockerfile"; then
    fail "Dockerfile must not copy or reference Codex auth for $case"
  fi
  grep -Fq "CODEX_HOME=/root/.codex-agent" "$dir/environment/Dockerfile" || \
    fail "Dockerfile must keep agent CODEX_HOME away from verifier auth mount for $case"
  grep -Fq "source: ./codex-auth" "$dir/environment/docker-compose.yaml" || \
    fail "docker-compose must mount local codex-auth directory for $case"
  grep -Fq "target: /run/codex-auth-source" "$dir/environment/docker-compose.yaml" || \
    fail "docker-compose must mount verifier Codex auth source away from CODEX_HOME for $case"
  grep -Fq "read_only: true" "$dir/environment/docker-compose.yaml" || \
    fail "docker-compose auth source mount must be read-only for $case"
  grep -Fq "CODEX_AUTH_SOURCE" "$dir/task.toml" || \
    fail "task verifier env must name CODEX_AUTH_SOURCE for $case"
  grep -Fq "VERIFIER_CODEX_HOME" "$dir/task.toml" || \
    fail "task verifier env must name VERIFIER_CODEX_HOME for $case"
  grep -Fq 'AUTH_SOURCE="${CODEX_AUTH_SOURCE:-/run/codex-auth-source}"' "$dir/tests/test.sh" || \
    fail "test.sh must read verifier auth from a separate runtime source for $case"
  grep -Fq 'VERIFIER_CODEX_HOME="${VERIFIER_CODEX_HOME:-/tmp/verifier-codex-home}"' "$dir/tests/test.sh" || \
    fail "test.sh must use a writable temporary verifier CODEX_HOME for $case"
  grep -Fq "trap cleanup_verifier_codex_home EXIT" "$dir/tests/test.sh" || \
    fail "test.sh must clean temporary verifier CODEX_HOME for $case"
  grep -Fq 'cp "$AUTH_SOURCE/auth.json" "$VERIFIER_CODEX_HOME/auth.json"' "$dir/tests/test.sh" || \
    fail "test.sh must copy auth only into the temporary verifier CODEX_HOME for $case"
  if grep -Eq "cat .*auth\.json|echo .*auth\.json|sed .*auth\.json" "$dir/tests/test.sh"; then
    fail "test.sh must not print or inspect auth.json contents for $case"
  fi
  if [[ "$case" == "native-safety-review" ]]; then
    grep -Fq "arbitrary command execution" "$dir/tests/review.toml" || \
      fail "native safety rubric must penalize arbitrary command execution"
    grep -Fq "secret/account-data capture" "$dir/tests/review.toml" || \
      fail "native safety rubric must penalize secret/account-data capture"
    grep -Fq "opaque or oversized payloads" "$dir/tests/review.toml" || \
      fail "native safety rubric must penalize opaque or oversized diffs/payloads"
    grep -Fq "lifecycle stop/reopen side effects" "$dir/tests/review.toml" || \
      fail "native safety rubric must penalize lifecycle stop/reopen side effects"
    grep -Fq "native opt-in bypasses" "$dir/tests/review.toml" || \
      fail "native safety rubric must penalize native opt-in bypasses"
    grep -Fq "unsafe just-run-it" "$dir/tests/review.toml" || \
      fail "native safety rubric must penalize unsafe just-run-it recommendations"
  fi

  "$dir/tests/objective.sh" "$dir/fixture/sample-pass.md" >/dev/null
  if "$dir/tests/objective.sh" "$dir/fixture/sample-fail.md" >/dev/null 2>&1; then
    fail "objective script should reject sample-fail.md for $case"
  fi
done

echo "ok: validated ${#CASES[@]} Harbor case scaffold(s)"
