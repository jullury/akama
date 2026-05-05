package job

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/jullury/akama/internal/agent"
	"github.com/jullury/akama/internal/git"
	"github.com/jullury/akama/internal/provider"
	"github.com/jullury/akama/internal/storage"
)

func RunFollowUp(ctx context.Context, jobID int64, userText string, jobsDB *sql.DB, bot *tgbotapi.BotAPI, agentCfg *agent.Config) {
	j, err := storage.GetJob(jobsDB, jobID)
	if err != nil {
		log.Printf("get job %d: %v", jobID, err)
		return
	}

	storage.SetJobStatus(jobsDB, jobID, "updating")

	userCfg, err := storage.GetUserConfig(jobsDB, j.ChatID)
	if err != nil {
		log.Printf("get user config: %v", err)
	}
	gitName, gitEmail := "", ""
	if userCfg != nil {
		gitName = userCfg.GitName
		gitEmail = userCfg.GitEmail
	}

	prompt := agent.BuildFollowUpPrompt(userText)
	promptPath, err := agent.WritePrompt(j.WorkspacePath, prompt)
	if err != nil {
		failFollowUp(jobsDB, bot, j, fmt.Sprintf("write prompt: %v", err))
		return
	}

	var followUpOutput string
	if err := withRetry(ctx, "agent run", 3, func() error {
		var e error
		followUpOutput, e = agent.Run(ctx, j.Agent, j.AgentModel, j.WorkspacePath, promptPath, agentCfg)
		return e
	}); err != nil {
		failFollowUp(jobsDB, bot, j, fmt.Sprintf("agent run: %v", err))
		return
	}

	followUpText := agent.ParseOutput(j.Agent, followUpOutput)
	if followUpText != "" {
		notifyChunked(bot, j.ChatID, fmt.Sprintf("📋 [%s] Agent output:", j.Provider), followUpText)
	}

	branchName := j.BranchName
	if branchName == "" {
		branchName = fmt.Sprintf("fix/issue-%s", j.IssueID)
	}

	commitMsg, prBody := agent.GenerateSummary(ctx, j.Agent, j.AgentModel, j.WorkspacePath, j.IssueURL, agentCfg)
	if err := git.Commit(j.WorkspacePath, branchName, j.GitToken, gitName, gitEmail, commitMsg); err != nil {
		failFollowUp(jobsDB, bot, j, fmt.Sprintf("git commit: %v", err))
		return
	}
	if err := withRetry(ctx, "git push", 3, func() error {
		return git.Push(j.WorkspacePath, branchName, j.GitToken)
	}); err != nil {
		failFollowUp(jobsDB, bot, j, fmt.Sprintf("git push: %v", err))
		return
	}

	// awaiting_input means no PR was created yet — create it now.
	if j.Status == "awaiting_input" || j.PRURL == "" {
		var prURL string
		if err := withRetry(ctx, "create PR", 3, func() error {
			var e error
			switch j.Provider {
			case "github":
				prURL, e = provider.CreateGitHubPR(j.RepoURL, j.GitToken, j.IssueTitle, branchName, j.DefaultBranch, prBody)
			case "gitlab":
				prURL, e = provider.CreateGitLabMR(j.RepoURL, j.GitToken, j.IssueTitle, branchName, j.DefaultBranch, prBody)
			}
			if e != nil && provider.IsPRAlreadyExists(e) {
				prURL, e = provider.FindExistingPR(j.RepoURL, j.GitToken, branchName, j.Provider)
			}
			return e
		}); err != nil {
			failFollowUp(jobsDB, bot, j, fmt.Sprintf("create PR: %v", err))
			return
		}
		storage.SetJobPRCreated(jobsDB, jobID, branchName, prURL)
		j.PRURL = prURL
	}

	storage.SetJobStatus(jobsDB, jobID, "pr_created")
	msg := tgbotapi.NewMessage(j.ChatID, fmt.Sprintf("[%s] Updated — %s\n\nReply for more or /done %d", j.Provider, j.PRURL, jobID))
	sent, _ := bot.Send(msg)
	if sent.MessageID != 0 {
		storage.SetJobNotifMsgID(jobsDB, jobID, int64(sent.MessageID))
	}

	storage.SetConversationState(jobsDB, j.ChatID, "telegram", "await_agent_input",
		map[string]interface{}{"job_id": jobID})

	os.Remove(promptPath)
}

func failFollowUp(jobsDB *sql.DB, bot *tgbotapi.BotAPI, j *storage.Job, errMsg string) {
	storage.SetJobStatus(jobsDB, j.ID, "pr_created")
	msg := tgbotapi.NewMessage(j.ChatID, fmt.Sprintf("❌ Follow-up failed: %s", errMsg))
	bot.Send(msg)
}
