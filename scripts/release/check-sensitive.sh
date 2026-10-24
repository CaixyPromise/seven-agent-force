#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: $0 <directory>" >&2
  exit 2
fi

target="$1"
root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
if [[ ! -d "$target" ]]; then
  echo "scan target is not a directory: $target" >&2
  exit 2
fi

forbidden_names="$({
  find "$target" -type f \( \
    -name 'admin-pwd.txt' -o \
    -name '*-auth.png' -o \
    -name '.env' -o \
    -name '.env.*' -o \
    -name 'application.yaml' -o \
    -name 'application-*.yaml' -o \
    -name '*.pem' -o \
    -name '*.key' -o \
    -name '*.p12' -o \
    -name '*.pfx' -o \
    -name '*.sql.gz' -o \
    -name '*.dump' \
  \) -print
} || true)"
if [[ -n "$forbidden_names" ]]; then
  echo "forbidden sensitive files found:" >&2
  echo "$forbidden_names" >&2
  exit 1
fi

if ! command -v gitleaks >/dev/null 2>&1; then
  echo "gitleaks is required for release scanning" >&2
  exit 1
fi
gitleaks dir "$target" --config "$root_dir/.gitleaks.toml" --no-banner --redact
