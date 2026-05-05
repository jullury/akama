package bot

import (
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/jullury/akama/internal/agent"
	"github.com/jullury/akama/internal/config"
	"github.com/jullury/akama/internal/job"
	"github.com/jullury/akama/internal/provider"
	"github.com/jullury/akama/internal/storage"
)

var (
	githubIssueRegex = regexp.MustCompile(`github\.com/[^/]+/[^/]+/issues/\d+`)
	gitlabIssueRegex = regexp.MustCompile(`gitlab\.com/[^/]+/[^/]+(?:/-)?/(?:issues|work_items)/\d+`)
)

func (b *Bot) handleMessage(msg *tgbotapi.Message) {
	chatID := msg.Chat.ID

	if msg.ReplyToMessage != nil {
		b.handleReply(chatID, msg)
		return
	}

	text := strings.TrimSpace(msg.Text)

	switch {
	case strings.HasPrefix(text, "/start"):
		b.handleStart(chatID)
	case strings.HasPrefix(text, "/help"):
		b.handleHelp(chatID)
	case strings.HasPrefix(text, "/config"):
		b.handleConfig(chatID)
	case strings.HasPrefix(text, "/newissue"):
		b.handleNewIssue(chatID)
	case strings.HasPrefix(text, "/connections"):
		b.handleConnections(chatID)
	case strings.HasPrefix(text, "/connection"):
		b.handleConnection(chatID, text)
	case strings.HasPrefix(text, "/connect"):
		b.handleConnect(chatID)
	case strings.HasPrefix(text, "/disconnect"):
		b.handleDisconnect(chatID)
	case strings.HasPrefix(text, "/issues"):
		b.handleIssues(chatID, text)
	case strings.HasPrefix(text, "/queue"):
		b.handleQueue(chatID)
	case strings.HasPrefix(text, "/status"):
		b.handleStatus(chatID, text)
	case strings.HasPrefix(text, "/logs"):
		b.handleLogs(chatID, text)
	case strings.HasPrefix(text, "/retry"):
		b.handleRetry(chatID, text)
	case strings.HasPrefix(text, "/done"):
		b.handleDone(chatID, text)
	case strings.HasPrefix(text, "/followup"):
		b.handleFollowUp(chatID, text)
	case strings.HasPrefix(text, "/cancel"):
		var jobID int64
		if n, _ := fmt.Sscanf(text, "/cancel %d", &jobID); n == 1 && jobID != 0 {
			b.handleCancelJob(chatID, jobID)
		} else {
			storage.ResetConversation(b.JobsDB, chatID, "telegram")
			b.send(chatID, "Conversation reset.")
		}
	default:
		b.handleText(chatID, text)
	}
}

func (b *Bot) handleReply(chatID int64, msg *tgbotapi.Message) {
	replyMsgID := msg.ReplyToMessage.MessageID

	j, err := storage.GetJobByNotifMsgID(b.JobsDB, int64(replyMsgID))
	if err != nil {
		log.Printf("lookup job by notif: %v", err)
		return
	}
	if j == nil {
		return
	}

	if j.Status == "pr_created" || j.Status == "updating" {
		agentCfg := &agent.Config{
			APIKeys:      b.Config.APIKeys,
			TimeoutMins:   b.Config.AgentTimeoutMins,
		}
		go job.RunFollowUp(b.ctx, j.ID, msg.Text, b.JobsDB, b.API, agentCfg)
		b.send(chatID, fmt.Sprintf("[%s] Updating...", j.Provider))
	} else {
		b.send(chatID, fmt.Sprintf("Follow-up only available for jobs with status 'pr_created' or 'updating'. Current status: %s", j.Status))
	}
}

func (b *Bot) handleCallback(query *tgbotapi.CallbackQuery) {
	// Answer first — must happen regardless of outcome so Telegram clears the spinner.
	defer func() {
		if _, err := b.API.Request(tgbotapi.NewCallback(query.ID, "")); err != nil {
			log.Printf("callback: answer query: %v", err)
		}
	}()

	if query.Message == nil {
		log.Printf("callback: no message attached")
		return
	}
	chatID := query.Message.Chat.ID
	data := query.Data
	log.Printf("callback: chatID=%d data=%q", chatID, data)

	switch data {
	case "connect:github":
		b.startDeviceFlow(chatID, "github")
	case "connect:gitlab":
		b.startDeviceFlow(chatID, "gitlab")
	case "config:git_name":
		storage.SetConversationState(b.JobsDB, chatID, "telegram", "await_config", map[string]interface{}{"field": "git_name"})
		b.send(chatID, "Enter your git commit name:")
	case "config:git_email":
		storage.SetConversationState(b.JobsDB, chatID, "telegram", "await_config", map[string]interface{}{"field": "git_email"})
		b.send(chatID, "Enter your git commit email:")
	case "config:model":
		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Claude", "config:model:claude"),
				tgbotapi.NewInlineKeyboardButtonData("OpenCode", "config:model:opencode"),
			),
		)
		msg := tgbotapi.NewMessage(chatID, "Select AI provider:")
		msg.ReplyMarkup = keyboard
		b.API.Send(msg)
	case "config:model:claude":
		cfg, _ := storage.GetUserConfig(b.JobsDB, chatID)
		if cfg == nil {
			cfg = &storage.UserConfig{ChatID: chatID}
		}
		cfg.Agent = "claude"
		storage.SetUserConfig(b.JobsDB, cfg)
		keyboard, title := buildModelKeyboard("claude", 0)
		msg := tgbotapi.NewMessage(chatID, title)
		msg.ReplyMarkup = keyboard
		b.API.Send(msg)
	case "config:model:opencode":
		cfg, _ := storage.GetUserConfig(b.JobsDB, chatID)
		if cfg == nil {
			cfg = &storage.UserConfig{ChatID: chatID}
		}
		cfg.Agent = "opencode"
		storage.SetUserConfig(b.JobsDB, cfg)
		keyboard, title := buildModelKeyboard("opencode", 0)
		msg := tgbotapi.NewMessage(chatID, title)
		msg.ReplyMarkup = keyboard
		b.API.Send(msg)
	default:
		if connIDStr, ok := strings.CutPrefix(data, "newissue:conn:"); ok {
			var connID int64
			fmt.Sscanf(connIDStr, "%d", &connID)
			conn, err := storage.GetConnectionByID(b.JobsDB, connID)
			if err != nil || conn == nil {
				b.send(chatID, "Repository not found.")
				return
			}
			storage.SetConversationState(b.JobsDB, chatID, "telegram", "await_issue_desc",
				map[string]interface{}{
					"repo_url": conn.RepoURL,
					"provider": conn.Provider,
					"token":    conn.GitToken,
				})
			b.send(chatID, fmt.Sprintf("Describe the issue for *%s*:\n\nFirst line = title, rest = description.", conn.RepoURL))
			return
		}
		if rest, ok := strings.CutPrefix(data, "config:model:page:"); ok {
			// format: config:model:page:<agentName>:<page>
			parts := strings.SplitN(rest, ":", 2)
			if len(parts) == 2 {
				agentName := parts[0]
				var page int
				fmt.Sscanf(parts[1], "%d", &page)
				keyboard, title := buildModelKeyboard(agentName, page)
				msg := tgbotapi.NewMessage(chatID, title)
				msg.ReplyMarkup = keyboard
				b.API.Send(msg)
			}
			return
		}
		if model, ok := strings.CutPrefix(data, "config:model:set:"); ok {
			cfg, _ := storage.GetUserConfig(b.JobsDB, chatID)
			if cfg == nil {
				cfg = &storage.UserConfig{ChatID: chatID}
			}
			cfg.AgentModel = model
			if err := storage.SetUserConfig(b.JobsDB, cfg); err != nil {
				log.Printf("set user config: %v", err)
				b.send(chatID, "Failed to save model.")
			} else {
				b.send(chatID, fmt.Sprintf("AI model set to: %s", model))
			}
			return
		}
		if rest, ok := strings.CutPrefix(data, "status:page:"); ok {
			var page int
			fmt.Sscanf(rest, "%d", &page)
			b.handleStatus(chatID, fmt.Sprintf("/status %d", page))
			return
		}
		if connIDStr, ok := strings.CutPrefix(data, "connection:delete:"); ok {
			var connID int
			fmt.Sscanf(connIDStr, "%d", &connID)
			if err := storage.DeleteConnection(b.JobsDB, connID); err != nil {
				log.Printf("delete connection: %v", err)
				b.send(chatID, "Failed to delete connection.")
				return
			}
			if err := storage.ResetConversation(b.JobsDB, chatID, "telegram"); err != nil {
				log.Printf("reset conversation: %v", err)
			}
			b.send(chatID, "Connection deleted.")
			return
		}
		log.Printf("callback: unhandled data: %q", data)
	}
}

func (b *Bot) handleText(chatID int64, text string) {
	conv, err := storage.GetConversation(b.JobsDB, chatID, "telegram")
	if err != nil {
		log.Printf("get conversation: %v", err)
		return
	}

	switch conv.State {
	case "await_issue_desc":
		repoURL, _ := conv.Data["repo_url"].(string)
		providerName, _ := conv.Data["provider"].(string)
		token, _ := conv.Data["token"].(string)
		storage.ResetConversation(b.JobsDB, chatID, "telegram")

		lines := strings.SplitN(strings.TrimSpace(text), "\n", 2)
		title := strings.TrimSpace(lines[0])
		body := ""
		if len(lines) > 1 {
			body = strings.TrimSpace(lines[1])
		}
		if title == "" {
			b.send(chatID, "Issue title cannot be empty. Use /newissue to try again.")
			return
		}

		var issueURL string
		var err error
		switch providerName {
		case "github":
			issueURL, err = provider.CreateGitHubIssue(repoURL, token, title, body)
		case "gitlab":
			issueURL, err = provider.CreateGitLabIssue(repoURL, token, title, body)
		}
		if err != nil {
			if provider.IsAuthError(err) {
				b.send(chatID, fmt.Sprintf(
					"❌ Authentication failed for %s. Your token may have expired or been revoked.\n\n"+
						"Use /connect to refresh your token, then /newissue to try again.",
					providerName,
				))
				return
			}
			b.send(chatID, fmt.Sprintf("Failed to create issue: %v", err))
			return
		}
		b.send(chatID, fmt.Sprintf("Issue created: %s\n\nProcessing it now...", issueURL))
		b.processIssue(chatID, issueURL, token)
	case "await_config":
		field, _ := conv.Data["field"].(string)
		storage.ResetConversation(b.JobsDB, chatID, "telegram")
		cfg, _ := storage.GetUserConfig(b.JobsDB, chatID)
		if cfg == nil {
			cfg = &storage.UserConfig{ChatID: chatID}
		}
		switch field {
		case "git_name":
			cfg.GitName = text
			b.send(chatID, fmt.Sprintf("Git name set to: %s", text))
		case "git_email":
			cfg.GitEmail = text
			b.send(chatID, fmt.Sprintf("Git email set to: %s", text))
		case "model":
			cfg.AgentModel = text
			b.send(chatID, fmt.Sprintf("AI model set to: %s", text))
		}
		if err := storage.SetUserConfig(b.JobsDB, cfg); err != nil {
			log.Printf("set user config: %v", err)
			b.send(chatID, "Failed to save config.")
		}
	case "await_agent_input":
		jobIDFloat, _ := conv.Data["job_id"].(float64)
		jobID := int64(jobIDFloat)
		j, err := storage.GetJob(b.JobsDB, jobID)
		if err != nil || j == nil {
			storage.ResetConversation(b.JobsDB, chatID, "telegram")
			b.send(chatID, "Job not found.")
			return
		}
		if j.Status != "pr_created" && j.Status != "updating" {
			b.send(chatID, fmt.Sprintf("Follow-up only available for jobs with status 'pr_created' or 'updating'. Current status: %s", j.Status))
			return
		}
		storage.SetConversationState(b.JobsDB, chatID, "telegram", "await_followup", conv.Data)
		agentCfg := &agent.Config{
			APIKeys:      b.Config.APIKeys,
			TimeoutMins:   b.Config.AgentTimeoutMins,
		}
		go job.RunFollowUp(b.ctx, jobID, text, b.JobsDB, b.API, agentCfg)
		b.send(chatID, "Got it, continuing work on the issue...")
	case "await_followup":
		jobIDFloat, _ := conv.Data["job_id"].(float64)
		jobID := int64(jobIDFloat)
		j, _ := storage.GetJob(b.JobsDB, jobID)
		if j != nil && j.Status == "updating" {
			b.send(chatID, "A follow-up is already in progress. Please wait for it to complete.")
		} else {
			storage.ResetConversation(b.JobsDB, chatID, "telegram")
		}
	case "idle":
		if isIssueURL(text) {
			b.processIssue(chatID, text, "")
		} else {
			b.send(chatID, "Send an issue URL or use /connect to add a repository.")
		}
	case "await_repo":
		// Device flow already obtained the token; user is now supplying the repo URL.
		providerName, _ := conv.Data["provider"].(string)
		token, _ := conv.Data["token"].(string)
		if providerName == "" || token == "" {
			storage.ResetConversation(b.JobsDB, chatID, "telegram")
			b.send(chatID, "Something went wrong. Use /connect to start over.")
			return
		}
		repoURL := extractRepoURL(strings.TrimSpace(text))
		defaultBranch := provider.GetDefaultBranch(repoURL, token, providerName)

		// Update existing connection if one exists for this repo, otherwise create new.
		existing, _ := storage.FindConnectionByRepo(b.JobsDB, chatID, repoURL)
		if existing != nil {
			if err := storage.UpdateConnectionToken(b.JobsDB, chatID, repoURL, token); err != nil {
				log.Printf("update connection token: %v", err)
				b.send(chatID, "Failed to update connection. Please try again.")
				return
			}
			if defaultBranch != "" {
				storage.UpdateConnectionDefaultBranch(b.JobsDB, chatID, repoURL, defaultBranch)
			}
			b.send(chatID, fmt.Sprintf("✅ Token refreshed for %s! Send an issue URL to start a job.", repoURL))
		} else {
			if err := storage.SaveConnection(b.JobsDB, chatID, providerName, repoURL, token, defaultBranch); err != nil {
				log.Printf("save connection: %v", err)
				b.send(chatID, "Failed to save connection. Please try again.")
				return
			}
			b.send(chatID, fmt.Sprintf("Connected! Send a %s issue URL to start a job.", providerName))
		}
		storage.ResetConversation(b.JobsDB, chatID, "telegram")
	case "await_branch_confirm":
		issueURL, _ := conv.Data["issue_url"].(string)
		gitToken, _ := conv.Data["git_token"].(string)
		detectedBranch, _ := conv.Data["detected_branch"].(string)
		repoURL, _ := conv.Data["repo_url"].(string)
		storage.ResetConversation(b.JobsDB, chatID, "telegram")

		chosenBranch := strings.TrimSpace(text)
		if chosenBranch == "" {
			chosenBranch = detectedBranch
		}
		log.Printf("[await_branch_confirm] Using branch %q for %s", chosenBranch, repoURL)
		if err := storage.UpdateConnectionDefaultBranch(b.JobsDB, chatID, repoURL, chosenBranch); err != nil {
			log.Printf("[await_branch_confirm] Failed to persist branch: %v", err)
		}
		b.continueIssueProcessing(chatID, issueURL, gitToken, chosenBranch)
	}
}

func (b *Bot) processIssue(chatID int64, issueURL, gitToken string) {
	providerName := detectProvider(issueURL)
	if providerName == "" {
		b.send(chatID, "Unsupported provider. Use GitHub or GitLab issue URL.")
		return
	}

	lookupURL := extractRepoURL(issueURL)
	log.Printf("[processIssue] Looking up connection for chatID=%d, repoURL=%q", chatID, lookupURL)
	conn, _ := storage.FindConnectionByRepo(b.JobsDB, chatID, lookupURL)

	var token string
	if gitToken != "" {
		token = gitToken
	} else if conn != nil {
		log.Printf("[processIssue] Found connection, token prefix: %s...", conn.GitToken[:10])
		token = conn.GitToken
	} else {
		log.Printf("[processIssue] No connection found for repoURL=%q", lookupURL)
	}

	if token == "" {
		b.send(chatID, "No git token found. Use /connect to add a repository first.")
		return
	}

	if existing := storage.FindActiveJobByIssue(b.JobsDB, chatID, issueURL); existing != nil {
		b.send(chatID, fmt.Sprintf("⚠️ Job #%d is already working on this issue (status: %s).", existing.ID, existing.Status))
		return
	}

	defaultBranch := "main"
	if conn != nil && conn.DefaultBranch != "" {
		defaultBranch = conn.DefaultBranch
	}

	jobCount, _ := storage.CountJobsByRepo(b.JobsDB, chatID, lookupURL)
	if jobCount == 0 && conn != nil && conn.DefaultBranch != "" {
		log.Printf("[processIssue] First issue for repo, prompting for branch confirmation")
		data := map[string]interface{}{
			"issue_url":       issueURL,
			"git_token":       token,
			"detected_branch": defaultBranch,
			"repo_url":        lookupURL,
		}
		storage.SetConversationState(b.JobsDB, chatID, "telegram", "await_branch_confirm", data)
		b.send(chatID, fmt.Sprintf(
			"This is the first issue for this repository.\n"+
				"Detected default branch: *%s*\n\n"+
				"Send a different branch name to override, or press Enter to use the detected branch.",
			defaultBranch,
		))
		return
	}

	b.continueIssueProcessing(chatID, issueURL, token, defaultBranch)
}

func (b *Bot) continueIssueProcessing(chatID int64, issueURL, gitToken, defaultBranch string) {
	providerName := detectProvider(issueURL)
	repoURL := extractRepoURL(issueURL)

	var title, body, issueID string
	var err error

	switch providerName {
	case "github":
		issue, e := provider.FetchGitHubIssue(issueURL, gitToken)
		if e != nil {
			err = e
		} else {
			title = issue.Title
			body = issue.Body
			issueID = fmt.Sprintf("%d", issue.Number)
		}
	case "gitlab":
		issue, e := provider.FetchGitLabIssue(issueURL, gitToken)
		if e != nil {
			err = e
		} else {
			title = issue.Title
			body = issue.Description
			issueID = fmt.Sprintf("%d", issue.IID)
		}
	}

	if err != nil {
		if provider.IsAuthError(err) {
			b.send(chatID, fmt.Sprintf(
				"❌ Authentication failed for %s. Your token may have expired or been revoked.\n\n"+
					"Use /connect to refresh your token for this repository.",
				providerName,
			))
			return
		}
		b.send(chatID, fmt.Sprintf("Failed to fetch issue: %v", err))
		return
	}
	if issueID == "" {
		b.send(chatID, "Failed to parse issue ID from fetched issue.")
		return
	}

	j := &storage.Job{
		ChatID:        chatID,
		IssueID:       issueID,
		IssueTitle:    title,
		IssueBody:     body,
		IssueURL:      issueURL,
		RepoURL:       repoURL,
		Provider:      providerName,
		GitToken:      gitToken,
		Agent:         b.Config.DefaultAgent,
		AgentModel:    b.Config.DefaultModel,
		DefaultBranch: defaultBranch,
	}

	jobID, err := storage.CreateJob(b.JobsDB, j)
	if err != nil {
		b.send(chatID, fmt.Sprintf("Failed to create job: %v", err))
		return
	}

	agentCfg := &agent.Config{
		APIKeys:      b.Config.APIKeys,
		TimeoutMins:   b.Config.AgentTimeoutMins,
	}
	job.Run(b.ctx, jobID, b.JobsDB, b.API, agentCfg, b.Config.WorkspaceDir)
}

func (b *Bot) send(chatID int64, text string) {
	if _, err := b.API.Send(tgbotapi.NewMessage(chatID, text)); err != nil {
		log.Printf("send to %d: %v", chatID, err)
	}
}

func (b *Bot) startDeviceFlow(chatID int64, p string) {
	var (
		deviceCode      string
		userCode        string
		verificationURI string
		interval        int
		pollFn          func(clientID, clientSecret, deviceCode string) (string, error, int)
		clientID        string
		clientSecret    string
	)

	switch p {
	case "github":
		clientID = config.GitHubClientID
		clientSecret = config.GitHubClientSecret
		if clientID == "" {
			b.send(chatID, "GitHub OAuth App not configured. Rebuild with GitHub credentials.")
			return
		}
		dc, err := provider.StartGitHubDeviceFlow(clientID)
		if err != nil {
			log.Printf("github device flow: %v", err)
			b.send(chatID, "Failed to start GitHub authorization. Please try again.")
			return
		}
		deviceCode = dc.DeviceCode
		userCode = dc.UserCode
		verificationURI = dc.VerificationURI
		interval = dc.Interval
		if interval < 5 {
			interval = 5
		}
		pollFn = provider.PollGitHubToken
	case "gitlab":
		clientID = config.GitLabClientID
		clientSecret = config.GitLabClientSecret
		if clientID == "" {
			b.send(chatID, "GitLab OAuth App not configured. Rebuild with GitLab credentials.")
			return
		}
		dc, err := provider.StartGitLabDeviceFlow(clientID)
		if err != nil {
			log.Printf("gitlab device flow: %v", err)
			b.send(chatID, "Failed to start GitLab authorization. Please try again.")
			return
		}
		deviceCode = dc.DeviceCode
		userCode = dc.UserCode
		verificationURI = dc.VerificationURI
		interval = dc.Interval
		if interval < 5 {
			interval = 5
		}
		pollFn = provider.PollGitLabToken
	default:
		b.send(chatID, "Unknown provider.")
		return
	}

	b.send(chatID, fmt.Sprintf(
		"Open %s and enter the code:\n\n`%s`\n\nI'll notify you once you've authorized.",
		verificationURI, userCode,
	))

	go b.pollDeviceAuth(chatID, p, clientID, clientSecret, deviceCode, interval, pollFn)
}

// maxNetworkErrors is how many consecutive network failures are tolerated during
// OAuth polling before aborting. Transient outages are retried silently.
const maxNetworkErrors = 5

func (b *Bot) pollDeviceAuth(chatID int64, p, clientID, clientSecret, deviceCode string, intervalSec int, pollFn func(string, string, string) (string, error, int)) {
	log.Printf("[pollDeviceAuth] Starting poll for chatID=%d, provider=%s, interval=%ds", chatID, p, intervalSec)
	interval := intervalSec
	deadline := time.NewTimer(15 * time.Minute)
	defer deadline.Stop()

	pollCount := 0
	networkErrCount := 0
	for {
		select {
		case <-deadline.C:
			log.Printf("[pollDeviceAuth] Authorization timed out for chatID=%d", chatID)
			b.send(chatID, "Authorization timed out. Use /connect to try again.")
			return
		default:
			pollCount++
			log.Printf("[pollDeviceAuth] Poll attempt #%d for chatID=%d (interval=%ds)", pollCount, chatID, interval)
			token, err, newInterval := pollFn(clientID, clientSecret, deviceCode)

			if newInterval > 0 && newInterval != interval {
				log.Printf("[pollDeviceAuth] Updating interval from %ds to %ds", interval, newInterval)
				interval = newInterval
			}

			if err == nil {
				log.Printf("[pollDeviceAuth] Authorization successful for chatID=%d, storing token", chatID)
				data := map[string]interface{}{"provider": p, "token": token}
				if err := storage.SetConversationState(b.JobsDB, chatID, "telegram", "await_repo", data); err != nil {
					log.Printf("[pollDeviceAuth] Failed to store state for chatID=%d: %v", chatID, err)
					b.send(chatID, "Internal error storing authorization. Use /connect to try again.")
					return
				}
				b.send(chatID, fmt.Sprintf("✅ %s authorized! Now send the repository URL (e.g. https://%s.com/owner/repo):", strings.Title(p), p))
				return
			}

			switch err {
			case provider.ErrAuthPending:
				networkErrCount = 0
				log.Printf("[pollDeviceAuth] Poll #%d: still pending", pollCount)
			case provider.ErrAuthExpired:
				log.Printf("[pollDeviceAuth] Poll #%d: expired for chatID=%d", pollCount, chatID)
				b.send(chatID, "Authorization code expired. Use /connect to try again.")
				return
			default:
				// Treat as transient network error — retry up to maxNetworkErrors times.
				networkErrCount++
				log.Printf("[pollDeviceAuth] Poll #%d network error (%d/%d) for chatID=%d: %v",
					pollCount, networkErrCount, maxNetworkErrors, chatID, err)
				if networkErrCount >= maxNetworkErrors {
					b.send(chatID, fmt.Sprintf("Authorization failed after %d network errors: %v\n\nUse /connect to try again.", maxNetworkErrors, err))
					return
				}
			}

			log.Printf("[pollDeviceAuth] Waiting %ds before next poll", interval)
			time.Sleep(time.Duration(interval) * time.Second)
		}
	}
}

func isIssueURL(text string) bool {
	return githubIssueRegex.MatchString(text) || gitlabIssueRegex.MatchString(text)
}

func detectProvider(url string) string {
	if githubIssueRegex.MatchString(url) {
		return "github"
	}
	if gitlabIssueRegex.MatchString(url) {
		return "gitlab"
	}
	return ""
}

func extractRepoURL(issueURL string) string {
	for _, sep := range []string{"/-/issues/", "/-/work_items/", "/issues/"} {
		if idx := strings.Index(issueURL, sep); idx != -1 {
			return issueURL[:idx]
		}
	}
	return issueURL
}
