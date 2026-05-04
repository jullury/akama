package bot

import (
	"fmt"
	"log"
	"regexp"
	"strings"

	"github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/jullury/akama/internal/agent"
	"github.com/jullury/akama/internal/job"
	"github.com/jullury/akama/internal/provider"
	"github.com/jullury/akama/internal/storage"
)

var (
	githubIssueRegex = regexp.MustCompile(`github\.com/[^/]+/[^/]+/issues/\d+`)
	gitlabIssueRegex = regexp.MustCompile(`gitlab\.com/[^/]+/[^/]+/issues/\d+`)
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
	case strings.HasPrefix(text, "/connect"):
		b.handleConnect(chatID)
	case strings.HasPrefix(text, "/connections"):
		b.handleConnections(chatID)
	case strings.HasPrefix(text, "/disconnect"):
		b.handleDisconnect(chatID)
	case strings.HasPrefix(text, "/issues"):
		b.handleIssues(chatID)
	case strings.HasPrefix(text, "/status"):
		b.handleStatus(chatID)
	case strings.HasPrefix(text, "/done"):
		b.handleDone(chatID, text)
	case strings.HasPrefix(text, "/cancel"):
		storage.ResetConversation(b.JobsDB, chatID, "telegram")
		b.send(chatID, "Conversation reset.")
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
			AnthropicAPIKey: b.Config.AnthropicAPIKey,
			OpenAIAPIKey:    b.Config.OpenAIAPIKey,
		}
		go job.RunFollowUp(j.ID, msg.Text, b.JobsDB, b.API, agentCfg)
		b.send(chatID, fmt.Sprintf("[%s] Updating...", j.Provider))
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
	case "connect:github", "connect:gitlab":
		p := strings.TrimPrefix(data, "connect:")
		if err := storage.SetConversationState(b.JobsDB, chatID, "telegram", "await_repo", map[string]interface{}{"provider": p}); err != nil {
			log.Printf("callback: set state: %v", err)
			b.send(chatID, "Internal error, please try again.")
			return
		}
		b.send(chatID, fmt.Sprintf("Send the %s repository URL (e.g. https://%s.com/owner/repo):", strings.Title(p), p))
	default:
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
	case "idle":
		if isIssueURL(text) {
			b.processIssue(chatID, text, "")
		} else {
			b.send(chatID, "Send an issue URL or use /connect to add a repository.")
		}
	case "await_repo":
		providerName, _ := conv.Data["provider"].(string)
		if providerName == "" {
			storage.ResetConversation(b.JobsDB, chatID, "telegram")
			b.send(chatID, "Something went wrong. Use /connect to start over.")
			return
		}
		nextData := map[string]interface{}{"provider": providerName, "repo_url": text}
		storage.SetConversationState(b.JobsDB, chatID, "telegram", "await_token", nextData)
		b.send(chatID, fmt.Sprintf("Send your %s personal access token (PAT):", strings.Title(providerName)))
	case "await_token":
		providerName, _ := conv.Data["provider"].(string)
		repoURL, _ := conv.Data["repo_url"].(string)
		if providerName == "" || repoURL == "" {
			storage.ResetConversation(b.JobsDB, chatID, "telegram")
			b.send(chatID, "Something went wrong. Use /connect to start over.")
			return
		}
		if err := storage.SaveConnection(b.JobsDB, chatID, providerName, repoURL, text); err != nil {
			log.Printf("save connection: %v", err)
			b.send(chatID, "Failed to save connection. Please try again.")
			return
		}
		storage.ResetConversation(b.JobsDB, chatID, "telegram")
		b.send(chatID, fmt.Sprintf("Connected to %s/%s! Send an issue URL to start a job.", providerName, repoURL))
	}
}

func (b *Bot) processIssue(chatID int64, issueURL, gitToken string) {
	providerName := detectProvider(issueURL)
	if providerName == "" {
		b.send(chatID, "Unsupported provider. Use GitHub or GitLab issue URL.")
		return
	}

	var token string
	if gitToken == "" {
		conn, _ := storage.FindConnectionByRepo(b.JobsDB, chatID, issueURL)
		if conn != nil {
			token = conn.GitToken
		}
	} else {
		token = gitToken
	}

	if token == "" {
		b.send(chatID, "No git token found. Use /connect to add a repository first.")
		return
	}

	var title, body, issueID string
	var err error

	switch providerName {
	case "github":
		issue, err := provider.FetchGitHubIssue(issueURL, token)
		if err == nil {
			title = issue.Title
			body = issue.Body
			issueID = fmt.Sprintf("%d", issue.Number)
		}
	case "gitlab":
		issue, err := provider.FetchGitLabIssue(issueURL, token)
		if err == nil {
			title = issue.Title
			body = issue.Description
			issueID = fmt.Sprintf("%d", issue.IID)
		}
	}

	if err != nil {
		b.send(chatID, fmt.Sprintf("Failed to fetch issue: %v", err))
		return
	}

	j := &storage.Job{
		ChatID:     chatID,
		IssueID:    issueID,
		IssueTitle:  title,
		IssueBody:   body,
		IssueURL:    issueURL,
		RepoURL:     extractRepoURL(issueURL),
		Provider:    providerName,
		GitToken:    token,
		Agent:       b.Config.DefaultAgent,
		AgentModel:  b.Config.DefaultModel,
	}

	jobID, err := storage.CreateJob(b.JobsDB, j)
	if err != nil {
		b.send(chatID, fmt.Sprintf("Failed to create job: %v", err))
		return
	}

	agentCfg := &agent.Config{
		AnthropicAPIKey: b.Config.AnthropicAPIKey,
		OpenAIAPIKey:    b.Config.OpenAIAPIKey,
	}
	job.Run(jobID, b.JobsDB, b.API, agentCfg, b.Config.WorkspaceDir)
}

func (b *Bot) send(chatID int64, text string) {
	if _, err := b.API.Send(tgbotapi.NewMessage(chatID, text)); err != nil {
		log.Printf("send to %d: %v", chatID, err)
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
	if idx := strings.Index(issueURL, "/issues/"); idx != -1 {
		return issueURL[:idx]
	}
	return issueURL
}
