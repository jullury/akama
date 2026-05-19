package bot

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/jullury/akama/internal/agent"
	"github.com/jullury/akama/internal/config"
	"github.com/jullury/akama/internal/git"
	"github.com/jullury/akama/internal/job"
	"github.com/jullury/akama/internal/metrics"
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
	msg := `Akama — Coding agent orchestration system

Akama is a coding agent orchestration system controlled via Telegram. Send it a GitHub or GitLab issue URL — Akama clones the repo, runs an agent to fix the issue, pushes a branch, and opens a pull request, then notifies you when done.

Repository
/connect — connect repository account
/connections — list saved repo connections
/delete_connection — delete a specific connection
/disconnect — delete all connections for this chat

Jobs
/newissue — create a new issue
/issues — list completed issues
/queue — list pending and running jobs
/status — show last 10 jobs
/logs — view agent output for a job
/retry — retry a failed job
/cancel — reset conversation state
/done — mark job done and clean up
/followup — continue working on a job

Settings
/config — configure git name, email, and model
/skills — browse and install skillhub.club skills
/update — update Akama server binary to the latest version
/update_agents — update agents to latest version
/version — show version information

Admin
/restart — restart the daemon (admin only)
/users — list authorized users
/add_user — add a user by Telegram user ID
/delete_user — delete a user by Telegram user ID

/start — welcome message
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

	// Store connections in conv data so toggle callbacks can look them up.
	connData := make([]map[string]interface{}, 0, len(conns))
	for _, c := range conns {
		branch := c.DefaultBranch
		if branch == "" {
			branch = "main"
		}
		connData = append(connData, map[string]interface{}{
			"id":             float64(c.ID),
			"repo_url":       c.RepoURL,
			"provider":       c.Provider,
			"token":          c.GitToken,
			"default_branch": branch,
		})
	}
	storage.SetConversationState(b.JobsDB, chatID, "telegram", "await_repo_select",
		map[string]interface{}{
			"connections":  connData,
			"selected_ids": []interface{}{},
		})

	msg := tgbotapi.NewMessage(chatID, "Select repositories for the new issue (tap to toggle, then press Done):")
	msg.ReplyMarkup = b.buildMultiRepoKeyboard(conns, nil)
	b.API.Send(msg)
}

func (b *Bot) buildMultiRepoKeyboard(conns []*storage.Connection, selectedIDs []int64) tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton
	selectedSet := make(map[int64]bool)
	for _, id := range selectedIDs {
		selectedSet[id] = true
	}
	for _, c := range conns {
		check := "☐"
		if selectedSet[c.ID] {
			check = "☑"
		}
		label := fmt.Sprintf("%s [%s] %s", check, c.Provider, c.RepoURL)
		data := fmt.Sprintf("newissue:toggle:%d", c.ID)
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(label, data),
		))
	}
	confirmLabel := "✓ Done selecting"
	if len(selectedIDs) > 0 {
		confirmLabel = fmt.Sprintf("✓ Done selecting (%d)", len(selectedIDs))
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData(confirmLabel, "newissue:confirm"),
	))
	return tgbotapi.NewInlineKeyboardMarkup(rows...)
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

func (b *Bot) showIssues(chatID int64, filterStatus string, page int) {
	const issuesPerPage = 5

	total, err := storage.CountJobsByChatIDAndStatus(b.JobsDB, chatID, filterStatus)
	if err != nil {
		b.send(chatID, fmt.Sprintf("Error: %v", err))
		return
	}

	if total == 0 {
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

	offset := page * issuesPerPage
	if offset >= total {
		page = 0
		offset = 0
	}

	jobs, err := storage.ListJobsByChatIDWithOffset(b.JobsDB, chatID, filterStatus, issuesPerPage, offset)
	if err != nil {
		b.send(chatID, fmt.Sprintf("Error: %v", err))
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Jobs (%s) - page %d/%d:\n", filterStatus, page+1, (total+issuesPerPage-1)/issuesPerPage))
	for _, j := range jobs {
		sb.WriteString(fmt.Sprintf("\n[#%d] %s — %s (%s)", j.ID, j.IssueTitle, j.Status, j.Provider))
		if j.PRURL != "" {
			sb.WriteString("\n  " + j.PRURL)
		}
		if j.ErrorMsg != "" {
			sb.WriteString("\n  Error: " + j.ErrorMsg)
		}
		sb.WriteString("\n")
	}

	var rows [][]tgbotapi.InlineKeyboardButton
	var navRow []tgbotapi.InlineKeyboardButton
	if page > 0 {
		navRow = append(navRow, tgbotapi.NewInlineKeyboardButtonData("← Back", fmt.Sprintf("issues:%s:page:%d", filterStatus, page-1)))
	}
	if offset+issuesPerPage < total {
		navRow = append(navRow, tgbotapi.NewInlineKeyboardButtonData("Next →", fmt.Sprintf("issues:%s:page:%d", filterStatus, page+1)))
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

const jobsPerPage = 10

func (b *Bot) handleStatus(chatID int64, page int) {

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

	b.send(chatID, fmt.Sprintf("Retrying job #%d: %s", jobID, j.IssueTitle))
	job.Run(b.ctx, jobID)
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

	// For grouped jobs, only remove workspace when all jobs in the group are done
	if j.GroupID != "" {
		active, _ := storage.CountActiveJobsByGroupID(b.JobsDB, j.GroupID)
		if active > 0 {
			b.send(chatID, fmt.Sprintf("Job %d marked as done. Other jobs in the same group are still active.", jobID))
			return
		}
	}

	os.RemoveAll(j.WorkspacePath)
	b.send(chatID, fmt.Sprintf("Job %d marked as done. Workspace cleaned up.", jobID))
}

func (b *Bot) doneAll(chatID int64) {
	jobs, err := storage.ListJobsByChatID(b.JobsDB, chatID, 200)
	if err != nil {
		b.send(chatID, fmt.Sprintf("Error: %v", err))
		return
	}

	cleanedPaths := make(map[string]bool)
	count := 0
	for _, j := range jobs {
		if j.Status == "pr_created" || j.Status == "failed" || j.Status == "done" {
			storage.SetJobStatus(b.JobsDB, j.ID, "done")

			// For grouped jobs, only remove workspace once
			if j.GroupID != "" {
				if !cleanedPaths[j.WorkspacePath] {
					active, _ := storage.CountActiveJobsByGroupID(b.JobsDB, j.GroupID)
					if active == 0 {
						if j.WorkspacePath != "" {
							os.RemoveAll(j.WorkspacePath)
							cleanedPaths[j.WorkspacePath] = true
						}
					}
				}
			} else if j.WorkspacePath != "" {
				if !cleanedPaths[j.WorkspacePath] {
					os.RemoveAll(j.WorkspacePath)
					cleanedPaths[j.WorkspacePath] = true
				}
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

func (b *Bot) handleSkills(chatID int64) {
	var sb strings.Builder
	sb.WriteString("Available skills from skillhub.club:\n\n")
	for i, s := range agent.BuiltinSkills {
		prefix := "  "
		suffix := ""
		if s.Required {
			prefix = "★ "
			suffix = " (always active)"
		}
		sb.WriteString(fmt.Sprintf("%s%d. %s — %s%s\n", prefix, i+1, s.Name, s.Description, suffix))
	}
	sb.WriteString("\n★ = required skill, always injected into every agent prompt.\nTap to install, or use + to add a custom skill by ID.")

	var rows [][]tgbotapi.InlineKeyboardButton
	for i, s := range agent.BuiltinSkills {
		label := s.Name
		if s.Required {
			label = "★ " + label
		}
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(label, fmt.Sprintf("skills:install:%d", i)),
		))
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("+ Custom skill by ID", "skills:custom"),
	))

	msg := tgbotapi.NewMessage(chatID, sb.String())
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	b.API.Send(msg)
}

func (b *Bot) installSkill(chatID int64, s agent.Skill) {
	if err := agent.InstallSkill(s); err != nil {
		b.send(chatID, fmt.Sprintf("Failed to install skill %s: %v", s.Name, err))
	} else {
		b.send(chatID, fmt.Sprintf("✅ Skill installed: %s", s.Name))
	}
}

func (b *Bot) handleUpdateAgents(chatID int64) {
	b.send(chatID, "Updating agents to latest version...")
	results := agent.UpdateAll()
	var msg strings.Builder
	msg.WriteString("Agent update results:\n")
	for name, err := range results {
		if err != nil {
			msg.WriteString(fmt.Sprintf("- %s: failed (%v)\n", name, err))
		} else {
			msg.WriteString(fmt.Sprintf("- %s: updated\n", name))
		}
	}
	b.send(chatID, msg.String())
}

type githubRelease struct {
	TagName string `json:"tag_name"`
}

func getLatestVersion() (string, error) {
	resp, err := http.Get("https://api.github.com/repos/jullury/akama/releases/latest")
	if err != nil {
		return "", fmt.Errorf("fetch release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	return strings.TrimPrefix(release.TagName, "v"), nil
}

func isNewerVersion(latest, current string) bool {
	latest = strings.TrimPrefix(latest, "v")
	current = strings.TrimPrefix(current, "v")

	if latest == current {
		return false
	}

	latestParts := strings.Split(latest, ".")
	currentParts := strings.Split(current, ".")

	for i := 0; i < len(latestParts) && i < len(currentParts); i++ {
		l, _ := strconv.Atoi(latestParts[i])
		c, _ := strconv.Atoi(currentParts[i])
		if l > c {
			return true
		}
		if l < c {
			return false
		}
	}

	return len(latestParts) > len(currentParts)
}

func (b *Bot) handleUsers(chatID int64) {
	if !storage.IsAdmin(b.JobsDB, chatID) {
		b.send(chatID, "Only the admin can list users.")
		return
	}
	users, err := storage.ListAuthorizedUsers(b.JobsDB)
	if err != nil {
		b.send(chatID, fmt.Sprintf("Error: %v", err))
		return
	}
	if len(users) == 0 {
		b.send(chatID, "No authorized users.")
		return
	}
	var sb strings.Builder
	sb.WriteString("Authorized users:\n")
	for _, u := range users {
		sb.WriteString(fmt.Sprintf("- %d (%s)\n", u.ChatID, u.Role))
	}
	b.send(chatID, sb.String())
}

func (b *Bot) handleUpdateCommand(chatID int64) {
	currentVersion := config.Version
	if currentVersion == "dev" {
		b.send(chatID, "Running dev build, cannot check for updates.")
		return
	}

	b.send(chatID, fmt.Sprintf("Current version: %s\nChecking for updates...", currentVersion))

	latest, err := getLatestVersion()
	if err != nil {
		b.send(chatID, fmt.Sprintf("Failed to check latest version: %v", err))
		return
	}

	if !isNewerVersion(latest, currentVersion) {
		b.send(chatID, "Already running the latest version.")
		return
	}

	b.send(chatID, fmt.Sprintf("New version available: %s\n\nDo you want to update now?", latest))

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Yes, update", "update:confirm"),
			tgbotapi.NewInlineKeyboardButtonData("No, cancel", "update:cancel"),
		),
	)
	msg := tgbotapi.NewMessage(chatID, "Press a button to continue:")
	msg.ReplyMarkup = keyboard
	b.API.Send(msg)
}

func (b *Bot) handleUpdateConfirm(chatID int64) {
	b.send(chatID, "Downloading and installing update...")

	if err := b.downloadUpdate(); err != nil {
		b.send(chatID, fmt.Sprintf("Update failed: %v", err))
		return
	}

	exePath, err := os.Executable()
	if err != nil {
		b.send(chatID, fmt.Sprintf("Could not determine binary path: %v", err))
		return
	}

	b.send(chatID, "Update installed. Restarting now...")

	if os.Getpid() != 1 {
		// Non-Docker: spawn a detached helper that waits for this process to
		// exit, pauses briefly for Telegram to drain the old long-poll connection,
		// then starts the new daemon. We cannot stop ourselves and then continue
		// in the same goroutine — sending SIGTERM to the daemon kills the goroutine
		// running this handler before any restart logic executes.
		script := fmt.Sprintf("while kill -0 %d 2>/dev/null; do sleep 1; done; sleep 3; '%s' start", os.Getpid(), exePath)
		helper := exec.Command("sh", "-c", script)
		helper.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if err := helper.Start(); err != nil {
			b.send(chatID, fmt.Sprintf("Failed to schedule restart: %v", err))
			return
		}
	}

	// Signal ourselves to shut down cleanly (closes all connections).
	// Docker: PID 1 exits → container restarts → entrypoint preserves the
	// updated binary on the volume.
	proc, _ := os.FindProcess(os.Getpid())
	proc.Signal(syscall.SIGTERM)
}

func (b *Bot) handleVersionCommand(chatID int64) {
	msg := fmt.Sprintf("Akama %s\nBuild time: %s\nPlatform: %s",
		config.Version, config.BuildTime, config.BuildPlatform)
	b.send(chatID, msg)
}

func (b *Bot) handleMetrics(chatID int64) {
	b.send(chatID, metrics.Summary())
}

func (b *Bot) handleRestartCommand(chatID int64) {
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Yes, restart", "restart:confirm"),
			tgbotapi.NewInlineKeyboardButtonData("No, cancel", "restart:cancel"),
		),
	)
	msg := tgbotapi.NewMessage(chatID, "Are you sure you want to restart the daemon?")
	msg.ReplyMarkup = keyboard
	b.API.Send(msg)
}

func (b *Bot) handleRestartConfirm(chatID int64) {
	b.send(chatID, "Restarting now...")

	exePath, err := os.Executable()
	if err != nil {
		b.send(chatID, fmt.Sprintf("Could not determine binary path: %v", err))
		return
	}

	if os.Getpid() != 1 {
		script := fmt.Sprintf("while kill -0 %d 2>/dev/null; do sleep 1; done; sleep 3; '%s' start", os.Getpid(), exePath)
		helper := exec.Command("sh", "-c", script)
		helper.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if err := helper.Start(); err != nil {
			b.send(chatID, fmt.Sprintf("Failed to schedule restart: %v", err))
			return
		}
	}

	proc, _ := os.FindProcess(os.Getpid())
	proc.Signal(syscall.SIGTERM)
}

func (b *Bot) downloadUpdate() error {
	goos := runtime.GOOS
	arch := runtime.GOARCH

	asset := fmt.Sprintf("akama-%s-%s", goos, arch)
	url := fmt.Sprintf("https://github.com/jullury/akama/releases/latest/download/%s", asset)

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("download failed: %d", resp.StatusCode)
	}

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}

	tmpPath := filepath.Join(os.TempDir(), "akama-update")
	out, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	out.Close()

	if err := os.Chmod(tmpPath, 0755); err != nil {
		return fmt.Errorf("chmod: %w", err)
	}

	if goos == "windows" {
		exePath = exePath + ".exe"
	}

	installDir := filepath.Dir(exePath)
	if err := checkWriteAccess(installDir); err != nil {
		return fmt.Errorf("cannot write to %s: %w", installDir, err)
	}

	if err := os.Rename(tmpPath, exePath); err != nil {
		cmd := exec.Command("mv", tmpPath, exePath)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("replace binary: %w", err)
		}
	}

	return nil
}

func checkWriteAccess(dir string) error {
	testFile := filepath.Join(dir, ".write-test")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		return err
	}
	os.Remove(testFile)
	return nil
}
