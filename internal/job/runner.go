package job

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/jullury/akama/internal/agent"
	"github.com/jullury/akama/internal/git"
	"github.com/jullury/akama/internal/provider"
	"github.com/jullury/akama/internal/storage"
)

var wg sync.WaitGroup

var (
	cancelsMu sync.Mutex
	cancels   = make(map[int64]context.CancelFunc)
)

func registerCancel(jobID int64, cancel context.CancelFunc) {
	cancelsMu.Lock()
	cancels[jobID] = cancel
	cancelsMu.Unlock()
}

func deregisterCancel(jobID int64) {
	cancelsMu.Lock()
	delete(cancels, jobID)
	cancelsMu.Unlock()
}

// CancelJob cancels a running job by jobID. Returns true if the job was found.
func CancelJob(jobID int64) bool {
	cancelsMu.Lock()
	cancel, ok := cancels[jobID]
	if ok {
		delete(cancels, jobID)
	}
	cancelsMu.Unlock()
	if ok {
		cancel()
	}
	return ok
}

func Run(ctx context.Context, jobID int64, jobsDB *sql.DB, bot *tgbotapi.BotAPI, agentCfg *agent.Config, workspaceDir string) {
	jobCtx, cancel := context.WithCancel(ctx)
	registerCancel(jobID, cancel)
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer deregisterCancel(jobID)
		defer cancel()
		runJob(jobCtx, jobID, jobsDB, bot, agentCfg, workspaceDir)
	}()
}

func runJob(ctx context.Context, jobID int64, jobsDB *sql.DB, bot *tgbotapi.BotAPI, agentCfg *agent.Config, workspaceDir string) {
	j, err := storage.GetJob(jobsDB, jobID)
	if err != nil {
		log.Printf("get job %d: %v", jobID, err)
		return
	}

	userCfg, err := storage.GetUserConfig(jobsDB, j.ChatID)
	if err != nil {
		log.Printf("get user config: %v", err)
	}
	gitName, gitEmail := "", ""
	if userCfg != nil {
		gitName = userCfg.GitName
		gitEmail = userCfg.GitEmail
		if userCfg.Agent != "" {
			j.Agent = userCfg.Agent
		}
		if userCfg.AgentModel != "" {
			j.AgentModel = userCfg.AgentModel
		}
	}

	repoPath, _ := url.Parse(j.RepoURL)
	parts := strings.Split(strings.Trim(repoPath.Path, "/"), "/")
	var workspacePath string
	if len(parts) >= 2 {
		workspacePath = filepath.Join(workspaceDir, j.Provider, parts[0], parts[1], j.IssueID)
	} else {
		workspacePath = filepath.Join(workspaceDir, fmt.Sprintf("%d", jobID))
	}

	owner, repo, _ := git.OwnerRepo(j.RepoURL)
	repoName := owner + "/" + repo

	if err := storage.SetJobRunning(jobsDB, jobID, workspacePath); err != nil {
		log.Printf("set running: %v", err)
		return
	}

	notify(bot, j.ChatID, fmt.Sprintf("🔍 [%s] %s — cloning repo for: %s...", j.Provider, repoName, j.IssueTitle))

	if err := withRetry(ctx, "git clone", 3, func() error {
		return git.Clone(j.RepoURL, j.GitToken, workspacePath)
	}); err != nil {
		failJob(jobsDB, bot, j, fmt.Sprintf("git clone: %v", err), workspacePath)
		return
	}

	notify(bot, j.ChatID, fmt.Sprintf("🤖 [%s] %s — running AI agent on: %s", j.Provider, repoName, j.IssueTitle))

	prompt := agent.BuildPrompt(j.IssueTitle, j.IssueURL, j.IssueBody)
	promptPath, err := agent.WritePrompt(workspacePath, prompt)
	if err != nil {
		failJob(jobsDB, bot, j, fmt.Sprintf("write prompt: %v", err), workspacePath)
		return
	}

	heartbeatStop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		elapsed := 0
		for {
			select {
			case <-heartbeatStop:
				return
			case <-ticker.C:
				elapsed += 5
				notify(bot, j.ChatID, fmt.Sprintf("⏳ [%s] Agent still working... (%d min elapsed)", j.Provider, elapsed))
			}
		}
	}()

	var rawOutput string
	agentErr := withRetry(ctx, "agent run", 3, func() error {
		var e error
		rawOutput, e = agent.Run(ctx, j.Agent, j.AgentModel, workspacePath, promptPath, agentCfg)
		return e
	})
	close(heartbeatStop)

	if agentErr != nil {
		failJob(jobsDB, bot, j, fmt.Sprintf("agent run: %v", agentErr), workspacePath)
		return
	}
	storage.SetJobAgentOutput(jobsDB, jobID, rawOutput)

	agentText := agent.ParseOutput(j.Agent, rawOutput)

	if agent.IsQuestion(agentText) {
		if err := storage.SetJobAwaitingInput(jobsDB, jobID, agentText); err != nil {
			log.Printf("set awaiting_input: %v", err)
		}
		storage.SetConversationState(jobsDB, j.ChatID, "telegram", "await_agent_input",
			map[string]interface{}{"job_id": jobID})
		msg := tgbotapi.NewMessage(j.ChatID, fmt.Sprintf(
			"🤔 [%s] %s — agent needs your input:\n\n%s\n\nJust reply with your answer.",
			j.Provider, repoName, agentText,
		))
		sent, _ := bot.Send(msg)
		if sent.MessageID != 0 {
			storage.SetJobNotifMsgID(jobsDB, jobID, int64(sent.MessageID))
		}
		return
	}

	notify(bot, j.ChatID, fmt.Sprintf("📦 [%s] %s — committing and pushing changes...", j.Provider, repoName))

	commitMsg, prBody := agent.GenerateSummary(ctx, j.Agent, j.AgentModel, workspacePath, j.IssueURL, agentCfg)
	branchName := agent.BranchFromCommit(commitMsg, fmt.Sprintf("fix/issue-%s", j.IssueID))
	if err := withRetry(ctx, "git push", 3, func() error {
		return git.CommitPush(workspacePath, branchName, j.GitToken, gitName, gitEmail, commitMsg)
	}); err != nil {
		failJob(jobsDB, bot, j, fmt.Sprintf("commit/push: %v", err), workspacePath)
		return
	}

	diffStat := git.DiffStat(workspacePath)

	notify(bot, j.ChatID, fmt.Sprintf("🔗 [%s] %s — creating pull request...", j.Provider, repoName))
	var prURL string
	if err := withRetry(ctx, "create PR", 3, func() error {
		var e error
		switch j.Provider {
		case "github":
			prURL, e = provider.CreateGitHubPR(j.RepoURL, j.GitToken, j.IssueTitle, branchName, prBody)
		case "gitlab":
			prURL, e = provider.CreateGitLabMR(j.RepoURL, j.GitToken, j.IssueTitle, branchName, prBody)
		}
		if e != nil && provider.IsPRAlreadyExists(e) {
			prURL, e = provider.FindExistingPR(j.RepoURL, j.GitToken, branchName, j.Provider)
		}
		return e
	}); err != nil {
		failJob(jobsDB, bot, j, fmt.Sprintf("create PR: %v", err), workspacePath)
		return
	}

	if err := storage.SetJobPRCreated(jobsDB, jobID, branchName, prURL); err != nil {
		log.Printf("set pr_created: %v", err)
	}

	msgText := fmt.Sprintf("[%s] PR ready — %s", j.Provider, prURL)
	if diffStat != "" {
		msgText += "\n\n" + diffStat
	}
	msgText += fmt.Sprintf("\n\nReply for follow-up or /done %d", jobID)

	msg := tgbotapi.NewMessage(j.ChatID, msgText)
	sent, _ := bot.Send(msg)
	if sent.MessageID != 0 {
		storage.SetJobNotifMsgID(jobsDB, jobID, int64(sent.MessageID))
	}

	storage.SetConversationState(jobsDB, j.ChatID, "telegram", "await_agent_input",
		map[string]interface{}{"job_id": jobID})

	go pollCI(ctx, j, branchName, jobsDB, bot)

	os.Remove(promptPath)
}

func pollCI(ctx context.Context, j *storage.Job, branch string, jobsDB *sql.DB, bot *tgbotapi.BotAPI) {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()
	deadline := time.NewTimer(20 * time.Minute)
	defer deadline.Stop()

	noneCount := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-deadline.C:
			return
		case <-ticker.C:
			status, err := provider.GetCIStatus(j.RepoURL, j.GitToken, branch, j.Provider)
			if err != nil {
				log.Printf("CI poll job %d: %v", j.ID, err)
				continue
			}
			switch status.State {
			case "none":
				noneCount++
				if noneCount >= 3 {
					return
				}
			case "pending":
				noneCount = 0
			case "success":
				notify(bot, j.ChatID, fmt.Sprintf("✅ [%s] CI passed — %s", j.Provider, j.PRURL))
				return
			case "failure":
				msg := fmt.Sprintf("❌ [%s] CI failed for job #%d", j.Provider, j.ID)
				if status.URL != "" {
					msg += "\n" + status.URL
				}
				notify(bot, j.ChatID, msg)
				return
			}
		}
	}
}

func notify(bot *tgbotapi.BotAPI, chatID int64, text string) {
	bot.Send(tgbotapi.NewMessage(chatID, text))
}

func failJob(jobsDB *sql.DB, bot *tgbotapi.BotAPI, j *storage.Job, errMsg, workspacePath string) {
	storage.SetJobFailed(jobsDB, j.ID, errMsg)
	msg := tgbotapi.NewMessage(j.ChatID, fmt.Sprintf("❌ Job %d failed: %s\n\nUse /logs %d to view details.", j.ID, errMsg, j.ID))
	bot.Send(msg)
	os.RemoveAll(workspacePath)
}

func WaitForJobs(timeoutSec int) {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Duration(timeoutSec) * time.Second):
	}
}
