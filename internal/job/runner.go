package job

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-telegram-bot-api/telegram-bot-api"

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

	workspacePath := filepath.Join(workspaceDir, fmt.Sprintf("%d", jobID))
	if err := storage.SetJobRunning(jobsDB, jobID, workspacePath); err != nil {
		log.Printf("set running: %v", err)
		return
	}

	msg := tgbotapi.NewMessage(j.ChatID, fmt.Sprintf("[%s] Working on: %s...", j.Provider, j.IssueTitle))
	bot.Send(msg)

	if err := git.Clone(j.RepoURL, j.GitToken, workspacePath); err != nil {
		failJob(jobsDB, bot, j, fmt.Sprintf("git clone: %v", err), workspacePath)
		return
	}

	prompt := agent.BuildPrompt(j.IssueTitle, j.IssueURL, j.IssueBody)
	promptPath, err := agent.WritePrompt(workspacePath, prompt)
	if err != nil {
		failJob(jobsDB, bot, j, fmt.Sprintf("write prompt: %v", err), workspacePath)
		return
	}

	_, err = agent.Run(j.Agent, j.AgentModel, workspacePath, promptPath, agentCfg)
	if err != nil {
		failJob(jobsDB, bot, j, fmt.Sprintf("agent run: %v", err), workspacePath)
		return
	}

	branchName := fmt.Sprintf("akama/issue-%s", j.IssueID)
	if err := git.CommitPush(workspacePath, branchName, j.GitToken); err != nil {
		failJob(jobsDB, bot, j, fmt.Sprintf("commit/push: %v", err), workspacePath)
		return
	}

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

	msg = tgbotapi.NewMessage(j.ChatID, fmt.Sprintf("[%s] PR ready — %s\n\nReply for follow-up or /done %d", j.Provider, prURL, jobID))
	sent, _ := bot.Send(msg)
	if sent.MessageID != 0 {
		storage.SetJobNotifMsgID(jobsDB, jobID, int64(sent.MessageID))
	}

	os.Remove(promptPath)
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
