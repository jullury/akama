package bot

import (
	"fmt"
	"os"
	"strings"

	"github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/jullury/akama/internal/storage"
)

func (b *Bot) handleStart(chatID int64) {
	msg := `Welcome to Akama!

I can fix GitHub/GitLab issues using AI agents.

Commands:
/connect - Connect a repository
/connections - List saved connections
/disconnect - Remove all connections
/config - Set git user, email and AI model
/issues - List open PR jobs
/status - Show recent jobs
/done <id> - Mark job as done
/cancel - Reset conversation state

Send an issue URL to get started!`
	b.send(chatID, msg)
}

func (b *Bot) handleConfig(chatID int64) {
	cfg, err := storage.GetUserConfig(b.JobsDB, chatID)
	if err != nil {
		b.send(chatID, fmt.Sprintf("Error loading config: %v", err))
		return
	}
	gitName := cfg.GitName
	if gitName == "" {
		gitName = "(not set — default: Akama)"
	}
	gitEmail := cfg.GitEmail
	if gitEmail == "" {
		gitEmail = "(not set — default: akama@bot)"
	}
	model := cfg.AgentModel
	if model == "" {
		model = fmt.Sprintf("(not set — using %s)", b.Config.DefaultAgent)
	}

	text := fmt.Sprintf("Current settings:\n\nGit Name:  %s\nGit Email: %s\nAI Model:  %s\n\nWhat would you like to change?",
		gitName, gitEmail, model)

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Git Name", "config:git_name"),
			tgbotapi.NewInlineKeyboardButtonData("Git Email", "config:git_email"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("AI Model", "config:model"),
		),
	)
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyMarkup = keyboard
	b.API.Send(msg)
}

func (b *Bot) handleConnect(chatID int64) {
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("GitHub", "connect:github"),
			tgbotapi.NewInlineKeyboardButtonData("GitLab", "connect:gitlab"),
		),
	)
	msg := tgbotapi.NewMessage(chatID, "Select provider:")
	msg.ReplyMarkup = keyboard
	b.API.Send(msg)
}

func (b *Bot) handleConnections(chatID int64) {
	conns, err := storage.ListConnections(b.JobsDB, chatID)
	if err != nil {
		b.send(chatID, fmt.Sprintf("Error: %v", err))
		return
	}
	if len(conns) == 0 {
		b.send(chatID, "No saved connections. Use /connect to add one.")
		return
	}
	var sb strings.Builder
	sb.WriteString("Saved connections:\n")
	for _, c := range conns {
		sb.WriteString(fmt.Sprintf("- [%s] %s\n", c.Provider, c.RepoURL))
	}
	b.send(chatID, sb.String())
}

func (b *Bot) handleDisconnect(chatID int64) {
	if err := storage.DeleteAllConnections(b.JobsDB, chatID); err != nil {
		b.send(chatID, fmt.Sprintf("Error: %v", err))
		return
	}
	b.send(chatID, "All connections removed.")
}

func (b *Bot) handleIssues(chatID int64) {
	jobs, err := storage.ListJobs(b.JobsDB, 50)
	if err != nil {
		b.send(chatID, fmt.Sprintf("Error: %v", err))
		return
	}

	var filtered []*storage.Job
	for _, j := range jobs {
		if j.Status == "pr_created" {
			filtered = append(filtered, j)
		}
	}

	if len(filtered) == 0 {
		b.send(chatID, "No open PR jobs.")
		return
	}

	var sb strings.Builder
	sb.WriteString("Open PR jobs:\n")
	for _, j := range filtered {
		sb.WriteString(fmt.Sprintf("- [#%s] %s\n  %s\n", j.IssueID, j.IssueTitle, j.PRURL))
	}
	b.send(chatID, sb.String())
}

func (b *Bot) handleStatus(chatID int64) {
	jobs, err := storage.ListJobs(b.JobsDB, 5)
	if err != nil {
		b.send(chatID, fmt.Sprintf("Error: %v", err))
		return
	}
	if len(jobs) == 0 {
		b.send(chatID, "No jobs yet.")
		return
	}

	var sb strings.Builder
	sb.WriteString("Recent jobs:\n")
	for _, j := range jobs {
		sb.WriteString(fmt.Sprintf("- [#%d] %s - %s (%s)\n", j.ID, j.IssueTitle, j.Status, j.Provider))
	}
	b.send(chatID, sb.String())
}

func (b *Bot) handleDone(chatID int64, text string) {
	var jobID int64
	fmt.Sscanf(text, "/done %d", &jobID)
	if jobID == 0 {
		b.send(chatID, "Usage: /done <job_id>")
		return
	}

	j, err := storage.GetJob(b.JobsDB, jobID)
	if err != nil {
		b.send(chatID, fmt.Sprintf("Error: %v", err))
		return
	}
	if j == nil || j.ChatID != chatID {
		b.send(chatID, "Job not found.")
		return
	}

	storage.SetJobStatus(b.JobsDB, jobID, "done")
	os.RemoveAll(j.WorkspacePath)
	b.send(chatID, fmt.Sprintf("Job %d marked as done. Workspace cleaned up.", jobID))
}
