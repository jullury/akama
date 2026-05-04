package job

import (
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

func RunFollowUp(jobID int64, userText string, jobsDB *sql.DB, bot *tgbotapi.BotAPI, agentCfg *agent.Config) {
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

	rawOutput, err := agent.Run(j.Agent, j.AgentModel, j.WorkspacePath, promptPath, agentCfg)
	if err != nil {
		failFollowUp(jobsDB, bot, j, fmt.Sprintf("agent run: %v", err))
		return
	}

	branchName := j.BranchName
	if branchName == "" {
		branchName = fmt.Sprintf("fix/issue-%s", j.IssueID)
	}

	commitMsg := agent.BuildCommitMessage(agent.ParseOutput(rawOutput))
	if err := git.CommitPush(j.WorkspacePath, branchName, j.GitToken, gitName, gitEmail, commitMsg); err != nil {
		failFollowUp(jobsDB, bot, j, fmt.Sprintf("commit/push: %v", err))
		return
	}

	// awaiting_input means no PR was created yet — create it now.
	if j.Status == "awaiting_input" || j.PRURL == "" {
		prBody := agent.BuildPRDescription(agent.ParseOutput(rawOutput), j.IssueURL)
		var prURL string
		switch j.Provider {
		case "github":
			prURL, err = provider.CreateGitHubPR(j.RepoURL, j.GitToken, j.IssueTitle, branchName, prBody)
		case "gitlab":
			prURL, err = provider.CreateGitLabMR(j.RepoURL, j.GitToken, j.IssueTitle, branchName, prBody)
		}
		if err != nil {
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

	os.Remove(promptPath)
}

func failFollowUp(jobsDB *sql.DB, bot *tgbotapi.BotAPI, j *storage.Job, errMsg string) {
	storage.SetJobStatus(jobsDB, j.ID, "pr_created")
	msg := tgbotapi.NewMessage(j.ChatID, fmt.Sprintf("❌ Follow-up failed: %s", errMsg))
	bot.Send(msg)
}
