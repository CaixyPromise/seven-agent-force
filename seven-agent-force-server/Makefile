SHELL := /bin/bash

APP_NAME := seven-agent-force-server
MCP_APP_NAME := seven-agent-force-mcp-server
MODULE_PATH := github.com/CaixyPromise/seven-agent/force
CMD_PATH := ./cmd/seven-agent-force-server
MCP_CMD_PATH := ./cmd/seven-agent-force-mcp-server
ROOT_DIR := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))
OUTPUT_DIR := $(ROOT_DIR)output
BIN_DIR := $(OUTPUT_DIR)/bin

GO ?= go
GOFMT ?= gofmt
CGO_ENABLED ?= 0
VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
CONFIG_DIR ?= $(ROOT_DIR)configs
MIGRATIONS_DIR ?= $(ROOT_DIR)migrations
MIGRATION_NAME ?= change_me
NAME ?= $(MIGRATION_NAME)

BUILD_FLAGS := -trimpath
RELEASE_LDFLAGS := -s -w \
	-X '$(MODULE_PATH)/internal/shared/buildinfo.Version=$(VERSION)' \
	-X '$(MODULE_PATH)/internal/shared/buildinfo.Commit=$(COMMIT)' \
	-X '$(MODULE_PATH)/internal/shared/buildinfo.BuildDate=$(BUILD_DATE)'

.PHONY: help tidy fmt fmt-check vet test test-race build build-release build-mcp run migrate-up migrate-down migrate-status migrate-version migrate-create clean

help:
	@echo "seven-agent-force-server targets"
	@echo "  make fmt-check      Check Go formatting"
	@echo "  make vet            Run go vet"
	@echo "  make test           Run all Go tests"
	@echo "  make test-race      Run all tests with the race detector"
	@echo "  make build-release  Build the public server binary"
	@echo "  make build-mcp      Build the stateless MCP server binary"
	@echo "  make run            Start the service from the source tree"
	@echo "  make migrate-up     Apply migrations"

tidy:
	cd "$(ROOT_DIR)" && $(GO) mod tidy

fmt:
	cd "$(ROOT_DIR)" && find . -name '*.go' -print0 | xargs -0 $(GOFMT) -w

fmt-check:
	@files="$$(cd "$(ROOT_DIR)" && find . -name '*.go' -type f -print)"; \
	bad="$$(cd "$(ROOT_DIR)" && $(GOFMT) -l $$files)"; \
	test -z "$$bad" || { echo "Unformatted Go files:"; echo "$$bad"; exit 1; }

vet:
	cd "$(ROOT_DIR)" && $(GO) vet ./...

test:
	cd "$(ROOT_DIR)" && $(GO) test ./... -count=1

test-race:
	cd "$(ROOT_DIR)" && CGO_ENABLED=1 $(GO) test -race ./... -count=1

build: build-release

build-release:
	@mkdir -p "$(BIN_DIR)"
	cd "$(ROOT_DIR)" && CGO_ENABLED=$(CGO_ENABLED) $(GO) build $(BUILD_FLAGS) -ldflags "$(RELEASE_LDFLAGS)" -o "$(BIN_DIR)/$(APP_NAME)" $(CMD_PATH)
	@echo "Built $(BIN_DIR)/$(APP_NAME)"

build-mcp:
	@mkdir -p "$(BIN_DIR)"
	cd "$(ROOT_DIR)" && CGO_ENABLED=$(CGO_ENABLED) $(GO) build $(BUILD_FLAGS) -ldflags "$(RELEASE_LDFLAGS)" -o "$(BIN_DIR)/$(MCP_APP_NAME)" $(MCP_CMD_PATH)
	@echo "Built $(BIN_DIR)/$(MCP_APP_NAME)"

run:
	cd "$(ROOT_DIR)" && $(GO) run $(CMD_PATH) serve --home "$(ROOT_DIR)" --config-dir "$(CONFIG_DIR)" --migrations-dir "$(MIGRATIONS_DIR)"

migrate-up:
	cd "$(ROOT_DIR)" && $(GO) run $(CMD_PATH) migrate --home "$(ROOT_DIR)" --config-dir "$(CONFIG_DIR)" --migrations-dir "$(MIGRATIONS_DIR)" up

migrate-down:
	cd "$(ROOT_DIR)" && $(GO) run $(CMD_PATH) migrate --home "$(ROOT_DIR)" --config-dir "$(CONFIG_DIR)" --migrations-dir "$(MIGRATIONS_DIR)" down

migrate-status:
	cd "$(ROOT_DIR)" && $(GO) run $(CMD_PATH) migrate --home "$(ROOT_DIR)" --config-dir "$(CONFIG_DIR)" --migrations-dir "$(MIGRATIONS_DIR)" status

migrate-version:
	cd "$(ROOT_DIR)" && $(GO) run $(CMD_PATH) migrate --home "$(ROOT_DIR)" --config-dir "$(CONFIG_DIR)" --migrations-dir "$(MIGRATIONS_DIR)" version

migrate-create:
	cd "$(ROOT_DIR)" && $(GO) run $(CMD_PATH) migrate --home "$(ROOT_DIR)" --config-dir "$(CONFIG_DIR)" --migrations-dir "$(MIGRATIONS_DIR)" create "$(NAME)" sql

clean:
	rm -rf "$(OUTPUT_DIR)"
