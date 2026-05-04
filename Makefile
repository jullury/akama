# Build configuration from .env file
# Usage: make build

.PHONY: build clean start

# Load .env file if it exists
ifneq (,$(wildcard .env))
    include .env
    export $(shell sed 's/=.*//' .env)
endif

build:
	@echo "Building akama with OAuth credentials from .env..."
	go build -ldflags "\
	-X github.com/jullury/akama/internal/config.GitHubClientID=$(GITHUB_CLIENT_ID) \
	-X github.com/jullury/akama/internal/config.GitHubClientSecret=$(GITHUB_CLIENT_SECRET) \
	-X github.com/jullury/akama/internal/config.GitLabClientID=$(GITLAB_CLIENT_ID) \
	-X github.com/jullury/akama/internal/config.GitLabClientSecret=$(GITLAB_CLIENT_SECRET)" \
	-o akama .
	@echo "Build complete: ./akama"

start: build
	@echo "Stopping any running akama instance..."
	@./akama stop 2>/dev/null || true
	@sleep 1
	@echo "Starting akama..."
	@./akama start
	@echo "Akama started."

clean:
	rm -f akama
