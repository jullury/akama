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

	"github.com/jullury/akama/cmd"
	"github.com/jullury/akama/internal/agent"
	"github.com/jullury/akama/internal/bot"
	"github.com/jullury/akama/internal/config"
	"github.com/jullury/akama/internal/crypto"
	"github.com/jullury/akama/internal/daemon"
	"github.com/jullury/akama/internal/job"
	"github.com/jullury/akama/internal/logger"
	"github.com/jullury/akama/internal/metrics"
	"github.com/jullury/akama/internal/storage"
)

func main() {
	for _, arg := range os.Args[1:] {
		if arg == "--daemon" {
			runDaemon()
			return
		}
	}

	cmd.Execute()
}

func runDaemon() {
	home, _ := os.UserHomeDir()
	cfgPath := filepath.Join(home, ".akama", "config.yaml")

	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("Load config: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		log.Fatalf("Invalid config: %v", err)
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
	if daemon.IsRunning(pidPath) {
		log.Fatalf("Another akama daemon is already running; run 'akama stop' first")
	}
	if err := daemon.WritePID(pidPath, os.Getpid()); err != nil {
		log.Fatalf("Write PID: %v", err)
	}
	defer daemon.RemovePID(pidPath)

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
		APIKeys:     cfg.APIKeys,
		TimeoutMins: cfg.AgentTimeoutMins,
	}
	job.InitScheduler(db, b.API, agentCfg, cfg.WorkspaceDir, cfg.MaxConcurrentJobs, cfg.QuietHoursStart, cfg.QuietHoursEnd)
	job.StartLabelPoller(ctx, db, b.API, agentCfg, cfg)
	job.StartReviewPoller(ctx, db, b.API, agentCfg, cfg)

	log.Println("Starting bot...")
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
