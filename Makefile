# Build configuration from .env file
# Usage: make build

.PHONY: build clean start dist release

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
		-X github.com/jullury/akama/internal/config.GitLabClientSecret=$(GITLAB_CLIENT_SECRET)" \
		-o dist/akama-$$os-$$arch . ; \
	done
	@echo "Binaries written to dist/"

release:
	$(eval VERSION ?= v$(shell date +'%Y.%m.%d'))
	git tag -a $(VERSION) -m "Release $(VERSION)"
	git push origin $(VERSION)

clean:
	rm -f akama
	rm -rf dist
