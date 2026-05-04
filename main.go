package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/jullury/akama/cmd"
	"github.com/jullury/akama/internal/bot"
	"github.com/jullury/akama/internal/config"
	"github.com/jullury/akama/internal/daemon"
	"github.com/jullury/akama/internal/job"
	"github.com/jullury/akama/internal/storage"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "start" {
		for _, arg := range os.Args[2:] {
			if arg == "--daemon" {
				runDaemon()
				return
			}
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

	// Write PID file from the daemon process itself so there is no
	// race between the parent's IsRunning check and fork.
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

	dbPath := cfg.DBPath
	if strings.HasPrefix(dbPath, "~/") {
		dbPath = filepath.Join(home, dbPath[2:])
	}
	db, err := storage.Open(dbPath)
	if err != nil {
		log.Fatalf("Open DB: %v", err)
	}
	defer db.Close()

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

	log.Println("Starting bot...")
	b.RunCtx(ctx)

	log.Println("Waiting for in-flight jobs (30s timeout)...")
	job.WaitForJobs(30)

	log.Println("Daemon stopped cleanly")
}
