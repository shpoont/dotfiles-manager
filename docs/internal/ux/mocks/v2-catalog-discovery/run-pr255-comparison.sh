#!/usr/bin/env bash
set -euo pipefail

# Build and run the current checked-out dotfiles-manager implementation against
# the runnable #228 catalog-discovery UX mock scenarios. This script uses only
# temporary settings/home/catalog fixtures and does not touch live user settings,
# the user's real settings storage folder, or real user catalog folders.

ROOT_DIR="$(git rev-parse --show-toplevel)"
GO_BIN="${GO:-${HOME}/.asdf/shims/go}"
# Keep this evidence run offline by default. If module dependencies are not
# already cached, fail rather than silently fetching from the network. Set
# DFM_PR255_ALLOW_GO_NETWORK=1 only for an explicit non-gate convenience rerun.
if [[ "${DFM_PR255_ALLOW_GO_NETWORK:-}" != "1" ]]; then
  export GOPROXY=off
  export GOSUMDB=off
fi
TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/dfm-pr255-compare.XXXXXX")"
trap 'rm -rf "${TMP_DIR}"' EXIT
REAL_TMP_DIR="$(cd "${TMP_DIR}" && pwd -P)"
BIN="${TMP_DIR}/dotfiles-manager"
SETTINGS_DIR="${TMP_DIR}/settings"
HOME_DIR="${TMP_DIR}/home"
CATALOG_DIR="${TMP_DIR}/catalog"
BROKEN_DIR="${TMP_DIR}/broken-recipes"

normalize_paths() {
  sed \
    -e "s|${REAL_TMP_DIR}|<tmp>|g" \
    -e "s|${TMP_DIR}|<tmp>|g" \
    -e "s|/private${REAL_TMP_DIR}|<tmp>|g" \
    -e "s|/private${TMP_DIR}|<tmp>|g"
}

mkdir -p \
  "${SETTINGS_DIR}/profiles/stacks" \
  "${SETTINGS_DIR}/profiles/layers" \
  "${HOME_DIR}" \
  "${CATALOG_DIR}/example-tool" \
  "${CATALOG_DIR}/git" \
  "${BROKEN_DIR}/broken-tool"

cat > "${SETTINGS_DIR}/dotfiles-manager.v2.yaml" <<'YAML'
schema: dotfiles-manager.v2.root-config
schemaVersion: 1
activeProfileStack: default
YAML
cat > "${SETTINGS_DIR}/profiles/stacks/default.yaml" <<'YAML'
schema: dotfiles-manager.v2.profile-stack
schemaVersion: 1
profileStack:
  - global
YAML
cat > "${SETTINGS_DIR}/profiles/layers/global.yaml" <<'YAML'
schema: dotfiles-manager.v2.profile-layer
schemaVersion: 1
selections:
  example-tool:
    settings:
      config:
        scope: user
YAML
cat > "${CATALOG_DIR}/example-tool/recipe.yaml" <<'YAML'
schema: dotfiles-manager.v2.recipe
schemaVersion: 1
target: example-tool
displayName: Example Tool
supportLevel: experimental
capability: read-write
locations:
  config:
    default: ~/.example-tool
settings:
  config:
    label: Example Tool setting
    supportLevel: experimental
    capability: read-write
    artifactForm: scalar
    scopeDefault: user
    resource: config-resource
resources:
  config-resource:
    driver: yaml-file
    location: config
    path: config.yaml
    selector:
      path: [user, email]
      createMissing: create
      duplicatePolicy: reject
YAML
cat > "${CATALOG_DIR}/git/recipe.yaml" <<'YAML'
schema: dotfiles-manager.v2.recipe
schemaVersion: 1
target: git
displayName: Git Custom
supportLevel: experimental
capability: read-write
locations:
  config:
    default: ~/.git-custom
settings:
  user.email:
    label: Git Custom setting
    supportLevel: experimental
    capability: read-write
    artifactForm: scalar
    scopeDefault: user
    resource: config-resource
resources:
  config-resource:
    driver: yaml-file
    location: config
    path: config.yaml
    selector:
      path: [user, email]
      createMissing: create
      duplicatePolicy: reject
YAML
cat > "${BROKEN_DIR}/broken-tool/recipe.yaml" <<'YAML'
schema: dotfiles-manager.v2.recipe
schemaVersion: 1
target: broken-tool
displayName: Broken Tool
supportLevel: experimental
capability: read-write
unexpectedField: inert-invalid-fixture
locations:
  config:
    default: ~/.broken
settings:
  config:
    scopeDefault: user
    resource: config-file
resources:
  config-file:
    driver: yaml-file
    location: config
    path: config.yaml
    selector:
      path: [user, email]
YAML

(cd "${ROOT_DIR}" && "${GO_BIN}" build -o "${BIN}" ./cmd/dotfiles-manager)

run_command() {
  local cmd_status=0
  {
    printf '$ dotfiles-manager'
    printf ' %q' "$@"
    printf '\n'
  } | normalize_paths
  (cd "${SETTINGS_DIR}" && HOME="${HOME_DIR}" "${BIN}" "$@") 2>&1 | normalize_paths || cmd_status=${PIPESTATUS[0]}
  if [[ ${cmd_status} -ne 0 ]]; then
    printf '[exit %d]\n' "${cmd_status}"
  fi
  printf '\n'
}

cat <<HEADER
# PR #255 implementation comparison transcript
# Work item: #228
# Binary: go build ./cmd/dotfiles-manager at $(git -C "${ROOT_DIR}" rev-parse --short HEAD)
# Branch: $(git -C "${ROOT_DIR}" branch --show-current)
# Go network: GOPROXY=${GOPROXY}, GOSUMDB=${GOSUMDB}
# Safety: temporary settings/home/catalog fixtures only; no live/user settings storage or real user catalog folders are touched; paths normalized to <tmp>.

HEADER

run_command list
run_command search git
run_command explain git
run_command catalog list
run_command catalog add "${BROKEN_DIR}" --name broken
run_command catalog add "${CATALOG_DIR}" --name personal
run_command list
run_command explain git
run_command explain example-tool
run_command sync example-tool --non-interactive --user-id leon
run_command catalog disable personal
run_command list
run_command status example-tool --user-id leon
run_command catalog enable personal
run_command catalog remove personal
run_command catalog add shpoont/custom-recipes

cat <<'CHECK'
# Additional command-shape check against the storyboard/mock command:
CHECK
run_command catalog add "${CATALOG_DIR}" --name personal-again
run_command sync --dry-run example-tool --user-id leon
