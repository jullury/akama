# Build configuration from .env file
# Usage: make build

.PHONY: setup build clean start dist release

# Load .env file if it exists
ifneq (,$(wildcard .env))
    include .env
    export $(shell sed 's/=.*//' .env)
endif

VERSION ?= $(shell git describe --tags --always --abbrev=0 2>/dev/null || echo "dev")
BUILD_TIME ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
BUILD_PLATFORM ?= $(shell go env GOOS)/$(shell go env GOARCH)

setup:
	@echo "Installing mise and toolchain..."
	@if command -v mise > /dev/null 2>&1; then \
		MISE=$$(command -v mise); \
	else \
		curl -sS https://mise.run/ | sh; \
		MISE="$$HOME/.local/bin/mise"; \
	fi; \
	$$MISE install
	@echo "Toolchain ready."

build:
	@echo "Building akama with OAuth credentials from .env..."
	go build -ldflags "\
	-X github.com/jullury/akama/internal/config.GitHubClientID=$(GITHUB_CLIENT_ID) \
	-X github.com/jullury/akama/internal/config.GitHubClientSecret=$(GITHUB_CLIENT_SECRET) \
	-X github.com/jullury/akama/internal/config.GitLabClientID=$(GITLAB_CLIENT_ID) \
	-X github.com/jullury/akama/internal/config.GitLabClientSecret=$(GITLAB_CLIENT_SECRET) \
	-X github.com/jullury/akama/internal/config.Version=$(VERSION) \
	-X github.com/jullury/akama/internal/config.BuildTime=$(BUILD_TIME) \
	-X github.com/jullury/akama/internal/config.BuildPlatform=$(BUILD_PLATFORM)" \
	-o akama .
	@echo "Build complete: ./akama (version: $(VERSION))"

start: build
	@echo "Stopping any running akama instance..."
	@./akama stop 2>/dev/null || true
	@sleep 1
	@echo "Starting akama..."
	@./akama start
	@echo "Akama started."

dist:
	@echo "Building release binaries for all platforms..."
	@mkdir -p dist
	@for pair in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64; do \
		os=$$(echo $$pair | cut -d/ -f1); \
		arch=$$(echo $$pair | cut -d/ -f2); \
		echo "  Building $$os/$$arch..."; \
		GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 go build -ldflags "-s -w \
		-X github.com/jullury/akama/internal/config.GitHubClientID=$(GITHUB_CLIENT_ID) \
		-X github.com/jullury/akama/internal/config.GitHubClientSecret=$(GITHUB_CLIENT_SECRET) \
		-X github.com/jullury/akama/internal/config.GitLabClientID=$(GITLAB_CLIENT_ID) \
		-X github.com/jullury/akama/internal/config.GitLabClientSecret=$(GITLAB_CLIENT_SECRET) \
		-X github.com/jullury/akama/internal/config.Version=$(VERSION) \
		-X github.com/jullury/akama/internal/config.BuildTime=$(BUILD_TIME) \
		-X github.com/jullury/akama/internal/config.BuildPlatform=$$os/$$arch" \
		-o dist/akama-$$os-$$arch . ; \
	done
	@echo "Binaries written to dist/"

release:
	@echo "Release is now handled by semantic-release on push to main."
	@echo "Ensure your commits follow conventional commits: https://www.conventionalcommits.org/"

clean:
	rm -f akama
	rm -rf dist
