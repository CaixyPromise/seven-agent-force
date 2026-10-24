#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root_dir/seven-agent-force-server"

go test ./internal/app/hub_control \
  -run '^TestHubControlDDDImportBoundaries$' -count=1
go test ./internal/app/system/cache_governance/application \
  -run '^TestDG5InfrastructureDependsOnlyOnSharedProtocolContracts$' -count=1
