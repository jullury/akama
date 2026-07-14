# Split Host CLI and Daemon into Two Binaries

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task.

**Goal:** Split the single `akama` binary into two: `akama` (host CLI for init + Docker management) and `akama-daemon` (in-container Telegram bot + agent execution).

**Architecture:** Two `main.go` entry points share the same `internal/` packages. The host CLI (`main.go`) runs Cobra commands for init, start/stop/status/logs/update/restart, all using Docker SDK. The daemon (`cmd/akama-daemon/main.go`) runs the Telegram bot, agent execution, job scheduling, and provider APIs. The `--daemon` flag detection is removed from the host binary entirely.

**Tech Stack:** Go 1.26, Cobra CLI, Docker SDK, Telegram Bot API

---

## Background

Currently a single binary serves dual purposes via a `--daemon` flag check in `main.go`. This couples host management logic (Docker container orchestration) with in-container daemon logic (Telegram bot, agent execution). The user wants clean separation:

- **`akama`** (host): manages init, Docker containers, config, host-side concerns
- **`akama-daemon`** (container): runs inside the Docker container — Telegram bot, agents, jobs

---

## Task 1: Remove skill installation from `akama start`

Skills are already installed in the Docker image (Dockerfile installs claude/opencode via npm). The host CLI doesn't need `agent.BuiltinSkills` or `agent.InstallSkill()` — those belong to the daemon side (Telegram `/skills` and `/install_skill` commands).

**Files:**
- Modify: `cmd/start.go:46-52`

**Step 1: Remove the skill installation block**

Delete these lines from `runStart()`:

```go
for _, s := range agent.BuiltinSkills {
    if s.Required {
        if err := agent.InstallSkill(s); err != nil {
            fmt.Fprintf(os.Stderr, "Install skill %s: %v\n", s.Name, err)
        }
    }
}
```

Also remove the `agent` import if it becomes unused (check other usages in the file first).

**Step 2: Verify build**

Run: `go build ./...`
Expected: PASS

**Step 3: Commit**

```bash
git add cmd/start.go
git commit -t .git/COMMIT_MSG -m "refactor: remove skill installation from host CLI start command

Skills are installed in the Docker image at build time.
The host CLI only manages container lifecycle, not agent skills."
```

---

## Task 2: Create daemon entry point

Create `cmd/akama-daemon/main.go` — the in-container binary that runs the Telegram bot.

**Files:**
- Create: `cmd/akama-daemon/main.go`

**Step 1: Create the daemon main.go**

Copy the `runDaemon()` function from the current `main.go` into a new file at `cmd/akama-daemon/main.go`. This file becomes `package main` and contains only the daemon logic — no Cobra, no CLI commands.

```go
package main

import (
	"context"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/jullury/akama/internal/agent"
	"github.com/jullury/akama/internal/bot"
	"github.com/jullury/akama/internal/config"
	"github.com/jullury/akama/internal/crypto"
	"github.com/jullury/akama/internal/daemon"
	"github.com/jullury/akama/internal/job"
	"github.com/jullury/akama/internal/knowledge"
	"github.com/jullury/akama/internal/logger"
	"github.com/jullury/akama/internal/metrics"
	"github.com/jullury/akama/internal/storage"
)

func main() {
	home, _ := os.UserHomeDir()
	cfgPath := filepath.Join(home, ".akama", "config.yaml")

	// Redirect log output to a file immediately so that crashes before the
	// rotating logger is ready are not silently swallowed into /dev/null.
	earlyLogDir := filepath.Join(home, ".akama", "logs")
	os.MkdirAll(earlyLogDir, 0755)
	if f, err := os.OpenFile(
		filepath.Join(earlyLogDir, "akama-startup.log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644,
	); err == nil {
		log.SetOutput(f)
		defer f.Close()
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("Load config: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		log.Fatalf("Invalid config: %v", err)
	}

	// When running inside a container, environment variables override the config
	// so the daemon uses internal container-network URLs instead of host-bound ones.
	if v := os.Getenv("POSTGRES_URL"); v != "" {
		cfg.PostgresURL = v
	}
	if v := os.Getenv("OLLAMA_URL"); v != "" {
		cfg.OllamaURL = v
	}

	lw, err := logger.NewRotatingWriter(logger.Config{
		LogPath: cfg.LogPath,
	})
	if err != nil {
		log.Fatalf("Create logger: %v", err)
	}
	defer lw.Close()
	if os.Getpid() == 1 {
		log.SetOutput(io.MultiWriter(os.Stdout, lw))
	} else {
		log.SetOutput(lw)
	}

	pidPath := cfg.PIDPath
	if strings.HasPrefix(pidPath, "~/") {
		pidPath = filepath.Join(home, pidPath[2:])
	}
	// When running as PID 1 (inside a container) Docker manages the lifecycle,
	// so skip the host-side PID file guard entirely.
	if os.Getpid() == 1 {
		os.Remove(pidPath)
	} else {
		if daemon.IsRunning(pidPath) {
			log.Fatalf("Another akama daemon is already running; run 'akama stop' first")
		}
		if err := daemon.WritePID(pidPath, os.Getpid()); err != nil {
			log.Fatalf("Write PID: %v", err)
		}
		defer daemon.RemovePID(pidPath)
	}

	db, err := storage.Open(cfg.PostgresURL)
	if err != nil {
		log.Fatalf("Open DB: %v", err)
	}
	defer db.Close()

	if err := storage.RecoverInterruptedJobs(db); err != nil {
		log.Printf("recover interrupted jobs: %v", err)
	}

	keyPath := filepath.Join(home, ".akama", "keyfile")
	encKey, err := crypto.LoadOrCreateKey(keyPath)
	if err != nil {
		log.Fatalf("Load encryption key: %v", err)
	}
	storage.SetEncryptionKey(encKey)
	if err := storage.MigrateTokenEncryption(db); err != nil {
		log.Printf("migrate token encryption: %v", err)
	}

	if cfg.AdminUserID != 0 {
		if err := storage.AddAuthorizedUser(db, cfg.AdminUserID, "admin", cfg.AdminUserID); err != nil {
			log.Printf("add admin user: %v", err)
		}
	}

	b, err := bot.New(cfg.TelegramToken)
	if err != nil {
		log.Fatalf("Create bot: %v", err)
	}
	b.JobsDB = db
	b.Config = cfg

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigChan
		log.Println("Received shutdown signal")
		cancel()
	}()

	agentCfg := &agent.Config{
		APIKeys:          cfg.APIKeys,
		TimeoutMins:      cfg.AgentTimeoutMins,
		WorkspaceBaseDir: cfg.WorkspaceDir,
	}
	job.InitScheduler(db, b.API, agentCfg, cfg.WorkspaceDir, cfg.MaxConcurrentJobs, cfg.QuietHoursStart, cfg.QuietHoursEnd, cfg.OllamaURL)
	job.StartLabelPoller(ctx, db, b.API, agentCfg, cfg)
	job.StartReviewPoller(ctx, db, b.API, agentCfg, cfg)

	// Pull the Ollama embedding model in the background so it's ready
	// when the first job needs knowledge retrieval.
	go knowledge.EnsureModel(ctx, cfg.OllamaURL, knowledge.EmbeddingModel)

	go func() {
		job.CleanOldWorkspaces(cfg.WorkspaceDir, cfg.MaxWorkspaceAgeDays)
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				job.CleanOldWorkspaces(cfg.WorkspaceDir, cfg.MaxWorkspaceAgeDays)
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				log.Printf("[metrics] %s", metrics.Summary())
			case <-ctx.Done():
				return
			}
		}
	}()
	b.RunCtx(ctx)

	log.Println("Waiting for in-flight jobs (30s timeout)...")
	job.WaitForJobs(30)

	log.Println("Daemon stopped cleanly")
}
```

**Step 2: Verify build**

Run: `go build ./cmd/akama-daemon/`
Expected: produces `akama-daemon` binary

**Step 3: Commit**

```bash
git add cmd/akama-daemon/main.go
git commit -m "feat: add daemon entry point as separate binary

Extracts the daemon (Telegram bot + agent execution) into its own
main.go at cmd/akama-daemon/. This binary runs inside the Docker
container and handles all in-container operations."
```

---

## Task 3: Simplify host `main.go` to CLI-only

Remove the `--daemon` flag detection and `runDaemon()` function from the root `main.go`. The host binary is now purely a CLI tool.

**Files:**
- Modify: `main.go`

**Step 1: Rewrite main.go to CLI-only**

Replace the entire file content:

```go
package main

import (
	"github.com/jullury/akama/cmd"
)

func main() {
	cmd.Execute()
}
```

All daemon imports (`bot`, `job`, `logger`, `metrics`, `knowledge`, `crypto`, `storage`, `agent.Config`, `daemon`) are removed from this file. They're no longer needed since the host binary only runs Cobra commands.

**Step 2: Verify build**

Run: `go build ./...`
Expected: PASS (both `main.go` and `cmd/akama-daemon/main.go` build)

**Step 3: Commit**

```bash
git add main.go
git commit -m "refactor: simplify host main.go to CLI-only

Removes --daemon flag detection and runDaemon() from the root binary.
The daemon is now a separate binary at cmd/akama-daemon/."
```

---

## Task 4: Update Dockerfile to build daemon binary

The Dockerfile currently builds from `.` (root main.go). Change it to build from `./cmd/akama-daemon/` and name the output `akama-daemon`.

**Files:**
- Modify: `Dockerfile:30-40` (build step)
- Modify: `Dockerfile:85-87` (copy + entrypoint)

**Step 1: Change the build output path**

In the Dockerfile, change line 40:
```dockerfile
# FROM:
        -o /akama .
# TO:
        -o /akama-daemon ./cmd/akama-daemon
```

**Step 2: Update the COPY and ENTRYPOINT**

```dockerfile
# FROM:
COPY --from=builder /akama /usr/local/bin/akama
WORKDIR /workspaces
ENTRYPOINT ["akama", "--daemon"]

# TO:
COPY --from=builder /akama-daemon /usr/local/bin/akama-daemon
WORKDIR /workspaces
ENTRYPOINT ["akama-daemon"]
```

**Step 3: Verify build**

Run: `docker build -t akama-daemon:test .`
Expected: image builds successfully

**Step 4: Commit**

```bash
git add Dockerfile
git commit -m "build: update Dockerfile to build daemon binary separately

Dockerfile now builds from cmd/akama-daemon/ and produces
akama-daemon binary. Entrypoint is now 'akama-daemon' directly
instead of 'akama --daemon'."
```

---

## Task 5: Update Makefile build targets

Add a `build-daemon` target and update existing targets to reflect the two-binary architecture.

**Files:**
- Modify: `Makefile`

**Step 1: Add daemon build target and update existing targets**

```makefile
# build: host CLI binary
build:
	@echo "Building akama (host CLI) with OAuth credentials from .env..."
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

# build-daemon: in-container daemon binary
build-daemon:
	@echo "Building akama-daemon (in-container binary)..."
	CGO_ENABLED=0 go build -ldflags "-s -w \
	-X github.com/jullury/akama/internal/config.Version=$(VERSION) \
	-X github.com/jullury/akama/internal/config.BuildTime=$(BUILD_TIME) \
	-X github.com/jullury/akama/internal/config.BuildPlatform=$(BUILD_PLATFORM)" \
	-o akama-daemon ./cmd/akama-daemon
	@echo "Build complete: ./akama-daemon (version: $(VERSION))"

# start: build both and start
start: build
	@echo "Stopping any running akama instance..."
	@./akama stop 2>/dev/null || true
	@sleep 1
	@echo "Starting akama..."
	@./akama start
	@echo "Akama started."

# dist: cross-compile host CLI for all platforms
dist:
	@echo "Building release binaries for all platforms..."
	@mkdir -p dist
	@for pair in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64; do \
		os=$$(echo $$pair | cut -d/ -f1); \
		arch=$$(echo $$pair | cut -d/ -f2); \
		ext=""; \
		if [ "$$os" = "windows" ]; then ext=".exe"; fi; \
		echo "  Building $$os/$$arch..."; \
		GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 go build -ldflags "-s -w \
		-X github.com/jullury/akama/internal/config.GitHubClientID=$(GITHUB_CLIENT_ID) \
		-X github.com/jullury/akama/internal/config.GitHubClientSecret=$(GITHUB_CLIENT_SECRET) \
		-X github.com/jullury/akama/internal/config.GitLabClientID=$(GITLAB_CLIENT_ID) \
		-X github.com/jullury/akama/internal/config.GitLabClientSecret=$(GITLAB_CLIENT_SECRET) \
		-X github.com/jullury/akama/internal/config.Version=$(VERSION) \
		-X github.com/jullury/akama/internal/config.BuildTime=$(BUILD_TIME) \
		-X github.com/jullury/akama/internal/config.BuildPlatform=$$os/$$arch" \
		-o "dist/akama-$$os-$$arch$$ext" . ; \
	done
	@echo "Host CLI binaries written to dist/"

# clean: remove all built artifacts
clean:
	rm -f akama akama-daemon
	rm -rf dist
```

**Step 2: Verify build**

Run: `make build && make build-daemon`
Expected: both binaries built

**Step 3: Commit**

```bash
git add Makefile
git commit -m "build: add build-daemon target for in-container binary

Separates host CLI and daemon build targets. 'make build' produces
the host CLI, 'make build-daemon' produces the in-container binary."
```

---

## Task 6: Update install.sh for two-binary awareness

The install script currently only downloads the host CLI binary. Update it to be explicit about its scope and handle the daemon binary for Docker-based installs.

**Files:**
- Modify: `install.sh`

**Step 1: Rewrite install.sh**

```bash
#!/bin/sh
set -e

REPO="jullury/akama"
INSTALL_DIR="/usr/local/bin"

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$ARCH" in
  x86_64)         ARCH="amd64" ;;
  aarch64|arm64)  ARCH="arm64" ;;
  *)
    echo "Unsupported architecture: $ARCH"
    exit 1
    ;;
esac

case "$OS" in
  linux|darwin) ;;
  *)
    echo "Unsupported OS: $OS"
    exit 1
    ;;
esac

# --- Install host CLI binary ---
ASSET="akama-${OS}-${ARCH}"
URL="https://github.com/${REPO}/releases/latest/download/${ASSET}"

echo "Downloading akama host CLI for ${OS}/${ARCH}..."
curl -fsSL "$URL" -o "/tmp/akama"
chmod +x "/tmp/akama"

echo "Installing to ${INSTALL_DIR}/akama..."
if [ -w "$INSTALL_DIR" ]; then
  mv "/tmp/akama" "${INSTALL_DIR}/akama"
else
  sudo mv "/tmp/akama" "${INSTALL_DIR}/akama"
fi

echo ""
echo "akama host CLI installed successfully!"
echo ""
echo "The akama binary manages host-side operations:"
echo "  - akama init      : configure Telegram bot token, API keys, admin user"
echo "  - akama start     : launch Docker containers (daemon, postgres, ollama)"
echo "  - akama stop      : stop all containers"
echo "  - akama status    : check container health and active jobs"
echo "  - akama logs      : view daemon container logs"
echo "  - akama restart   : restart all containers"
echo "  - akama update    : pull latest daemon image and recreate container"
echo ""
echo "The daemon (Telegram bot + agent execution) runs inside the"
echo "akama-daemon Docker container, managed automatically by 'akama start'."
echo ""
echo "Run 'akama init' to get started."
```

**Step 2: Commit**

```bash
git add install.sh
git commit -m "docs: update install script to clarify host CLI scope

The install script only manages the host-side binary. The daemon
(Telegram bot + agent execution) runs inside the Docker container."
```

---

## Task 7: Update CI release workflow

The CI workflow needs to build the host CLI binary for releases AND the daemon Docker image. The host CLI binary is what gets uploaded to GitHub releases.

**Files:**
- Modify: `.github/workflows/release.yml:79-95` (build-binaries step)

**Step 1: Verify the build command**

The current CI builds from `.` (root main.go). After Task 3, the root `main.go` is CLI-only, so the CI build command `go build ... -o "akama-${{ matrix.goos }}-${{ matrix.goarch }}"` still works correctly — it builds the host CLI binary. No change needed to the binary build step.

The `build-daemon` job already builds the Docker image using the Dockerfile, which was updated in Task 4.

**Step 2: Verify no changes needed**

Check: The `build-binaries` job builds from `.` which is now CLI-only. The `build-daemon` job uses the Dockerfile which builds from `./cmd/akama-daemon/`. Both are correct.

If the build step needs the OAuth ldflags, those are already present. The daemon binary built in the Dockerfile doesn't need host-side OAuth since it runs containerized (OAuth is baked in at Docker build time).

**Step 3: Commit (if any changes)**

Only commit if modifications are actually needed. If the workflow already works as-is after Tasks 3-4, skip this commit.

---

## Task 8: Update AGENTS.md documentation

Update the architecture section in AGENTS.md to reflect the two-binary split.

**Files:**
- Modify: `AGENTS.md`

**Step 1: Update relevant sections**

Update the "Architecture" section to document:

```
## Two Binaries

- **`akama`** (host CLI): `main.go` → `cmd.Execute()`. Manages Docker containers, config, init.
  - Commands: `init`, `start`, `stop`, `status`, `logs`, `restart`, `update`, `db`, `migrate`
  - Built from: `.` (root main.go)

- **`akama-daemon`** (in-container): `cmd/akama-daemon/main.go`. Runs Telegram bot, agent execution, job scheduling.
  - Built from: `./cmd/akama-daemon/`
  - Runs inside Docker container (`ghcr.io/jullury/akama-daemon:latest`)
```

Also update the entrypoint references: the container now runs `akama-daemon` directly (not `akama --daemon`).

**Step 2: Commit**

```bash
git add AGENTS.md
git commit -m "docs: update AGENTS.md for two-binary architecture

Documents the split between host CLI (akama) and in-container
daemon (akama-daemon) binaries."
```

---

## Verification Checklist

After all tasks:

1. `go build ./...` — both binaries compile
2. `make build` — produces `./akama` (host CLI)
3. `make build-daemon` — produces `./akama-daemon` (in-container)
4. `./akama --help` — shows Cobra CLI commands (init, start, stop, etc.)
5. `./akama-daemon --help` — shows no Cobra, just starts the daemon
6. `docker build -t akama-daemon:test .` — Dockerfile builds successfully
7. `./akama init` — works as before (config setup)
8. `./akama start` — works as before (Docker container management)
9. The daemon container runs `akama-daemon` directly as its entrypoint

---

## What NOT to Change

- `internal/` packages remain untouched — they serve both binaries via imports
- `cmd/` files (start.go, stop.go, status.go, etc.) remain in the `cmd` package — they're used by the host CLI's `cmd.Execute()`
- The Telegram `/update` and `/update_agents` commands inside the daemon still work — they call `agent.UpdateAll()` which is in the `internal/agent` package
- `cmd/init.go`'s `UpdateAgents()` function stays — it's used by `akama init` on the host side
- No changes to `internal/docker/`, `internal/bot/`, `internal/job/`, etc.
