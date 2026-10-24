SHELL := /bin/bash

ROOT_DIR := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))
SERVER_DIR := $(ROOT_DIR)seven-agent-force-server
WEB_DIR := $(ROOT_DIR)seven-agent-force-web
VERSION ?= dev
SOURCE_COMMIT ?= source-archive
FAKE_HARNESS_VERSION ?= 0.6.0-packaging-delivery
FAKE_HARNESS_TAG ?= seven-agent-force/fake-harness:$(FAKE_HARNESS_VERSION)
FAKE_HARNESS_BUILD_IMAGE ?= golang:1.26.10-alpine
FAKE_HARNESS_RUNTIME_IMAGE ?= alpine:3.22
FAKE_HARNESS_BATCH ?= batch2
PROVIDER_PROXY_VERSION ?= 0.2.4-provider-gateway
PROVIDER_PROXY_TAG ?= seven-agent-force/provider-proxy:$(PROVIDER_PROXY_VERSION)
CODEX_HARNESS_VERSION ?= 0.3.0-packaging-delivery
CODEX_HARNESS_TAG ?= seven-agent-force/codex-harness:$(CODEX_HARNESS_VERSION)
CODEX_HARNESS_GO_PROXY ?= https://proxy.golang.org,direct
CODEX_HARNESS_RUNTIME_IMAGE ?= node:22-bookworm-slim
CLAUDE_HARNESS_VERSION ?= 0.1.0-model-credential
CLAUDE_HARNESS_TAG ?= seven-agent-force/claude-harness:$(CLAUDE_HARNESS_VERSION)
CLAUDE_HARNESS_GO_PROXY ?= https://proxy.golang.org,direct
CLAUDE_HARNESS_RUNTIME_IMAGE ?= node:22-bookworm-slim

.PHONY: help server-check server-build-mcp web-install web-check check build fake-harness-image provider-proxy-image codex-harness-image claude-harness-image release security-scan clean

help:
	@echo "Seven Agent Force monorepo"
	@echo "  make server-check   Format, vet, and test the Go service"
	@echo "  make server-build-mcp  Build the stateless MCP server"
	@echo "  make web-install    Install frozen frontend dependencies"
	@echo "  make web-check      Type-check, lint, and build the Web app"
	@echo "  make check          Run server and Web checks"
	@echo "  make release        Build a local release package"
	@echo "  make provider-proxy-image  Build the private Responses proxy image"
	@echo "  make fake-harness-image    Build the Fleet Supervisor v2 FakeHarness image"
	@echo "  make codex-harness-image   Build the pinned Codex Harness image"
	@echo "  make claude-harness-image  Build the pinned Claude Code Harness image"

server-check:
	$(MAKE) -C "$(SERVER_DIR)" fmt-check
	$(MAKE) -C "$(SERVER_DIR)" vet
	$(MAKE) -C "$(SERVER_DIR)" test

server-build-mcp:
	$(MAKE) -C "$(SERVER_DIR)" build-mcp VERSION="$(VERSION)"

web-install:
	cd "$(WEB_DIR)" && pnpm install --frozen-lockfile

web-check:
	cd "$(WEB_DIR)" && pnpm exec tsc -b
	cd "$(WEB_DIR)" && pnpm lint
	cd "$(WEB_DIR)" && pnpm build

check: server-check web-check

build:
	$(MAKE) -C "$(SERVER_DIR)" build VERSION="$(VERSION)"
	cd "$(WEB_DIR)" && pnpm build

fake-harness-image:
	docker build --build-arg BUILD_IMAGE="$(FAKE_HARNESS_BUILD_IMAGE)" --build-arg RUNTIME_IMAGE="$(FAKE_HARNESS_RUNTIME_IMAGE)" --build-arg SEVEN_FAKE_BATCH="$(FAKE_HARNESS_BATCH)" --build-arg SEVEN_VERSION="$(FAKE_HARNESS_VERSION)" --build-arg SEVEN_COMMIT="$(SOURCE_COMMIT)" --build-arg SEVEN_BUILD_DATE="$$(date -u +%Y-%m-%dT%H:%M:%SZ)" -f "$(ROOT_DIR)deploy/workforce/images/fake-harness.Dockerfile" -t "$(FAKE_HARNESS_TAG)" "$(ROOT_DIR)"

provider-proxy-image:
	docker build --build-arg SEVEN_VERSION="$(PROVIDER_PROXY_VERSION)" --build-arg SEVEN_COMMIT="$(SOURCE_COMMIT)" --build-arg SEVEN_BUILD_DATE="$$(date -u +%Y-%m-%dT%H:%M:%SZ)" -f "$(ROOT_DIR)deploy/workforce/images/provider-proxy.Dockerfile" -t "$(PROVIDER_PROXY_TAG)" "$(ROOT_DIR)"

codex-harness-image:
	docker build --build-arg RUNTIME_IMAGE="$(CODEX_HARNESS_RUNTIME_IMAGE)" --build-arg GO_PROXY="$(CODEX_HARNESS_GO_PROXY)" --build-arg SEVEN_VERSION="$(CODEX_HARNESS_VERSION)" --build-arg SEVEN_COMMIT="$(SOURCE_COMMIT)" --build-arg SEVEN_BUILD_DATE="$$(date -u +%Y-%m-%dT%H:%M:%SZ)" -f "$(ROOT_DIR)deploy/workforce/images/codex-harness.Dockerfile" -t "$(CODEX_HARNESS_TAG)" "$(ROOT_DIR)"

claude-harness-image:
	docker build --build-arg RUNTIME_IMAGE="$(CLAUDE_HARNESS_RUNTIME_IMAGE)" --build-arg GO_PROXY="$(CLAUDE_HARNESS_GO_PROXY)" --build-arg SEVEN_VERSION="$(CLAUDE_HARNESS_VERSION)" --build-arg SEVEN_COMMIT="$(SOURCE_COMMIT)" --build-arg SEVEN_BUILD_DATE="$$(date -u +%Y-%m-%dT%H:%M:%SZ)" -f "$(ROOT_DIR)deploy/workforce/images/claude-harness.Dockerfile" -t "$(CLAUDE_HARNESS_TAG)" "$(ROOT_DIR)"

release:
	"$(ROOT_DIR)scripts/release/build.sh" "$(VERSION)"

security-scan:
	"$(ROOT_DIR)scripts/release/check-source.sh"

clean:
	$(MAKE) -C "$(SERVER_DIR)" clean
	rm -rf "$(WEB_DIR)dist" "$(ROOT_DIR)dist"
