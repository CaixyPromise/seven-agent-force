#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# A release's source identity must describe the actual tracked build input.
if ! git -C "$root_dir" diff --quiet || ! git -C "$root_dir" diff --cached --quiet; then
  echo "commit tracked changes before building a release" >&2
  exit 1
fi
version="${1:-dev}"
shift "$(( $# > 0 ? 1 : 0 ))"
ci_run=""
if [[ $# -gt 0 ]]; then
  if [[ $# -ne 2 || "$1" != "--verified-ci-run" ]]; then
    echo "usage: build.sh VERSION [--verified-ci-run RUN_ID]" >&2
    exit 2
  fi
  ci_run="$2"
fi
if [[ ! "$version" =~ ^[0-9A-Za-z][0-9A-Za-z._-]*$ ]]; then
  echo "invalid version: use only letters, numbers, dots, underscores, and hyphens" >&2
  exit 2
fi
goos="${GOOS:-$(go env GOOS)}"
goarch="${GOARCH:-$(go env GOARCH)}"
# GOOS/GOARCH describe the release target, but any Go-based build tools must
# still be installed for the host runner. The service build receives the
# target values explicitly below.
unset GOOS GOARCH
commit="$(git -C "$root_dir" rev-parse HEAD)"
build_date="${BUILD_DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
module_path="github.com/CaixyPromise/seven-agent/force"
package_name="seven-agent-force_${version}_${goos}_${goarch}"
release_root="$root_dir/dist/release"
stage="$release_root/$package_name"
archive="$release_root/$package_name.tar.gz"

if [[ "$stage" != "$release_root"/seven-agent-force_* ]]; then
  echo "refusing unsafe release stage: $stage" >&2
  exit 1
fi
if [[ -e "$stage" ]]; then
  rm -r "$stage"
fi
mkdir -p "$stage/bin" "$stage/configs" "$stage/migrations" "$stage/web" "$stage/deploy/nginx"

if [[ -n "$ci_run" ]]; then
  python3 "$root_dir/scripts/release/verified_checks.py" --run "$ci_run" --output "$stage/checks.json"
fi

cd "$root_dir/seven-agent-force-web"
pnpm install --frozen-lockfile
if [[ -z "$ci_run" ]]; then
  pnpm audit --prod --audit-level high
  pnpm exec tsc -b
  pnpm lint
fi
pnpm build

cd "$root_dir/seven-agent-force-server"
if [[ -z "$ci_run" ]]; then
if ! command -v govulncheck >/dev/null 2>&1; then
  GOTOOLCHAIN="$(go env GOVERSION)" GOBIN="$root_dir/.local/bin" go install golang.org/x/vuln/cmd/govulncheck@v1.1.4
  export PATH="$root_dir/.local/bin:$PATH"
fi
"$root_dir/scripts/check-go-vulnerabilities.sh"
fi
for command in seven-agent-force-server seven-agent-force-mcp-server seven-worker seven-workerctl seven-worker-cli seven-provider-proxy; do
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go build -trimpath \
    -ldflags "-s -w -X '${module_path}/internal/shared/buildinfo.Version=${version}' -X '${module_path}/internal/shared/buildinfo.Commit=${commit}' -X '${module_path}/internal/shared/buildinfo.BuildDate=${build_date}'" \
    -o "$stage/bin/$command" "./cmd/$command"
done

cp "$root_dir/seven-agent-force-server/configs/application.example.yaml" "$stage/configs/"
cp "$root_dir/seven-agent-force-server/configs/mcp.example.yaml" "$stage/configs/"
cp -R "$root_dir/seven-agent-force-server/migrations/mysql" "$stage/migrations/"
cp -R "$root_dir/seven-agent-force-server/migrations/postgres" "$stage/migrations/"
cp -R "$root_dir/seven-agent-force-server/migrations/postgres-baseline" "$stage/migrations/"
mkdir -p "$stage/schema"
cp "$root_dir/seven-agent-force-server/schema/postgres.sql" "$stage/schema/"
cp -R "$root_dir/seven-agent-force-web/dist" "$stage/web/"
cp "$root_dir/deploy/nginx/seven-framework.conf.example" "$stage/deploy/nginx/"
mkdir -p "$stage/docs/workforce" "$stage/deploy/workforce/runtime-sdk" "$stage/metadata"
cp "$root_dir/seven-agent-force-server/go.mod" "$root_dir/seven-agent-force-server/go.sum" "$stage/metadata/"
cp "$root_dir/seven-agent-force-web/package.json" "$root_dir/seven-agent-force-web/pnpm-lock.yaml" "$stage/metadata/"
cp "$root_dir/LICENSE" "$stage/deploy/workforce/runtime-sdk/"
cp "$root_dir/docs/deployment.md" "$root_dir/docs/database-schema.md" "$root_dir/docs/database-storage-contract.md" "$root_dir/docs/migration.md" "$stage/docs/"
cp "$root_dir/docs/workforce/mvp-runbook.md" "$root_dir/docs/workforce/worker-container.md" "$stage/docs/workforce/"
# Explicit source allowlist: never recursively copy the local deployment tree.
# Dockerfiles require the corresponding source checkout as their build context;
# they are documented in the manifest, not shipped as standalone runnable images.
printf 'version=%s\ncommit=%s\nos=%s\narch=%s\nruntime_images=separately-built-and-admitted-by-digest\n' \
  "$version" "$commit" "$goos" "$goarch" > "$stage/RELEASE.txt"
cp "$root_dir/deploy/workforce/runtime-sdk/pyproject.toml" "$stage/deploy/workforce/runtime-sdk/"
cp "$root_dir/deploy/workforce/runtime-sdk/README.md" "$stage/deploy/workforce/runtime-sdk/"
mkdir -p "$stage/deploy/workforce/runtime-sdk/src"
for package in seven_agent seven_workforce_runtime; do
  cp -R "$root_dir/deploy/workforce/runtime-sdk/src/$package" "$stage/deploy/workforce/runtime-sdk/src/"
done
find "$stage/deploy/workforce/runtime-sdk" -type d -name __pycache__ -prune -exec rm -r {} +
cp "$root_dir/LICENSE" "$root_dir/NOTICE" "$root_dir/README.md" "$stage/"

if command -v syft >/dev/null 2>&1; then
  syft scan "dir:$stage" --source-name "$package_name" --source-version "$version" -o "spdx-json=$stage/sbom.spdx.json"
else
  GOBIN="$root_dir/.local/bin" go install github.com/anchore/syft/cmd/syft@v1.31.0
  "$root_dir/.local/bin/syft" scan "dir:$stage" --source-name "$package_name" --source-version "$version" -o "spdx-json=$stage/sbom.spdx.json"
fi

python3 "$root_dir/scripts/release/check_licenses.py" --sbom "$stage/sbom.spdx.json" --stage "$stage"

"$root_dir/scripts/release/check-sensitive.sh" "$stage"

cd "$stage"
find . -type f ! -name SHA256SUMS -print | LC_ALL=C sort | while IFS= read -r file; do
  shasum -a 256 "$file"
done > SHA256SUMS
shasum -a 256 -c SHA256SUMS

cd "$release_root"
rm -f "$archive"
tar -czf "$archive" "$package_name"
shasum -a 256 "$package_name.tar.gz" > "$package_name.tar.gz.sha256"
echo "$archive"
