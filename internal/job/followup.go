package job

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

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

	// Check if this is part of a group
	if j.GroupID != "" {
		runGroupedFollowUp(ctx, j, userText, jobsDB, bot, agentCfg)
		return
	}

	// Refresh git token from connection so follow-ups pick up refreshed tokens.
	if conn, err := storage.FindConnectionByRepo(jobsDB, j.ChatID, j.RepoURL); err == nil && conn != nil {
		if conn.GitToken != j.GitToken {
			log.Printf("[RunFollowUp] Refreshing token from connection for job %d", jobID)
			j.GitToken = conn.GitToken
			if err := storage.UpdateJobToken(jobsDB, jobID, conn.GitToken); err != nil {
				log.Printf("[RunFollowUp] Failed to update job token: %v", err)
			}
		}
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
	promptPath, err := agent.WritePrompt(j.WorkspacePath, j.Agent, prompt)
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
	msg := tgbotapi.NewMessage(j.ChatID, fmt.Sprintf("[%s] Updated — %s\n\nReply to the PR message or use /followup %d for more changes.", j.Provider, j.PRURL, jobID))
	sent, _ := bot.Send(msg)
	if sent.MessageID != 0 {
		storage.SetJobNotifMsgID(jobsDB, jobID, int64(sent.MessageID))
	}

	os.Remove(promptPath)
}

func runGroupedFollowUp(ctx context.Context, primary *storage.Job, userText string, jobsDB *sql.DB, bot *tgbotapi.BotAPI, agentCfg *agent.Config) {
	jobs, err := storage.FindJobsByGroupID(jobsDB, primary.GroupID)
	if err != nil || len(jobs) == 0 {
		log.Printf("runGroupedFollowUp: no jobs found for group %s: %v", primary.GroupID, err)
		return
	}

	chatID := primary.ChatID
	groupWorkspace := primary.WorkspacePath
	if groupWorkspace == "" && len(jobs) > 0 {
		groupWorkspace = jobs[0].WorkspacePath
	}

	// Set all jobs to updating
	for _, j := range jobs {
		storage.SetJobStatus(jobsDB, j.ID, "updating")

		// Refresh tokens
		if conn, err := storage.FindConnectionByRepo(jobsDB, j.ChatID, j.RepoURL); err == nil && conn != nil {
			if conn.GitToken != j.GitToken {
				j.GitToken = conn.GitToken
				storage.UpdateJobToken(jobsDB, j.ID, conn.GitToken)
			}
		}
	}

	userCfg, _ := storage.GetUserConfig(jobsDB, chatID)
	gitName, gitEmail := "", ""
	if userCfg != nil {
		gitName = userCfg.GitName
		gitEmail = userCfg.GitEmail
	}

	prompt := agent.BuildFollowUpPrompt(userText)
	promptPath, err := agent.WritePrompt(groupWorkspace, primary.Agent, prompt)
	if err != nil {
		for _, j := range jobs {
			failFollowUp(jobsDB, bot, j, fmt.Sprintf("write prompt: %v", err))
		}
		return
	}

	var followUpOutput string
	if err := withRetry(ctx, "agent run", 3, func() error {
		var e error
		followUpOutput, e = agent.Run(ctx, primary.Agent, primary.AgentModel, groupWorkspace, promptPath, agentCfg)
		return e
	}); err != nil {
		for _, j := range jobs {
			failFollowUp(jobsDB, bot, j, fmt.Sprintf("agent run: %v", err))
		}
		return
	}

	followUpText := agent.ParseOutput(primary.Agent, followUpOutput)
	if followUpText != "" {
		notifyChunked(bot, chatID, "📋 Agent output across all repositories:", followUpText)
	}

	// Process each repo's git operations
	var prURLs []string
	for _, j := range jobs {
		owner, repo, _ := git.OwnerRepo(j.RepoURL)
		dirName := owner + "-" + repo
		clonePath := filepath.Join(groupWorkspace, dirName)
		repoName := owner + "/" + repo

		branchName := j.BranchName
		if branchName == "" {
			branchName = fmt.Sprintf("fix/issue-%s", j.IssueID)
		}

		commitMsg, prBody := agent.GenerateSummary(ctx, j.Agent, j.AgentModel, clonePath, j.IssueURL, agentCfg)
		if err := git.Commit(clonePath, branchName, j.GitToken, gitName, gitEmail, commitMsg); err != nil {
			failFollowUp(jobsDB, bot, j, fmt.Sprintf("git commit: %v", err))
			continue
		}
		if err := withRetry(ctx, "git push", 3, func() error {
			return git.Push(clonePath, branchName, j.GitToken)
		}); err != nil {
			failFollowUp(jobsDB, bot, j, fmt.Sprintf("git push: %v", err))
			continue
		}

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
				continue
			}
			storage.SetJobPRCreated(jobsDB, j.ID, branchName, prURL)
			j.PRURL = prURL
		} else {
			storage.SetJobStatus(jobsDB, j.ID, "pr_created")
		}

		prURLs = append(prURLs, fmt.Sprintf("[%s] %s — %s", j.Provider, repoName, j.PRURL))
	}

	if len(prURLs) > 0 {
		msg := tgbotapi.NewMessage(chatID, "Updated across all repositories:\n"+strings.Join(prURLs, "\n"))
		sent, _ := bot.Send(msg)
		if sent.MessageID != 0 {
			storage.SetJobNotifMsgID(jobsDB, primary.ID, int64(sent.MessageID))
		}
	}

	storage.SetJobStatus(jobsDB, primary.ID, "pr_created")
	os.Remove(promptPath)
}

func failFollowUp(jobsDB *sql.DB, bot *tgbotapi.BotAPI, j *storage.Job, errMsg string) {
	storage.SetJobStatus(jobsDB, j.ID, "pr_created")
	storage.ResetConversation(jobsDB, j.ChatID, "telegram")
	// Check if the failure is auth-related and give specific guidance
	if provider.IsAuthError(fmt.Errorf("%s", errMsg)) {
		msg := tgbotapi.NewMessage(j.ChatID, fmt.Sprintf(
			"❌ Follow-up failed: authentication error.\n\n"+
				"Your token for %s may have expired or been revoked.\n"+
				"Use /connect to refresh your token, then use /followup %d to try again.",
			j.Provider, j.ID,
		))
		bot.Send(msg)
	} else {
		msg := tgbotapi.NewMessage(j.ChatID, fmt.Sprintf("❌ Follow-up failed: %s\n\nUse /followup %d to try again.", errMsg, j.ID))
		bot.Send(msg)
	}
}
