#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat >&2 <<'USAGE'
usage: check-goreleaser-archive-version-metadata.sh <dist-dir> [--expected-version <version>] [--expected-channel <stable|prerelease|snapshot>] [--expected-provenance <value>]

Extracts each GoReleaser tar.gz archive in <dist-dir>, statically validates the
dotfiles-manager binary version metadata recorded in Go build metadata, and
executes the native host-matching archive binary when one is present.
USAGE
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

if [[ "$#" -lt 1 ]]; then
  usage
  exit 2
fi

dist_dir="$1"
shift
expected_version=""
expected_channel=""
expected_provenance=""

while [[ "$#" -gt 0 ]]; do
  case "$1" in
    --expected-version)
      expected_version="${2:-}"
      shift 2
      ;;
    --expected-channel)
      expected_channel="${2:-}"
      shift 2
      ;;
    --expected-provenance)
      expected_provenance="${2:-}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "error: unknown argument: $1" >&2
      usage
      exit 2
      ;;
  esac
done

if [[ ! -d "${dist_dir}" ]]; then
  echo "error: dist directory not found: ${dist_dir}" >&2
  exit 2
fi

if ! command -v go >/dev/null 2>&1; then
  echo "error: go is required to inspect cross-platform release binaries" >&2
  exit 2
fi

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
metadata_check="${script_dir}/check-release-version-metadata.sh"
host_goos="$(go env GOOS)"
host_goarch="$(go env GOARCH)"

shopt -s nullglob
archives=("${dist_dir}"/*.tar.gz)
shopt -u nullglob

if [[ "${#archives[@]}" -eq 0 ]]; then
  echo "error: no GoReleaser tar.gz archives found in ${dist_dir}" >&2
  exit 1
fi

checked=0
for archive in "${archives[@]}"; do
  (
    tmp="$(mktemp -d)"
    trap 'rm -rf "${tmp}"' EXIT
    tar -xzf "${archive}" -C "${tmp}"
    candidates=()
    while IFS= read -r candidate; do
      candidates+=("${candidate}")
    done < <(find "${tmp}" -type f -name dotfiles-manager | sort)
    if [[ "${#candidates[@]}" -ne 1 ]]; then
      echo "error: expected exactly one dotfiles-manager binary in ${archive}, got ${#candidates[@]}" >&2
      exit 1
    fi
    chmod +x "${candidates[0]}"
    metadata="$(go version -m "${candidates[0]}")"
    ldflags_line="$(printf '%s\n' "${metadata}" | grep -E '^[[:space:]]*build[[:space:]]+-ldflags=' || true)"
    if [[ -z "${ldflags_line}" ]]; then
      echo "error: no Go -ldflags build metadata found in ${archive}" >&2
      exit 1
    fi

    extract_ldflag() {
      local key="$1"
      local re="${key}=([^[:space:]\"]+)"
      if [[ "${ldflags_line}" =~ ${re} ]]; then
        printf '%s\n' "${BASH_REMATCH[1]}"
      fi
    }

    version="$(extract_ldflag 'buildVersion')"
    commit="$(extract_ldflag 'buildCommit')"
    date="$(extract_ldflag 'buildDate')"
    channel="$(extract_ldflag 'buildChannel')"
    provenance="$(extract_ldflag 'buildProvenance')"

    failures=()
    if [[ "${version}" == "dev" || -z "${version}" ]]; then
      failures+=("version must not be dev/empty")
    fi
    if [[ "${commit}" == "unknown" || ! "${commit}" =~ ^[0-9a-f]{40}$ ]]; then
      failures+=("commit must be a non-unknown 40-character git SHA")
    fi
    if [[ "${date}" == "unknown" || ! "${date}" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$ ]]; then
      failures+=("date must be a non-unknown UTC RFC3339 timestamp")
    fi
    case "${channel}" in
      stable|prerelease|snapshot) ;;
      dev|"")
        failures+=("channel must not be dev/empty")
        ;;
      *)
        failures+=("channel must be stable, prerelease, or snapshot")
        ;;
    esac
    if [[ "${provenance}" == "unspecified" || -z "${provenance}" ]]; then
      failures+=("provenance must not be unspecified/empty")
    fi

    if [[ -n "${expected_version}" && "${version}" != "${expected_version}" ]]; then
      failures+=("version ${version} does not match expected ${expected_version}")
    fi
    if [[ -n "${expected_channel}" && "${channel}" != "${expected_channel}" ]]; then
      failures+=("channel ${channel} does not match expected ${expected_channel}")
    fi
    if [[ -n "${expected_provenance}" && "${provenance}" != "${expected_provenance}" ]]; then
      failures+=("provenance ${provenance} does not match expected ${expected_provenance}")
    fi

    if [[ "${#failures[@]}" -gt 0 ]]; then
      printf 'error: release archive metadata check failed for %s\n' "${archive}" >&2
      printf 'ldflags: %s\n' "${ldflags_line}" >&2
      for failure in "${failures[@]}"; do
        printf ' - %s\n' "${failure}" >&2
      done
      exit 1
    fi

    printf 'release archive metadata ok: %s version=%s commit=%s date=%s channel=%s provenance=%s\n' \
      "${archive}" "${version}" "${commit}" "${date}" "${channel}" "${provenance}"

    metadata_goos="$(printf '%s\n' "${metadata}" | sed -n 's/^[[:space:]]*build[[:space:]]GOOS=//p' | head -n 1)"
    metadata_goarch="$(printf '%s\n' "${metadata}" | sed -n 's/^[[:space:]]*build[[:space:]]GOARCH=//p' | head -n 1)"
    if [[ "${metadata_goos}" == "${host_goos}" && "${metadata_goarch}" == "${host_goarch}" ]]; then
      native_args=()
      if [[ -n "${expected_version}" ]]; then
        native_args+=(--expected-version "${expected_version}")
      fi
      if [[ -n "${expected_channel}" ]]; then
        native_args+=(--expected-channel "${expected_channel}")
      fi
      if [[ -n "${expected_provenance}" ]]; then
        native_args+=(--expected-provenance "${expected_provenance}")
      fi
      bash "${metadata_check}" --binary "${candidates[0]}" "${native_args[@]}"
    fi
  )
  checked=$((checked + 1))
done

printf 'checked %d GoReleaser archive(s) for release version metadata\n' "${checked}"
