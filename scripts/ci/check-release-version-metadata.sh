#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat >&2 <<'USAGE'
usage: check-release-version-metadata.sh --binary <path> [--expected-version <version>] [--expected-channel <stable|prerelease|snapshot>] [--expected-provenance <value>]

Validates that a release-built dotfiles-manager binary reports enriched version
metadata and does not fall back to local-dev placeholders.
USAGE
}

binary=""
expected_version=""
expected_channel=""
expected_provenance=""

while [[ "$#" -gt 0 ]]; do
  case "$1" in
    --binary)
      binary="${2:-}"
      shift 2
      ;;
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

if [[ -z "${binary}" ]]; then
  echo "error: --binary is required" >&2
  usage
  exit 2
fi
if [[ ! -x "${binary}" ]]; then
  echo "error: binary is not executable: ${binary}" >&2
  exit 2
fi

version_output="$("${binary}" version | tr -d '\r')"
if [[ ! "${version_output}" =~ ^dotfiles-manager[[:space:]]version=([^[:space:]]+)[[:space:]]commit=([^[:space:]]+)[[:space:]]date=([^[:space:]]+)[[:space:]]channel=([^[:space:]]+)[[:space:]]provenance=([^[:space:]]+)$ ]]; then
  echo "error: unexpected version output format: ${version_output}" >&2
  exit 1
fi

version="${BASH_REMATCH[1]}"
commit="${BASH_REMATCH[2]}"
date="${BASH_REMATCH[3]}"
channel="${BASH_REMATCH[4]}"
provenance="${BASH_REMATCH[5]}"

failures=()
if [[ "${version}" == "dev" || -z "${version}" ]]; then
  failures+=("version must not be dev/empty")
fi
if [[ "${commit}" == "unknown" || ! "${commit}" =~ ^[0-9a-f]{7,40}$ ]]; then
  failures+=("commit must be a non-unknown git SHA")
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
  printf 'error: release version metadata check failed for %s\n' "${binary}" >&2
  printf 'version output: %s\n' "${version_output}" >&2
  for failure in "${failures[@]}"; do
    printf ' - %s\n' "${failure}" >&2
  done
  exit 1
fi

printf 'release version metadata ok: %s\n' "${version_output}"
