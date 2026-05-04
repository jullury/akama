package bot

import (
	"fmt"
	"log"
	"regexp"
	"strings"

	"github.com/go-telegram-bot-api/telegram-bot-api"

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
	chatID := query.Message.Chat.ID
	data := query.Data

	switch {
	case data == "connect:github":
		storage.SetConversationState(b.JobsDB, chatID, "telegram", "await_repo", map[string]interface{}{"provider": "github"})
		b.send(chatID, "Send the GitHub repository URL.")
	case data == "connect:gitlab":
		storage.SetConversationState(b.JobsDB, chatID, "telegram", "await_repo", map[string]interface{}{"provider": "gitlab"})
		b.send(chatID, "Send the GitLab repository URL.")
	}

	b.API.AnswerCallbackQuery(tgbotapi.NewCallback(query.ID, ""))
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
		providerName := conv.Data["provider"].(string)
		storage.SaveConnection(b.JobsDB, chatID, providerName, text, "")
		storage.SetConversationState(b.JobsDB, chatID, "telegram", "await_token", conv.Data)
		b.send(chatID, fmt.Sprintf("Send your %s personal access token.", strings.Title(providerName)))
	case "await_token":
		providerName := conv.Data["provider"].(string)
		repoURL := conv.Data["repo_url"].(string)
		storage.SaveConnection(b.JobsDB, chatID, providerName, repoURL, text)
		storage.ResetConversation(b.JobsDB, chatID, "telegram")
		b.send(chatID, "Connected! Send an issue URL.")
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
	msg := tgbotapi.NewMessage(chatID, text)
	b.API.Send(msg)
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
