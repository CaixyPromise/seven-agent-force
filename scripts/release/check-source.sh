#!/usr/bin/env bash
# Read-only checks of committed source and full history; reports stay local.
set -euo pipefail
history_scope=all
if [[ "${1:-}" == "--candidate-only" && $# -eq 1 ]]; then
  history_scope=candidate
elif [[ $# -ne 0 ]]; then
  echo "usage: $0 [--candidate-only]" >&2
  exit 2
fi
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
report="$root/.local/open-source-check"
mkdir -p "$report"
command -v gitleaks >/dev/null
stage="$(mktemp -d)"
trap 'rm -rf "$stage"' EXIT
git_root="$(git -C "$root" rev-parse --show-toplevel 2>/dev/null || true)"
if [[ "$git_root" == "$root" ]]; then
  git -C "$root" archive HEAD | tar -x -C "$stage"
else
  # Archive mode scans source files directly; generated dependencies and reports
  # are excluded, but source configuration and fixtures remain in scope.
  python3 - "$root" "$stage" <<'COPY'
import os,shutil,sys
from pathlib import Path
root,stage=map(Path,sys.argv[1:])
ignored={'.git','.local','node_modules','dist','__pycache__','.venv'}
for directory,dirs,files in os.walk(root):
    for name in dirs+files:
        if (Path(directory)/name).is_symlink():
            raise ValueError('archive source must not contain symlinks')
    dirs[:]=[x for x in dirs if x not in ignored]
    for name in files:
        src=Path(directory)/name;dst=stage/src.relative_to(root)
        dst.parent.mkdir(parents=True,exist_ok=True);shutil.copy2(src,dst)
COPY
fi
status=0
scan() {
  local scope="$1" code=0
  shift
  rm -f "$report/$scope.json" "$report/$scope.review.json"
  "$@" >"$report/$scope.log" 2>&1 || code=$?
  if [[ "$code" -ne 0 && "$code" -ne 1 ]]; then
    echo "$scope scanner execution failed ($code); see local log" >&2
    return 1
  fi
  python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); assert isinstance(d,list) and ((int(sys.argv[2])==1)==bool(d)), "inconsistent scanner exit/report"' "$report/$scope.json" "$code" || return 1
  python3 "$root/scripts/release/review_secrets.py" "$scope" "$report/$scope.json" \
    --root "$root" --source-root "$stage"
}
scan source gitleaks dir "$stage" --config "$root/.gitleaks.toml" --no-banner --redact \
  --report-format json --report-path "$report/source.json" || status=1
if [[ "$git_root" == "$root" ]]; then
scan history gitleaks git "$root" --log-opts="--all" --config "$root/.gitleaks.toml" --no-banner --redact \
  --report-format json --report-path "$report/history.json" || status=1
# Names only: never print matching lines containing private information.
git -C "$root" ls-tree -r --name-only HEAD | \
  grep -E '(^|/)(admin-pwd\.txt|application\.yaml)$|(^|/)\.env($|\.)|^\.local/|\.(pem|key|dump)$|auth\.png$' | grep -vE '(^|/)\.env\.example$' \
  >"$report/forbidden-paths.txt" && status=1
python3 "$root/scripts/release/check_history_files.py" --root "$root" --report "$report/history-files.json" --scope "$history_scope" || status=1
else
  echo 'History: NOT_APPLICABLE (independent source archive has no Git history)'
  find "$stage" -type f | sed "s|$stage/||" | grep -E '(^|/)(admin-pwd\.txt|application\.yaml)$|(^|/)\.env($|\.)|^\.local/|\.(pem|key|dump)$|auth\.png$' | grep -vE '(^|/)\.env\.example$' >"$report/forbidden-paths.txt" && status=1
fi
rg -l '/Users/[^/]+/|/home/[^/]+/' "$stage" >"$report/local-paths.txt" || true
printf 'Reports: %s\n' "$report"
printf 'Secret/path gate exit: %s (local-paths also requires human privacy review)\n' "$status"
exit "$status"
