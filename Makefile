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

docker-build:
	@echo "Building akama-daemon Docker image..."
	@if [ ! -f .env ]; then echo ".env not found — OAuth creds required"; exit 1; fi
	@docker build \
		--build-arg GITHUB_CLIENT_ID=$(GITHUB_CLIENT_ID) \
		--build-arg GITHUB_CLIENT_SECRET=$(GITHUB_CLIENT_SECRET) \
		--build-arg GITLAB_CLIENT_ID=$(GITLAB_CLIENT_ID) \
		--build-arg GITLAB_CLIENT_SECRET=$(GITLAB_CLIENT_SECRET) \
		--build-arg VERSION=$(VERSION) \
		--build-arg BUILD_TIME=$(BUILD_TIME) \
		--build-arg BUILD_PLATFORM=$(BUILD_PLATFORM) \
		-t ghcr.io/jullury/akama-daemon:latest \
		-t ghcr.io/jullury/akama-daemon:$(VERSION) \
		.
	@echo "Image built: ghcr.io/jullury/akama-daemon:$(VERSION)"

docker-push:
	@echo "Pushing akama-daemon to ghcr.io..."
	@docker push ghcr.io/jullury/akama-daemon:latest
	@docker push ghcr.io/jullury/akama-daemon:$(VERSION)
	@echo "Pushed: ghcr.io/jullury/akama-daemon:$(VERSION)"

# Bare fix to trigger patch release
# TODO: remove this commit after v3.0.1 is released

clean:
	rm -f akama
	rm -rf dist
