package job

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/jullury/akama/internal/agent"
	"github.com/jullury/akama/internal/config"
	"github.com/jullury/akama/internal/provider"
	"github.com/jullury/akama/internal/storage"
)

// StartLabelPoller starts a background goroutine that polls all connections
// for issues labeled with cfg.TriggerLabel and creates jobs for new ones.
// It is a no-op when cfg.PollIntervalMins <= 0 or cfg.TriggerLabel is empty.
func StartLabelPoller(ctx context.Context, db *sql.DB, bot *tgbotapi.BotAPI, agentCfg *agent.Config, cfg *config.Config) {
	if cfg.PollIntervalMins <= 0 || cfg.TriggerLabel == "" {
		return
	}
	go func() {
		ticker := time.NewTicker(time.Duration(cfg.PollIntervalMins) * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				pollLabels(ctx, db, bot, agentCfg, cfg)
			}
		}
	}()
}

func pollLabels(ctx context.Context, db *sql.DB, bot *tgbotapi.BotAPI, agentCfg *agent.Config, cfg *config.Config) {
	connections, err := storage.ListAllConnections(db)
	if err != nil {
		log.Printf("label poller: list connections: %v", err)
		return
	}
	now := time.Now()
	for _, conn := range connections {
		var since time.Time
		if conn.LastPolledAt != nil {
			since = *conn.LastPolledAt
		} else {
			since = now.Add(-time.Duration(cfg.PollIntervalMins) * time.Minute)
		}
		issues, err := provider.ListIssuesByLabel(conn.RepoURL, conn.GitToken, cfg.TriggerLabel, since, conn.Provider)
		if err != nil {
			log.Printf("label poller: list issues for %s: %v", conn.RepoURL, err)
			continue
		}
		for _, iss := range issues {
			if existing := storage.FindActiveJobByIssue(db, conn.ChatID, iss.URL); existing != nil {
				continue
			}
			startPolledJob(ctx, db, bot, agentCfg, conn, iss, cfg)
		}
		if err := storage.UpdateConnectionLastPolled(db, conn.ID, now); err != nil {
			log.Printf("label poller: update last_polled_at for connection %d: %v", conn.ID, err)
		}
	}
}

func StartReviewPoller(ctx context.Context, db *sql.DB, bot *tgbotapi.BotAPI, agentCfg *agent.Config, cfg *config.Config) {
	if cfg.PollIntervalMins <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(time.Duration(cfg.PollIntervalMins) * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				pollReviews(ctx, db, bot, agentCfg)
			}
		}
	}()
}

func pollReviews(ctx context.Context, db *sql.DB, bot *tgbotapi.BotAPI, agentCfg *agent.Config) {
	jobs, err := storage.FindJobsByPRCreatedStatus(db)
	if err != nil {
		log.Printf("review poller: list jobs: %v", err)
		return
	}
	now := time.Now()
	for _, j := range jobs {
		var since time.Time
		if j.LastReviewCheckAt != nil {
			since = *j.LastReviewCheckAt
		} else {
			since = j.UpdatedAt
		}
		reviewText, err := provider.GetPRReviewsSince(j.PRURL, j.GitToken, j.Provider, since)
		if err != nil {
			log.Printf("review poller: job %d: %v", j.ID, err)
			storage.SetJobLastReviewCheck(db, j.ID, now)
			continue
		}
		if reviewText == "" {
			storage.SetJobLastReviewCheck(db, j.ID, now)
			continue
		}
		bot.Send(tgbotapi.NewMessage(j.ChatID, fmt.Sprintf("🔍 Detected PR review on job #%d — addressing feedback automatically...", j.ID)))
		RunFollowUp(ctx, j.ID, reviewText, db, bot, agentCfg)
		storage.SetJobLastReviewCheck(db, j.ID, now)
	}
}

func startPolledJob(ctx context.Context, db *sql.DB, bot *tgbotapi.BotAPI, agentCfg *agent.Config, conn *storage.Connection, iss provider.IssueRef, cfg *config.Config) {
	// Fetch the full issue body from the provider.
	var issueBody string
	switch conn.Provider {
	case "github":
		gh, err := provider.FetchGitHubIssue(iss.URL, conn.GitToken)
		if err != nil {
			log.Printf("label poller: fetch github issue %s: %v", iss.URL, err)
		} else {
			issueBody = gh.Body
		}
	case "gitlab":
		gl, err := provider.FetchGitLabIssue(iss.URL, conn.GitToken)
		if err != nil {
			log.Printf("label poller: fetch gitlab issue %s: %v", iss.URL, err)
		} else {
			issueBody = gl.Description
		}
	}
	if issueBody != "" {
		issueBody = provider.EnrichIssueBody(conn.Provider, iss.URL, conn.GitToken, issueBody)
	}

	defaultBranch := conn.DefaultBranch
	if defaultBranch == "" {
		defaultBranch = "main"
	}

	// Determine agent name and model: prefer per-connection settings, fall back to global.
	agentName := cfg.DefaultAgent
	agentModel := cfg.DefaultModel
	if conn.Agent != "" {
		agentName = conn.Agent
	}
	if conn.AgentModel != "" {
		agentModel = conn.AgentModel
	}

	j := &storage.Job{
		ChatID:        conn.ChatID,
		IssueID:       iss.ID,
		IssueTitle:    iss.Title,
		IssueBody:     issueBody,
		IssueURL:      iss.URL,
		RepoURL:       conn.RepoURL,
		Provider:      conn.Provider,
		GitToken:      conn.GitToken,
		Agent:         agentName,
		AgentModel:    agentModel,
		DefaultBranch: defaultBranch,
	}
	jobID, err := storage.CreateJob(db, j)
	if err != nil {
		log.Printf("label poller: create job for issue %s: %v", iss.URL, err)
		return
	}

	msg := tgbotapi.NewMessage(conn.ChatID, fmt.Sprintf("🏷️ Detected label '%s' on: %s\nStarting job #%d...", cfg.TriggerLabel, iss.Title, jobID))
	if _, err := bot.Send(msg); err != nil {
		log.Printf("label poller: notify chat %d: %v", conn.ChatID, err)
	}

	Run(ctx, jobID)
}
