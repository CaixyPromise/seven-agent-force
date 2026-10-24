#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
server_dir="$root_dir/seven-agent-force-server"
report="$(mktemp)"
trap 'rm -f "$report"' EXIT

if ! command -v govulncheck >/dev/null 2>&1; then
  echo "govulncheck is required" >&2
  exit 2
fi
if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required to evaluate govulncheck JSON" >&2
  exit 2
fi

set +e
(cd "$server_dir" && govulncheck -json ./...) >"$report"
scan_status=$?
set -e
if [[ $scan_status -ne 0 && $scan_status -ne 3 ]]; then
  echo "govulncheck failed with exit code $scan_status" >&2
  exit "$scan_status"
fi

called_ids="$({
  jq -r 'select(.finding and (.finding.trace | length > 1)) | .finding.osv' "$report"
} | LC_ALL=C sort -u)"

unexpected=""
while IFS= read -r vulnerability_id; do
  [[ -z "$vulnerability_id" ]] && continue
  case "$vulnerability_id" in
    GO-2026-4883|GO-2026-4887)
      ;;
    *)
      unexpected+="${vulnerability_id}"$'\n'
      ;;
  esac
done <<<"$called_ids"

if [[ -n "$unexpected" ]]; then
  echo "unapproved reachable Go vulnerabilities:" >&2
  printf '%s' "$unexpected" >&2
  exit 1
fi

if grep -qx 'GO-2026-4883' <<<"$called_ids" || grep -qx 'GO-2026-4887' <<<"$called_ids"; then
  echo "accepted upstream scanner findings: GO-2026-4883, GO-2026-4887"
  echo "scope: Moby daemon plugin/AuthZ validation; this repository compiles only the Docker API client"
  echo "status: no fixed stable github.com/docker/docker release is available; review on every dependency update"
fi

echo "no unapproved reachable Go vulnerabilities found"
