package bot

import (
	"fmt"
	"os"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/jullury/akama/internal/agent"
	"github.com/jullury/akama/internal/git"
	"github.com/jullury/akama/internal/job"
	"github.com/jullury/akama/internal/storage"
)

const modelsPerPage = 8

func buildModelKeyboard(agentName string, page int) (tgbotapi.InlineKeyboardMarkup, string) {
	models := agent.FetchModels(agentName)
	total := len(models)
	start := page * modelsPerPage
	if start >= total {
		start = 0
		page = 0
	}
	end := start + modelsPerPage
	if end > total {
		end = total
	}

	var rows [][]tgbotapi.InlineKeyboardButton
	for _, m := range models[start:end] {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(m, "config:model:set:"+m),
		))
	}

	// Navigation row
	var navRow []tgbotapi.InlineKeyboardButton
	if page > 0 {
		navRow = append(navRow, tgbotapi.NewInlineKeyboardButtonData(
			"← Back", fmt.Sprintf("config:model:page:%s:%d", agentName, page-1),
		))
	}
	if end < total {
		navRow = append(navRow, tgbotapi.NewInlineKeyboardButtonData(
			"Next →", fmt.Sprintf("config:model:page:%s:%d", agentName, page+1),
		))
	}
	if len(navRow) > 0 {
		rows = append(rows, navRow)
	}

	title := fmt.Sprintf("Select %s model (page %d/%d):", agentName, page+1, (total+modelsPerPage-1)/modelsPerPage)
	return tgbotapi.NewInlineKeyboardMarkup(rows...), title
}

func (b *Bot) handleStart(chatID int64) {
	b.handleHelp(chatID)
}

func (b *Bot) handleHelp(chatID int64) {
	msg := `Akama — AI-powered issue fixer

Send a GitHub or GitLab issue URL to start a job, or use these commands:

Repository
/connect — connect a repository via OAuth
/connections — list saved connections
/delete-connection — delete a single connection
/disconnect — remove all connections

Jobs
/newissue — create and immediately fix a new issue
/issues — list jobs (select filter with buttons)
/queue — show pending and running jobs
/status — show recent jobs
/logs — view agent output for a job (will prompt for ID)
/retry — retry a failed job (will prompt for ID)
/cancel — cancel a running job (will prompt for ID)
/done — mark job done and clean up workspace (will prompt for ID)
/done all — clean up all completed and failed jobs
/followup — continue working on a job with status 'pr_created' or 'updating' (will prompt for ID)

Settings
/config — set git name, email and AI model

/cancel — reset conversation state
/help — show this message`
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
	agentDisplay := cfg.Agent
	if agentDisplay == "" {
		agentDisplay = fmt.Sprintf("(not set — using %s)", b.Config.DefaultAgent)
	}
	model := cfg.AgentModel
	if model == "" {
		model = "(not set — using default)"
	}

	text := fmt.Sprintf("Current settings:\n\nGit Name:  %s\nGit Email: %s\nAgent:     %s\nAI Model:  %s\n\nWhat would you like to change?",
		gitName, gitEmail, agentDisplay, model)

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

func (b *Bot) handleNewIssue(chatID int64) {
	conns, err := storage.ListConnections(b.JobsDB, chatID)
	if err != nil || len(conns) == 0 {
		b.send(chatID, "No repositories connected. Use /connect to add one first.")
		return
	}
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, c := range conns {
		label := fmt.Sprintf("[%s] %s", c.Provider, c.RepoURL)
		data := fmt.Sprintf("newissue:conn:%d", c.ID)
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(label, data),
		))
	}
	msg := tgbotapi.NewMessage(chatID, "Select the repository for the new issue:")
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
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

func (b *Bot) handleDeleteConnection(chatID int64) {
	conns, err := storage.FindConnectionsByChat(b.JobsDB, chatID)
	if err != nil {
		b.send(chatID, fmt.Sprintf("Error: %v", err))
		return
	}
	if len(conns) == 0 {
		b.send(chatID, "No saved connections. Use /connect to add one.")
		return
	}
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, c := range conns {
		label := fmt.Sprintf("[%s] %s", c.Provider, c.RepoURL)
		data := fmt.Sprintf("connection:delete:%d", c.ID)
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(label, data),
		))
	}
	msg := tgbotapi.NewMessage(chatID, "Select a connection to delete:")
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	b.API.Send(msg)
}

func (b *Bot) showIssues(chatID int64, filterStatus string) {
	jobs, err := storage.ListJobsByChatID(b.JobsDB, chatID, 50)
	if err != nil {
		b.send(chatID, fmt.Sprintf("Error: %v", err))
		return
	}

	var filtered []*storage.Job
	for _, j := range jobs {
		switch filterStatus {
		case "", "open":
			if j.Status != "done" {
				filtered = append(filtered, j)
			}
		case "all":
			filtered = append(filtered, j)
		default:
			if j.Status == filterStatus {
				filtered = append(filtered, j)
			}
		}
	}

	if len(filtered) == 0 {
		switch filterStatus {
		case "all":
			b.send(chatID, "No jobs.")
		case "", "open":
			b.send(chatID, "No active jobs.")
		default:
			b.send(chatID, fmt.Sprintf("No %s jobs.", filterStatus))
		}
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Jobs (%s):\n", filterStatus))
	for _, j := range filtered {
		sb.WriteString(fmt.Sprintf("\n[#%d] %s — %s (%s)", j.ID, j.IssueTitle, j.Status, j.Provider))
		if j.PRURL != "" {
			sb.WriteString("\n  " + j.PRURL)
		}
		if j.ErrorMsg != "" {
			sb.WriteString("\n  Error: " + j.ErrorMsg)
		}
		sb.WriteString("\n")
	}
	b.send(chatID, sb.String())
}

func (b *Bot) handleQueue(chatID int64) {
	jobs, err := storage.ListJobsByChatID(b.JobsDB, chatID, 20)
	if err != nil {
		b.send(chatID, fmt.Sprintf("Error: %v", err))
		return
	}

	var active []*storage.Job
	for _, j := range jobs {
		if j.Status == "pending" || j.Status == "running" || j.Status == "awaiting_input" {
			active = append(active, j)
		}
	}

	if len(active) == 0 {
		b.send(chatID, "No jobs in queue.")
		return
	}

	var sb strings.Builder
	sb.WriteString("Active jobs:\n")
	for _, j := range active {
		sb.WriteString(fmt.Sprintf("- [#%d] %s — %s (%s)\n", j.ID, j.IssueTitle, j.Status, j.Provider))
	}
	b.send(chatID, sb.String())
}

const jobsPerPage = 5

func (b *Bot) handleStatus(chatID int64) {
	page := 0

	total, err := storage.CountAllJobs(b.JobsDB)
	if err != nil {
		b.send(chatID, fmt.Sprintf("Error: %v", err))
		return
	}

	if total == 0 {
		b.send(chatID, "No jobs yet.")
		return
	}

	offset := page * jobsPerPage
	if offset >= total {
		page = 0
		offset = 0
	}

	jobs, err := storage.ListJobs(b.JobsDB, jobsPerPage, offset)
	if err != nil {
		b.send(chatID, fmt.Sprintf("Error: %v", err))
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Recent jobs (page %d/%d):\n", page+1, (total+jobsPerPage-1)/jobsPerPage))
	for _, j := range jobs {
		repoDisplay := j.RepoURL
		if owner, repo, err := git.OwnerRepo(j.RepoURL); err == nil {
			repoDisplay = owner + "/" + repo
		}
		sb.WriteString(fmt.Sprintf("- [#%d] %s - %s - %s (%s)\n", j.ID, j.IssueTitle, repoDisplay, j.Status, j.Provider))
	}

	var rows [][]tgbotapi.InlineKeyboardButton
	var navRow []tgbotapi.InlineKeyboardButton
	if page > 0 {
		navRow = append(navRow, tgbotapi.NewInlineKeyboardButtonData("← Back", fmt.Sprintf("status:page:%d", page-1)))
	}
	if offset+jobsPerPage < total {
		navRow = append(navRow, tgbotapi.NewInlineKeyboardButtonData("Next →", fmt.Sprintf("status:page:%d", page+1)))
	}
	if len(navRow) > 0 {
		rows = append(rows, navRow)
	}

	msg := tgbotapi.NewMessage(chatID, sb.String())
	if len(rows) > 0 {
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	}
	b.API.Send(msg)
}

func (b *Bot) showLogs(chatID int64, jobID int64) {
	j, err := storage.GetJob(b.JobsDB, jobID)
	if err != nil || j == nil || j.ChatID != chatID {
		b.send(chatID, "Job not found.")
		return
	}

	if j.AgentOutput == "" {
		if j.ErrorMsg != "" {
			b.send(chatID, fmt.Sprintf("Job #%d failed:\n\n%s", jobID, j.ErrorMsg))
		} else {
			b.send(chatID, fmt.Sprintf("Job #%d has no output yet (status: %s).", jobID, j.Status))
		}
		return
	}

	output := agent.ParseOutput(j.Agent, j.AgentOutput)
	if output == "" {
		output = j.AgentOutput
	}
	const maxLen = 4000
	if len(output) > maxLen {
		output = output[:maxLen] + "\n...[truncated]"
	}
	b.send(chatID, fmt.Sprintf("Job #%d output:\n\n%s", jobID, output))
}

func (b *Bot) retryJob(chatID int64, jobID int64) {
	j, err := storage.GetJob(b.JobsDB, jobID)
	if err != nil || j == nil || j.ChatID != chatID {
		b.send(chatID, "Job not found.")
		return
	}
	if j.Status != "failed" {
		b.send(chatID, fmt.Sprintf("Job #%d is not failed (status: %s). Only failed jobs can be retried.", jobID, j.Status))
		return
	}

	if err := storage.SetJobStatus(b.JobsDB, jobID, "pending"); err != nil {
		b.send(chatID, fmt.Sprintf("Failed to reset job: %v", err))
		return
	}

	agentCfg := &agent.Config{
		APIKeys:     b.Config.APIKeys,
		TimeoutMins: b.Config.AgentTimeoutMins,
	}
	b.send(chatID, fmt.Sprintf("Retrying job #%d: %s", jobID, j.IssueTitle))
	job.Run(b.ctx, jobID, b.JobsDB, b.API, agentCfg, b.Config.WorkspaceDir)
}

func (b *Bot) doneJob(chatID int64, jobID int64) {
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
	storage.ResetConversation(b.JobsDB, chatID, "telegram")
	os.RemoveAll(j.WorkspacePath)
	b.send(chatID, fmt.Sprintf("Job %d marked as done. Workspace cleaned up.", jobID))
}

func (b *Bot) doneAll(chatID int64) {
	jobs, err := storage.ListJobsByChatID(b.JobsDB, chatID, 200)
	if err != nil {
		b.send(chatID, fmt.Sprintf("Error: %v", err))
		return
	}
	count := 0
	for _, j := range jobs {
		if j.Status == "pr_created" || j.Status == "failed" || j.Status == "done" {
			storage.SetJobStatus(b.JobsDB, j.ID, "done")
			if j.WorkspacePath != "" {
				os.RemoveAll(j.WorkspacePath)
			}
			count++
		}
	}
	storage.ResetConversation(b.JobsDB, chatID, "telegram")
	b.send(chatID, fmt.Sprintf("Cleaned up %d jobs.", count))
}

func (b *Bot) startFollowUp(chatID int64, jobID int64) {
	j, err := storage.GetJob(b.JobsDB, jobID)
	if err != nil || j == nil || j.ChatID != chatID {
		b.send(chatID, "Job not found.")
		return
	}

	if j.Status != "pr_created" && j.Status != "updating" {
		b.send(chatID, fmt.Sprintf("Follow-up only available for jobs with status 'pr_created' or 'updating'. Current status: %s", j.Status))
		return
	}

	storage.SetConversationState(b.JobsDB, chatID, "telegram", "await_followup", map[string]interface{}{"job_id": float64(jobID)})
	b.send(chatID, fmt.Sprintf("Job #%d is ready for follow-up. Send your message to continue working on it.", jobID))
}

func (b *Bot) handleCancelJob(chatID int64, jobID int64) {
	j, err := storage.GetJob(b.JobsDB, jobID)
	if err != nil || j == nil || j.ChatID != chatID {
		b.send(chatID, "Job not found.")
		return
	}
	if j.Status != "running" && j.Status != "pending" && j.Status != "awaiting_input" {
		b.send(chatID, fmt.Sprintf("Job #%d is not active (status: %s).", jobID, j.Status))
		return
	}
	job.CancelJob(jobID)
	storage.SetJobFailed(b.JobsDB, jobID, "cancelled by user")
	storage.ResetConversation(b.JobsDB, chatID, "telegram")
	b.send(chatID, fmt.Sprintf("Job #%d cancelled.", jobID))
}
