package job

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	"github.com/go-telegram-bot-api/telegram-bot-api"

	"github.com/jullury/akama/internal/agent"
	"github.com/jullury/akama/internal/git"
	"github.com/jullury/akama/internal/storage"
)

func RunFollowUp(jobID int64, userText string, jobsDB *sql.DB, bot *tgbotapi.BotAPI, agentCfg *agent.Config) {
	j, err := storage.GetJob(jobsDB, jobID)
	if err != nil {
		log.Printf("get job %d: %v", jobID, err)
		return
	}

	storage.SetJobStatus(jobsDB, jobID, "updating")

	prompt := agent.BuildFollowUpPrompt(userText)
	promptPath, err := agent.WritePrompt(j.WorkspacePath, prompt)
	if err != nil {
		failFollowUp(jobsDB, bot, j, fmt.Sprintf("write prompt: %v", err))
		return
	}

	_, err = agent.Run(j.Agent, j.AgentModel, j.WorkspacePath, promptPath, agentCfg)
	if err != nil {
		failFollowUp(jobsDB, bot, j, fmt.Sprintf("agent run: %v", err))
		return
	}

	if err := git.CommitPush(j.WorkspacePath, j.BranchName, j.GitToken); err != nil {
		failFollowUp(jobsDB, bot, j, fmt.Sprintf("commit/push: %v", err))
		return
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
