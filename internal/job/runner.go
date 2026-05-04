package job

import (
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

func Run(jobID int64, jobsDB *sql.DB, bot *tgbotapi.BotAPI, agentCfg *agent.Config, workspaceDir string) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		runJob(jobID, jobsDB, bot, agentCfg, workspaceDir)
	}()
}

func runJob(jobID int64, jobsDB *sql.DB, bot *tgbotapi.BotAPI, agentCfg *agent.Config, workspaceDir string) {
	j, err := storage.GetJob(jobsDB, jobID)
	if err != nil {
		log.Printf("get job %d: %v", jobID, err)
		return
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

	if err := git.Clone(j.RepoURL, j.GitToken, workspacePath); err != nil {
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

	rawOutput, err := agent.Run(j.Agent, j.AgentModel, workspacePath, promptPath, agentCfg)
	if err != nil {
		failJob(jobsDB, bot, j, fmt.Sprintf("agent run: %v", err), workspacePath)
		return
	}

	agentText := agent.ParseOutput(rawOutput)

	if agent.IsQuestion(agentText) {
		if err := storage.SetJobAwaitingInput(jobsDB, jobID, agentText); err != nil {
			log.Printf("set awaiting_input: %v", err)
		}
		// Set conversation state so any plain-text reply (not just a quoted reply) answers this job.
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

	branchName := fmt.Sprintf("akama/issue-%s", j.IssueID)
	if err := git.CommitPush(workspacePath, branchName, j.GitToken); err != nil {
		failJob(jobsDB, bot, j, fmt.Sprintf("commit/push: %v", err), workspacePath)
		return
	}

	notify(bot, j.ChatID, fmt.Sprintf("🔗 [%s] %s — creating pull request...", j.Provider, repoName))

	var prURL string
	switch j.Provider {
	case "github":
		prURL, err = provider.CreateGitHubPR(j.RepoURL, j.GitToken, j.IssueTitle, branchName, fmt.Sprintf("Fixes %s", j.IssueURL))
	case "gitlab":
		prURL, err = provider.CreateGitLabMR(j.RepoURL, j.GitToken, j.IssueTitle, branchName, fmt.Sprintf("Fixes %s", j.IssueURL))
	}
	if err != nil {
		failJob(jobsDB, bot, j, fmt.Sprintf("create PR: %v", err), workspacePath)
		return
	}

	if err := storage.SetJobPRCreated(jobsDB, jobID, branchName, prURL); err != nil {
		log.Printf("set pr_created: %v", err)
	}

	msg := tgbotapi.NewMessage(j.ChatID, fmt.Sprintf("[%s] PR ready — %s\n\nReply for follow-up or /done %d", j.Provider, prURL, jobID))
	sent, _ := bot.Send(msg)
	if sent.MessageID != 0 {
		storage.SetJobNotifMsgID(jobsDB, jobID, int64(sent.MessageID))
	}

	os.Remove(promptPath)
}

func notify(bot *tgbotapi.BotAPI, chatID int64, text string) {
	bot.Send(tgbotapi.NewMessage(chatID, text))
}

func failJob(jobsDB *sql.DB, bot *tgbotapi.BotAPI, j *storage.Job, errMsg, workspacePath string) {
	storage.SetJobFailed(jobsDB, j.ID, errMsg)
	msg := tgbotapi.NewMessage(j.ChatID, fmt.Sprintf("❌ Job %d failed: %s", j.ID, errMsg))
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
