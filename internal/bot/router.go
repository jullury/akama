package bot

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/jullury/akama/internal/agent"
	"github.com/jullury/akama/internal/config"
	"github.com/jullury/akama/internal/git"
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
	case strings.HasPrefix(text, "/quickfix"):
		b.handleQuickFix(chatID)
	case strings.HasPrefix(text, "/connections"):
		b.handleConnections(chatID)
	case strings.HasPrefix(text, "/delete_connection"):
		b.handleDeleteConnection(chatID)
	case strings.HasPrefix(text, "/connect"):
		b.handleConnect(chatID)
	case strings.HasPrefix(text, "/disconnect"):
		b.handleDisconnect(chatID)
	case strings.HasPrefix(text, "/issues"):
		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Open", "issues:open"),
				tgbotapi.NewInlineKeyboardButtonData("Running", "issues:running"),
				tgbotapi.NewInlineKeyboardButtonData("Failed", "issues:failed"),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Pending", "issues:pending"),
				tgbotapi.NewInlineKeyboardButtonData("All", "issues:all"),
			),
		)
		msg := tgbotapi.NewMessage(chatID, "Show jobs:")
		msg.ReplyMarkup = keyboard
		b.API.Send(msg)
	case strings.HasPrefix(text, "/metrics"):
		b.handleMetrics(chatID)
	case strings.HasPrefix(text, "/queue"):
		b.handleQueue(chatID)
	case strings.HasPrefix(text, "/status"):
		b.handleStatus(chatID, 0)
	case strings.HasPrefix(text, "/logs"):
		storage.SetConversationState(b.JobsDB, chatID, "telegram", "await_logs", nil)
		b.send(chatID, "Enter the job ID to view logs:")
	case strings.HasPrefix(text, "/preview"):
		parts := strings.Fields(text)
		if len(parts) > 1 {
			var jobID int64
			fmt.Sscanf(parts[1], "%d", &jobID)
			if jobID > 0 {
				b.handlePreview(chatID, jobID)
				return
			}
		}
		storage.SetConversationState(b.JobsDB, chatID, "telegram", "await_preview", nil)
		b.send(chatID, "Enter the job ID to preview:")
	case strings.HasPrefix(text, "/retry_all"):
		b.handleRetryAll(chatID)
	case strings.HasPrefix(text, "/retry"):
		storage.SetConversationState(b.JobsDB, chatID, "telegram", "await_retry", nil)
		b.send(chatID, "Enter the job ID to retry:")
	case strings.HasPrefix(text, "/restart"):
		if !storage.IsAdmin(b.JobsDB, chatID) {
			b.send(chatID, "Only the admin can restart the server.")
			return
		}
		b.handleRestartCommand(chatID)
	case strings.HasPrefix(text, "/done"):
		// Check if user is in issue image collection mode first
		conv, err := storage.GetConversation(b.JobsDB, chatID, "telegram")
		if err == nil && conv.State == "await_issue_images" {
			title, _ := conv.Data["title"].(string)
			body, _ := conv.Data["body"].(string)
			images, _ := conv.Data["images"].(string)

			reposInterface, hasRepos := conv.Data["repos"].([]interface{})
			if hasRepos && len(reposInterface) > 0 {
				// Multi-repo image skipper
				repos := make([]map[string]interface{}, 0, len(reposInterface))
				for _, ri := range reposInterface {
					if m, ok := ri.(map[string]interface{}); ok {
						repos = append(repos, m)
					}
				}
				storage.ResetConversation(b.JobsDB, chatID, "telegram")

				firstRepo := repos[0]
				firstRepoURL := firstRepo["repo_url"].(string)
				firstProvider := firstRepo["provider"].(string)
				firstToken := firstRepo["token"].(string)

				fullBody := embedImages(body, images, firstProvider, firstToken, firstRepoURL)
				var issueURL string
				var issueErr error
				switch firstProvider {
				case "github":
					issueURL, issueErr = provider.CreateGitHubIssue(firstRepoURL, firstToken, title, fullBody)
				case "gitlab":
					issueURL, issueErr = provider.CreateGitLabIssue(firstRepoURL, firstToken, title, fullBody)
				}
				if issueErr != nil {
					if provider.IsAuthError(issueErr) {
						b.send(chatID, fmt.Sprintf(
							"❌ Authentication failed for %s. Your token may have expired.\n"+
								"Use /connect to refresh your token, then /newissue to try again.",
							firstProvider,
						))
						return
					}
					b.send(chatID, fmt.Sprintf("❌ Failed to create issue: %v", issueErr))
					return
				}
				b.send(chatID, fmt.Sprintf("✅ Issue created: %s\n\nProcessing it across %d repositories...", issueURL, len(repos)))
				b.processMultiIssue(chatID, issueURL, repos, images)
				return
			}

			// Single-repo image skipper
			repoURL, _ := conv.Data["repo_url"].(string)
			providerName, _ := conv.Data["provider"].(string)
			token, _ := conv.Data["token"].(string)

			storage.ResetConversation(b.JobsDB, chatID, "telegram")

			var issueURL string
			var issueErr error
			fullBody := embedImages(body, images, providerName, token, repoURL)
			switch providerName {
			case "github":
				issueURL, issueErr = provider.CreateGitHubIssue(repoURL, token, title, fullBody)
			case "gitlab":
				issueURL, issueErr = provider.CreateGitLabIssue(repoURL, token, title, fullBody)
			}
			if issueErr != nil {
				if provider.IsAuthError(issueErr) {
					pendingData := map[string]interface{}{
						"pending_action": "create_issue",
						"title":          title,
						"body":           body,
						"repo_url":       repoURL,
						"provider_name":  providerName,
						"token":          token,
						"images":         images,
					}
					if saveErr := storage.SetConversationState(b.JobsDB, chatID, "telegram", "await_token_refresh", pendingData); saveErr != nil {
						log.Printf("[await_issue_images] Failed to save pending action: %v", saveErr)
					}
					b.send(chatID, fmt.Sprintf(
						"❌ Authentication failed for %s. Your token may have expired or been revoked.\n\n"+
							"Use /connect to refresh your token, then /newissue to try again.",
						providerName,
					))
					return
				}
				b.send(chatID, fmt.Sprintf("❌ Failed to create issue: %v", issueErr))
				return
			}
			b.send(chatID, fmt.Sprintf("✅ Issue created: %s", issueURL))
			return
		}
		// Normal /done command: mark job as done
		storage.SetConversationState(b.JobsDB, chatID, "telegram", "await_done", nil)
		b.send(chatID, "Enter the job ID to mark as done, or type 'all' to clean up all completed jobs:")
	case strings.HasPrefix(text, "/followup"):
		storage.SetConversationState(b.JobsDB, chatID, "telegram", "await_followup_id", nil)
		b.send(chatID, "Enter the job ID for follow-up:")
	case strings.HasPrefix(text, "/skills"):
		b.handleSkills(chatID)
	case strings.HasPrefix(text, "/cancel"):
		storage.ResetConversation(b.JobsDB, chatID, "telegram")
		b.send(chatID, "Conversation reset.")
	case strings.HasPrefix(text, "/users"):
		b.handleUsers(chatID)
	case strings.HasPrefix(text, "/add_user"):
		if !storage.IsAdmin(b.JobsDB, chatID) {
			b.send(chatID, "Only the admin can add users.")
			return
		}
		storage.SetConversationState(b.JobsDB, chatID, "telegram", "await_add_user", nil)
		b.send(chatID, "Send the Telegram user ID to add:")
	case strings.HasPrefix(text, "/delete_user"):
		if !storage.IsAdmin(b.JobsDB, chatID) {
			b.send(chatID, "Only the admin can delete users.")
			return
		}
		storage.SetConversationState(b.JobsDB, chatID, "telegram", "await_delete_user", nil)
		b.send(chatID, "Send the Telegram user ID to delete:")
	case strings.HasPrefix(text, "/update_agents"):
		if !storage.IsAdmin(b.JobsDB, chatID) {
			b.send(chatID, "Only the admin can update agents.")
			return
		}
		go b.handleUpdateAgents(chatID)
	case strings.HasPrefix(text, "/version"):
		b.handleVersionCommand(chatID)
	case strings.HasPrefix(text, "/update"):
		if !storage.IsAdmin(b.JobsDB, chatID) {
			b.send(chatID, "Only the admin can update the server.")
			return
		}
		go b.handleUpdateCommand(chatID)
	default:
		if msg.Photo != nil {
			b.handlePhoto(chatID, msg)
		} else {
			b.handleText(chatID, text)
		}
	}
}

func (b *Bot) handlePhoto(chatID int64, msg *tgbotapi.Message) {
	conv, err := storage.GetConversation(b.JobsDB, chatID, "telegram")
	if err != nil {
		log.Printf("get conversation: %v", err)
		return
	}

	if conv.State == "await_issue_desc" {
		b.send(chatID, "Please describe the issue first before sending images. Send the issue description (first line = title, rest = description).")
		return
	}

	if conv.State != "idle" && conv.State != "await_issue_images" {
		b.send(chatID, "Images can only be sent when creating a new issue. Use /newissue to start.")
		return
	}

	if conv.State == "idle" {
		b.send(chatID, "Use /newissue to create a new issue first, then you can attach images.")
		return
	}

	// Get the highest resolution photo
	photos := msg.Photo
	photo := photos[len(photos)-1]

	fileConfig := tgbotapi.FileConfig{FileID: photo.FileID}
	file, err := b.API.GetFile(fileConfig)
	if err != nil {
		log.Printf("[handlePhoto] Failed to get file: %v", err)
		b.send(chatID, "Failed to download image. Please try again.")
		return
	}

	fileURL := file.Link(b.API.Token)

	resp, err := http.Get(fileURL)
	if err != nil {
		log.Printf("[handlePhoto] Failed to download: %v", err)
		b.send(chatID, "Failed to download image. Please try again.")
		return
	}
	defer resp.Body.Close()

	imageData, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[handlePhoto] Failed to read: %v", err)
		b.send(chatID, "Failed to read image. Please try again.")
		return
	}

	// Store image info in conversation data: URL|size in bytes
	imagesInterface, _ := conv.Data["images"]
	images := ""
	if imagesInterface != nil {
		images, _ = imagesInterface.(string)
	}

	imageEntry := fmt.Sprintf("%s|%d", fileURL, len(imageData))
	if images != "" {
		images += ";"
	}
	images += imageEntry

	conv.Data["images"] = images
	if err := storage.SetConversationState(b.JobsDB, chatID, "telegram", "await_issue_images", conv.Data); err != nil {
		log.Printf("[handlePhoto] Failed to save: %v", err)
		b.send(chatID, "Failed to save image. Please try again.")
		return
	}

	b.send(chatID, fmt.Sprintf("✅ Image received (%d bytes). Send more images or /done to finish.", len(imageData)))
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
			APIKeys:     b.Config.APIKeys,
			TimeoutMins: b.Config.AgentTimeoutMins,
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
		keyboard, title := buildModelKeyboard("claude", 0, b.Config.APIKeys)
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
		keyboard, title := buildModelKeyboard("opencode", 0, b.Config.APIKeys)
		msg := tgbotapi.NewMessage(chatID, title)
		msg.ReplyMarkup = keyboard
		b.API.Send(msg)
	default:
		if connIDStr, ok := strings.CutPrefix(data, "newissue:toggle:"); ok {
			var connID int64
			fmt.Sscanf(connIDStr, "%d", &connID)
			conv, err := storage.GetConversation(b.JobsDB, chatID, "telegram")
			if err != nil || conv.State != "await_repo_select" {
				return
			}
			connsInterface, _ := conv.Data["connections"].([]interface{})
			selInterface, _ := conv.Data["selected_ids"].([]interface{})

			selectedIDs := make([]int64, 0, len(selInterface))
			selSet := make(map[int64]bool)
			for _, s := range selInterface {
				if id, ok := s.(float64); ok {
					selectedIDs = append(selectedIDs, int64(id))
					selSet[int64(id)] = true
				}
			}

			if selSet[connID] {
				// Remove from selection
				newSel := make([]int64, 0, len(selectedIDs)-1)
				for _, id := range selectedIDs {
					if id != connID {
						newSel = append(newSel, id)
					}
				}
				selectedIDs = newSel
			} else {
				selectedIDs = append(selectedIDs, connID)
			}

			selFloats := make([]interface{}, len(selectedIDs))
			for i, id := range selectedIDs {
				selFloats[i] = float64(id)
			}
			conv.Data["selected_ids"] = selFloats
			storage.SetConversationState(b.JobsDB, chatID, "telegram", "await_repo_select", conv.Data)

			// Rebuild connections list
			var conns []*storage.Connection
			for _, ci := range connsInterface {
				if m, ok := ci.(map[string]interface{}); ok {
					c := &storage.Connection{
						ID:            int64(m["id"].(float64)),
						RepoURL:       m["repo_url"].(string),
						Provider:      m["provider"].(string),
						GitToken:      m["token"].(string),
						DefaultBranch: m["default_branch"].(string),
					}
					conns = append(conns, c)
				}
			}

			// Edit the original message with updated keyboard
			editMsg := tgbotapi.NewEditMessageText(chatID, query.Message.MessageID,
				"Select repositories for the new issue (tap to toggle, then press Done):")
			kb := b.buildMultiRepoKeyboard(conns, selectedIDs)
			editMsg.ReplyMarkup = &kb
			b.API.Send(editMsg)
			return
		}
		if data == "newissue:confirm" {
			conv, err := storage.GetConversation(b.JobsDB, chatID, "telegram")
			if err != nil || conv.State != "await_repo_select" {
				b.send(chatID, "No active repository selection. Use /newissue to start over.")
				return
			}
			selInterface, _ := conv.Data["selected_ids"].([]interface{})
			if len(selInterface) == 0 {
				b.send(chatID, "Please select at least one repository.")
				return
			}
			connsInterface, _ := conv.Data["connections"].([]interface{})

			selectedIDs := make([]int64, 0, len(selInterface))
			for _, s := range selInterface {
				if id, ok := s.(float64); ok {
					selectedIDs = append(selectedIDs, int64(id))
				}
			}

			// Build repos array for conversation data
			repos := make([]map[string]interface{}, 0)
			for _, ci := range connsInterface {
				m, ok := ci.(map[string]interface{})
				if !ok {
					continue
				}
				id := int64(m["id"].(float64))
				for _, selID := range selectedIDs {
					if id == selID {
						branch, _ := m["default_branch"].(string)
						if branch == "" {
							branch = "main"
						}
						repos = append(repos, map[string]interface{}{
							"repo_url":       m["repo_url"],
							"provider":       m["provider"],
							"token":          m["token"],
							"default_branch": branch,
						})
						break
					}
				}
			}

			storage.SetConversationState(b.JobsDB, chatID, "telegram", "await_branch_select",
				map[string]interface{}{
					"repos":            repos,
					"current_repo_idx": float64(0),
				})

			firstRepo := repos[0]
			b.send(chatID, fmt.Sprintf(
				"Select branch for [%s] %s:\nDetected default branch: *%s*\n\n"+
					"Send a branch name or press Enter to use the detected branch.",
				firstRepo["provider"], firstRepo["repo_url"], firstRepo["default_branch"],
			))
			return
		}
		if rest, ok := strings.CutPrefix(data, "config:model:page:"); ok {
			// format: config:model:page:<agentName>:<page>
			parts := strings.SplitN(rest, ":", 2)
			if len(parts) == 2 {
				agentName := parts[0]
				var page int
				fmt.Sscanf(parts[1], "%d", &page)
				keyboard, title := buildModelKeyboard(agentName, page, b.Config.APIKeys)
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
			b.handleStatus(chatID, page)
			return
		}
		if rest, ok := strings.CutPrefix(data, "issues:"); ok {
			// Parse "status:page:N" or just "status"
			page := 0
			filterStatus := rest
			if idx := strings.LastIndex(rest, ":page:"); idx >= 0 {
				filterStatus = rest[:idx]
				fmt.Sscanf(rest[idx+len(":page:"):], "%d", &page)
			}
			b.showIssues(chatID, filterStatus, page)
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
		if connIDStr, ok := strings.CutPrefix(data, "connection_agent:"); ok {
			var connID int64
			fmt.Sscanf(connIDStr, "%d", &connID)
			row1 := tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Claude", fmt.Sprintf("set_conn_agent:%d:claude", connID)),
				tgbotapi.NewInlineKeyboardButtonData("OpenCode", fmt.Sprintf("set_conn_agent:%d:opencode", connID)),
				tgbotapi.NewInlineKeyboardButtonData("Use default", fmt.Sprintf("set_conn_agent:%d:", connID)),
			)
			kb := tgbotapi.NewInlineKeyboardMarkup(row1)
			msg := tgbotapi.NewMessage(chatID, "Select agent for this repository:")
			msg.ReplyMarkup = kb
			b.API.Send(msg)
			return
		}
		if rest, ok := strings.CutPrefix(data, "set_conn_agent:"); ok {
			parts := strings.SplitN(rest, ":", 2)
			if len(parts) != 2 {
				return
			}
			var connID int64
			fmt.Sscanf(parts[0], "%d", &connID)
			agentName := parts[1]
			if err := storage.SetConnectionAgent(b.JobsDB, connID, agentName, ""); err != nil {
				log.Printf("set connection agent: %v", err)
				b.send(chatID, "Failed to save agent setting.")
				return
			}
			if agentName == "" {
				b.send(chatID, "Connection will use your default agent.")
			} else {
				b.send(chatID, fmt.Sprintf("Connection agent set to: %s", agentName))
			}
			return
		}
		if rest, ok := strings.CutPrefix(data, "skills:install:"); ok {
			var idx int
			fmt.Sscanf(rest, "%d", &idx)
			s := agent.SkillByIndex(idx)
			if s == nil {
				b.send(chatID, "Unknown skill.")
				return
			}
			b.send(chatID, fmt.Sprintf("Installing %s...", s.Name))
			go b.installSkill(chatID, *s)
			return
		}
		if data == "skills:custom" {
			storage.SetConversationState(b.JobsDB, chatID, "telegram", "await_skill_id", nil)
			b.send(chatID, "Send the skillhub.club skill ID to install:\n\nExample: `massgen-massgen-file-search`")
			return
		}
		if data == "update:confirm" {
			if !storage.IsAdmin(b.JobsDB, chatID) {
				b.send(chatID, "Only the admin can confirm updates.")
				return
			}
			go b.handleUpdateConfirm(chatID)
			return
		}
		if data == "update:cancel" {
			b.send(chatID, "Update cancelled.")
			return
		}
		if data == "restart:confirm" {
			if !storage.IsAdmin(b.JobsDB, chatID) {
				b.send(chatID, "Only the admin can confirm restart.")
				return
			}
			go b.handleRestartConfirm(chatID)
			return
		}
		if data == "restart:cancel" {
			b.send(chatID, "Restart cancelled.")
			return
		}
		if data == "plan:confirm" {
			conv, err := storage.GetConversation(b.JobsDB, chatID, "telegram")
			if err != nil {
				log.Printf("plan confirm: get conversation: %v", err)
				b.send(chatID, "Failed to load conversation state.")
				return
			}
			if conv.State == "await_plan_regen" {
				b.send(chatID, "⏳ Plan is still being regenerated — please wait.")
				return
			}
			if conv.State != "await_plan_review" {
				b.send(chatID, "No plan to confirm. Use /cancel to reset.")
				return
			}
			if multiRepo, _ := conv.Data["multi_repo"].(bool); multiRepo {
				b.proceedWithMultiPlan(chatID, conv)
			} else {
				b.proceedWithPlan(chatID, conv)
			}
			return
		}
		if data == "plan:cancel" {
			conv, convErr := storage.GetConversation(b.JobsDB, chatID, "telegram")
			if convErr == nil {
				if ws, _ := conv.Data["plan_workspace"].(string); ws != "" {
					os.RemoveAll(ws)
				}
			}
			storage.ResetConversation(b.JobsDB, chatID, "telegram")
			b.send(chatID, "Plan cancelled. You can send another issue URL to start over.")
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

		reposInterface, hasRepos := conv.Data["repos"].([]interface{})
		if hasRepos && len(reposInterface) > 0 {
			// Multi-repo flow
			repos := make([]map[string]interface{}, 0, len(reposInterface))
			for _, ri := range reposInterface {
				if m, ok := ri.(map[string]interface{}); ok {
					repos = append(repos, m)
				}
			}
			pendingData := map[string]interface{}{
				"repos":  repos,
				"title":  title,
				"body":   body,
				"images": "",
			}
			storage.SetConversationState(b.JobsDB, chatID, "telegram", "await_issue_images", pendingData)
			b.send(chatID, "Now you can send images to attach to the issue. Send images or /done to create the issue.")
			return
		}

		// Single-repo flow (backward compat)
		repoURL, _ := conv.Data["repo_url"].(string)
		providerName, _ := conv.Data["provider"].(string)
		token, _ := conv.Data["token"].(string)
		storage.ResetConversation(b.JobsDB, chatID, "telegram")

		pendingData := map[string]interface{}{
			"repo_url":      repoURL,
			"provider":      providerName,
			"token":         token,
			"title":         title,
			"body":          body,
			"images":        "",
		}
		storage.SetConversationState(b.JobsDB, chatID, "telegram", "await_issue_images", pendingData)
		b.send(chatID, "Now you can send images to attach to the issue. Send images or /done to create the issue.")
		return
	case "await_issue_images":
		title, _ := conv.Data["title"].(string)
		body, _ := conv.Data["body"].(string)
		images, _ := conv.Data["images"].(string)

		reposInterface, hasRepos := conv.Data["repos"].([]interface{})
		if hasRepos && len(reposInterface) > 0 {
			// Multi-repo flow
			repos := make([]map[string]interface{}, 0, len(reposInterface))
			for _, ri := range reposInterface {
				if m, ok := ri.(map[string]interface{}); ok {
					repos = append(repos, m)
				}
			}
			storage.ResetConversation(b.JobsDB, chatID, "telegram")

			// Create the issue on the first repo only
			firstRepo := repos[0]
			firstRepoURL := firstRepo["repo_url"].(string)
			firstProvider := firstRepo["provider"].(string)
			firstToken := firstRepo["token"].(string)

			fullBody := embedImages(body, images, firstProvider, firstToken, firstRepoURL)
			var issueURL string
			var err error
			switch firstProvider {
			case "github":
				issueURL, err = provider.CreateGitHubIssue(firstRepoURL, firstToken, title, fullBody)
			case "gitlab":
				issueURL, err = provider.CreateGitLabIssue(firstRepoURL, firstToken, title, fullBody)
			}
			if err != nil {
				if provider.IsAuthError(err) {
					b.send(chatID, fmt.Sprintf(
						"❌ Authentication failed for %s. Your token may have expired.\n"+
							"Use /connect to refresh your token, then /newissue to try again.",
						firstProvider,
					))
					return
				}
				b.send(chatID, fmt.Sprintf("Failed to create issue: %v", err))
				return
			}
			b.send(chatID, fmt.Sprintf("Issue created: %s\n\nProcessing it across %d repositories...", issueURL, len(repos)))
			b.processMultiIssue(chatID, issueURL, repos, images)
			return
		}

		// Single-repo flow
		repoURL, _ := conv.Data["repo_url"].(string)
		providerName, _ := conv.Data["provider"].(string)
		token, _ := conv.Data["token"].(string)

		storage.ResetConversation(b.JobsDB, chatID, "telegram")

		fullBody := embedImages(body, images, providerName, token, repoURL)
		var issueURL string
		var err error
		switch providerName {
		case "github":
			issueURL, err = provider.CreateGitHubIssue(repoURL, token, title, fullBody)
		case "gitlab":
			issueURL, err = provider.CreateGitLabIssue(repoURL, token, title, fullBody)
		}
		if err != nil {
			if provider.IsAuthError(err) {
				pendingData := map[string]interface{}{
					"pending_action": "create_issue",
					"title":          title,
					"body":           body,
					"repo_url":       repoURL,
					"provider_name":  providerName,
					"token":          token,
					"images":         images,
				}
				if saveErr := storage.SetConversationState(b.JobsDB, chatID, "telegram", "await_token_refresh", pendingData); saveErr != nil {
					log.Printf("[await_issue_images] Failed to save pending action: %v", saveErr)
				}
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
		b.processIssueWithImages(chatID, issueURL, token, images)
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
			APIKeys:     b.Config.APIKeys,
			TimeoutMins: b.Config.AgentTimeoutMins,
		}
		go job.RunFollowUp(b.ctx, jobID, text, b.JobsDB, b.API, agentCfg)
		b.send(chatID, "Got it, continuing work on the issue...")
	case "await_clarifying_questions":
		answers := strings.TrimSpace(text)
		if answers == "" {
			b.send(chatID, "Please answer the questions to help me create a plan.")
			return
		}

		conv.Data["answers"] = answers
		title, _ := conv.Data["title"].(string)
		body, _ := conv.Data["body"].(string)
		agentName, _ := conv.Data["agent_name"].(string)
		agentModel, _ := conv.Data["agent_model"].(string)

		if agentName == "" {
			agentName = b.Config.DefaultAgent
		}

		agentCfg := &agent.Config{
			APIKeys:          b.Config.APIKeys,
			TimeoutMins:      b.Config.AgentTimeoutMins,
			WorkspaceBaseDir: b.Config.WorkspaceDir,
		}

		b.send(chatID, "Generating implementation plan...")

		planWorkspace, _ := conv.Data["plan_workspace"].(string)
		repoSources, _ := conv.Data["repo_sources"].([]string)
		prompt := agent.BuildPlanFromAnswers(title, body, answers, repoSources...)
		planOutput, agentErr := agent.RunPlanAgent(b.ctx, agentName, agentModel, planWorkspace, prompt, agentCfg)
		if agentErr != nil {
			log.Printf("[await_clarifying_questions] Failed to generate plan: %v", agentErr)
			b.send(chatID, fmt.Sprintf("Failed to generate plan: %v. Please try again or use /cancel to abort.", agentErr))
			return
		}

		issueURL, _ := conv.Data["issue_url"].(string)
		providerName, _ := conv.Data["provider"].(string)

		multiRepo, _ := conv.Data["multi_repo"].(bool)
		var gitToken string
		if multiRepo {
			reposInterface, _ := conv.Data["repos"].([]interface{})
			if len(reposInterface) > 0 {
				if firstRepo, ok := reposInterface[0].(map[string]interface{}); ok {
					gitToken, _ = firstRepo["token"].(string)
				}
			}
		} else {
			gitToken, _ = conv.Data["git_token"].(string)
			repoURL, _ := conv.Data["repo_url"].(string)
			if conn, err := storage.FindConnectionByRepo(b.JobsDB, chatID, repoURL); err == nil && conn != nil {
				gitToken = conn.GitToken
			}
		}

		comment := fmt.Sprintf("## Implementation Plan\n\n%s", planOutput)
		if err := provider.PostIssueComment(providerName, issueURL, gitToken, comment); err != nil {
			log.Printf("[await_clarifying_questions] Failed to post plan comment: %v", err)
			b.send(chatID, "⚠️ Couldn't post plan as a GitHub comment (check token permissions), but it's shown below.")
		} else {
			b.send(chatID, "Plan posted as comment on the issue.")
		}

		conv.Data["plan"] = planOutput
		storage.SetConversationState(b.JobsDB, chatID, "telegram", "await_plan_review", conv.Data)

		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Confirm", "plan:confirm"),
				tgbotapi.NewInlineKeyboardButtonData("Cancel", "plan:cancel"),
			),
		)

		b.sendChunked(chatID, "Implementation plan ready:", planOutput)
		confirm := tgbotapi.NewMessage(chatID, "Do you want to proceed with this plan? Reply with changes or tap Confirm.")
		confirm.ReplyMarkup = keyboard
		b.API.Send(confirm)

	case "await_plan_review":
		mods := strings.TrimSpace(text)

		title, _ := conv.Data["title"].(string)
		body, _ := conv.Data["body"].(string)
		answers, _ := conv.Data["answers"].(string)
		agentName, _ := conv.Data["agent_name"].(string)
		agentModel, _ := conv.Data["agent_model"].(string)

		if agentName == "" {
			agentName = b.Config.DefaultAgent
		}

		agentCfg := &agent.Config{
			APIKeys:          b.Config.APIKeys,
			TimeoutMins:      b.Config.AgentTimeoutMins,
			WorkspaceBaseDir: b.Config.WorkspaceDir,
		}

		updatedAnswers := answers
		if mods != "" {
			updatedAnswers = fmt.Sprintf("%s\n\nUser requested changes:\n%s", answers, mods)
			conv.Data["answers"] = updatedAnswers
		}

		// Persist modifications before running the agent so a daemon restart
		// doesn't lose the user's changes.
		storage.SetConversationState(b.JobsDB, chatID, "telegram", "await_plan_regen", conv.Data)

		b.send(chatID, "⏳ Regenerating plan with your changes...")

		savedData := conv.Data
		go func() {
			heartbeatStop := make(chan struct{})
			go func() {
				ticker := time.NewTicker(30 * time.Second)
				defer ticker.Stop()
				for {
					select {
					case <-heartbeatStop:
						return
					case <-ticker.C:
						b.send(chatID, "⏳ Still thinking...")
					}
				}
			}()

			planWorkspace, _ := savedData["plan_workspace"].(string)
			repoSources, _ := savedData["repo_sources"].([]string)
			prompt := agent.BuildPlanFromAnswers(title, body, updatedAnswers, repoSources...)
			planOutput, agentErr := agent.RunPlanAgent(b.ctx, agentName, agentModel, planWorkspace, prompt, agentCfg)
			close(heartbeatStop)

			if agentErr != nil {
				log.Printf("[await_plan_review] Failed to regenerate plan: %v", agentErr)
				// Restore state so the user can try again
				storage.SetConversationState(b.JobsDB, chatID, "telegram", "await_plan_review", savedData)
				b.send(chatID, fmt.Sprintf("❌ Failed to regenerate plan: %v\n\nReply again to retry.", agentErr))
				return
			}

			savedData["plan"] = planOutput
			storage.SetConversationState(b.JobsDB, chatID, "telegram", "await_plan_review", savedData)

			keyboard := tgbotapi.NewInlineKeyboardMarkup(
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("Confirm", "plan:confirm"),
					tgbotapi.NewInlineKeyboardButtonData("Cancel", "plan:cancel"),
				),
			)
			b.sendChunked(chatID, "Updated plan:", planOutput)
			confirm := tgbotapi.NewMessage(chatID, "Reply with further changes or tap Confirm.")
			confirm.ReplyMarkup = keyboard
			b.API.Send(confirm)
		}()

	case "await_plan_regen":
		b.send(chatID, "⏳ Still regenerating the plan — please wait a moment.")

	case "await_followup":
		jobIDFloat, _ := conv.Data["job_id"].(float64)
		jobID := int64(jobIDFloat)
		j, _ := storage.GetJob(b.JobsDB, jobID)
		if j != nil && j.Status == "updating" {
			b.send(chatID, "A follow-up is already in progress. Please wait for it to complete.")
		} else {
			storage.ResetConversation(b.JobsDB, chatID, "telegram")
		}
	case "await_quickfix_url":
		storage.ResetConversation(b.JobsDB, chatID, "telegram")
		if !isIssueURL(text) {
			b.send(chatID, "That doesn't look like a valid GitHub or GitLab issue URL. Use /quickfix to try again.")
			return
		}
		issueURL := strings.TrimSpace(text)
		providerName := detectProvider(issueURL)
		lookupURL := extractRepoURL(issueURL)
		conn, _ := storage.FindConnectionByRepo(b.JobsDB, chatID, lookupURL)
		if conn == nil {
			b.send(chatID, "No connection found for this repository. Use /connect to add it first.")
			return
		}
		if existing := storage.FindActiveJobByIssue(b.JobsDB, chatID, issueURL); existing != nil {
			b.send(chatID, fmt.Sprintf("⚠️ Job #%d is already working on this issue (status: %s).", existing.ID, existing.Status))
			return
		}
		var title, body, issueID string
		var fetchErr error
		switch providerName {
		case "github":
			issue, e := provider.FetchGitHubIssue(issueURL, conn.GitToken)
			if e != nil {
				fetchErr = e
			} else {
				title = issue.Title
				body = issue.Body
				issueID = fmt.Sprintf("%d", issue.Number)
			}
		case "gitlab":
			issue, e := provider.FetchGitLabIssue(issueURL, conn.GitToken)
			if e != nil {
				fetchErr = e
			} else {
				title = issue.Title
				body = issue.Description
				issueID = fmt.Sprintf("%d", issue.IID)
			}
		}
		if fetchErr != nil {
			if provider.IsAuthError(fetchErr) {
				b.send(chatID, fmt.Sprintf("Authentication failed for %s. Use /connect to refresh your token.", providerName))
				return
			}
			b.send(chatID, fmt.Sprintf("Failed to fetch issue: %v", fetchErr))
			return
		}
		if issueID == "" {
			b.send(chatID, "Failed to parse issue ID from fetched issue.")
			return
		}
		if body != "" {
			body = provider.EnrichIssueBody(providerName, issueURL, conn.GitToken, body)
		}
		defaultBranch := conn.DefaultBranch
		if defaultBranch == "" {
			defaultBranch = "main"
		}
		userCfg, _ := storage.GetUserConfig(b.JobsDB, chatID)
		agentName := b.Config.DefaultAgent
		agentModel := b.Config.DefaultModel
		if userCfg != nil {
			if userCfg.Agent != "" {
				agentName = userCfg.Agent
			}
			if userCfg.AgentModel != "" {
				agentModel = userCfg.AgentModel
			}
		}
		j := &storage.Job{
			ChatID:        chatID,
			IssueID:       issueID,
			IssueTitle:    title,
			IssueBody:     body,
			IssueURL:      issueURL,
			RepoURL:       lookupURL,
			Provider:      providerName,
			GitToken:      conn.GitToken,
			Agent:         agentName,
			AgentModel:    agentModel,
			DefaultBranch: defaultBranch,
		}
		jobID, err := storage.CreateJob(b.JobsDB, j)
		if err != nil {
			b.send(chatID, fmt.Sprintf("Failed to create job: %v", err))
			return
		}
		b.send(chatID, fmt.Sprintf("⚡ QuickFix started for job #%d — PR incoming shortly.", jobID))
		job.Run(b.ctx, jobID)
	case "idle":
		if isIssueURL(text) {
			b.processIssueWithImages(chatID, text, "", "")
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
		b.startPlanMode(chatID, issueURL, gitToken, chosenBranch, "")
	case "await_branch_select":
		reposInterface, _ := conv.Data["repos"].([]interface{})
		repos := make([]map[string]interface{}, 0, len(reposInterface))
		for _, ri := range reposInterface {
			if m, ok := ri.(map[string]interface{}); ok {
				repos = append(repos, m)
			}
		}
		currentIdxFloat, _ := conv.Data["current_repo_idx"].(float64)
		currentIdx := int(currentIdxFloat)

		if currentIdx >= len(repos) {
			storage.ResetConversation(b.JobsDB, chatID, "telegram")
			b.send(chatID, "Something went wrong. Use /newissue to start over.")
			return
		}

		repo := repos[currentIdx]
		chosenBranch := strings.TrimSpace(text)
		if chosenBranch == "" {
			chosenBranch, _ = repo["default_branch"].(string)
			if chosenBranch == "" {
				chosenBranch = "main"
			}
		}
		repo["default_branch"] = chosenBranch

		repoURL := repo["repo_url"].(string)
		if err := storage.UpdateConnectionDefaultBranch(b.JobsDB, chatID, repoURL, chosenBranch); err != nil {
			log.Printf("[await_branch_select] Failed to persist branch for %s: %v", repoURL, err)
		}

		currentIdx++
		if currentIdx < len(repos) {
			storage.SetConversationState(b.JobsDB, chatID, "telegram", "await_branch_select",
				map[string]interface{}{
					"repos":            repos,
					"current_repo_idx": float64(currentIdx),
				})
			nextRepo := repos[currentIdx]
			b.send(chatID, fmt.Sprintf(
				"Select branch for [%s] %s:\nDetected default branch: *%s*\n\n"+
					"Send a branch name or press Enter to use the detected branch.",
				nextRepo["provider"], nextRepo["repo_url"], nextRepo["default_branch"],
			))
		} else {
			storage.SetConversationState(b.JobsDB, chatID, "telegram", "await_issue_desc",
				map[string]interface{}{
					"repos": repos,
				})
			repoBranchList := make([]string, len(repos))
			for i, r := range repos {
				repoBranchList[i] = fmt.Sprintf("%s (branch: %s)", r["repo_url"], r["default_branch"])
			}
			b.send(chatID, fmt.Sprintf("Describe the issue for the selected repositories:\n%s\n\nFirst line = title, rest = description.",
				strings.Join(repoBranchList, "\n")))
		}
		return
	case "await_repo_select":
		b.send(chatID, "Please select repositories using the buttons above, then tap \"Done\".")
		return
	case "await_logs":
		storage.ResetConversation(b.JobsDB, chatID, "telegram")
		var jobID int64
		fmt.Sscanf(text, "%d", &jobID)
		if jobID == 0 {
			b.send(chatID, "Invalid job ID. Use /logs to try again.")
			return
		}
		b.showLogs(chatID, jobID)
	case "await_preview":
		storage.ResetConversation(b.JobsDB, chatID, "telegram")
		var jobID int64
		fmt.Sscanf(text, "%d", &jobID)
		if jobID == 0 {
			b.send(chatID, "Invalid job ID. Use /preview to try again.")
			return
		}
		b.handlePreview(chatID, jobID)
	case "await_retry":
		storage.ResetConversation(b.JobsDB, chatID, "telegram")
		var jobID int64
		fmt.Sscanf(text, "%d", &jobID)
		if jobID == 0 {
			b.send(chatID, "Invalid job ID. Use /retry to try again.")
			return
		}
		b.retryJob(chatID, jobID)
	case "await_done":
		storage.ResetConversation(b.JobsDB, chatID, "telegram")
		if strings.TrimSpace(text) == "all" {
			b.doneAll(chatID)
			return
		}
		var jobID int64
		fmt.Sscanf(text, "%d", &jobID)
		if jobID == 0 {
			b.send(chatID, "Invalid job ID. Use /done to try again.")
			return
		}
		b.doneJob(chatID, jobID)
	case "await_followup_id":
		storage.ResetConversation(b.JobsDB, chatID, "telegram")
		var jobID int64
		fmt.Sscanf(text, "%d", &jobID)
		if jobID == 0 {
			b.send(chatID, "Invalid job ID. Use /followup to try again.")
			return
		}
		b.startFollowUp(chatID, jobID)
	case "await_cancel":
		storage.ResetConversation(b.JobsDB, chatID, "telegram")
		var jobID int64
		fmt.Sscanf(text, "%d", &jobID)
		if jobID == 0 {
			b.send(chatID, "Invalid job ID. Use /cancel to try again.")
			return
		}
		b.handleCancelJob(chatID, jobID)
	case "await_skill_id":
		storage.ResetConversation(b.JobsDB, chatID, "telegram")
		skillID := strings.TrimSpace(text)
		b.send(chatID, fmt.Sprintf("Installing skill %s...", skillID))
		go b.installSkill(chatID, agent.Skill{ID: skillID})
	case "await_issues_filter":
		storage.ResetConversation(b.JobsDB, chatID, "telegram")
		filterStatus := strings.ToLower(strings.TrimSpace(text))
		if filterStatus == "" {
			filterStatus = "open"
		}
		b.showIssues(chatID, filterStatus, 0)
	case "await_add_user":
		if !storage.IsAdmin(b.JobsDB, chatID) {
			storage.ResetConversation(b.JobsDB, chatID, "telegram")
			b.send(chatID, "Only the admin can add users.")
			return
		}
		storage.ResetConversation(b.JobsDB, chatID, "telegram")
		var userID int64
		fmt.Sscanf(text, "%d", &userID)
		if userID == 0 {
			b.send(chatID, "Invalid user ID. Use /add_user to try again.")
			return
		}
		if err := storage.AddAuthorizedUser(b.JobsDB, userID, "user", chatID); err != nil {
			b.send(chatID, fmt.Sprintf("Failed to add user: %v", err))
			return
		}
		b.send(chatID, fmt.Sprintf("User %d added.", userID))
	case "await_delete_user":
		if !storage.IsAdmin(b.JobsDB, chatID) {
			storage.ResetConversation(b.JobsDB, chatID, "telegram")
			b.send(chatID, "Only the admin can delete users.")
			return
		}
		storage.ResetConversation(b.JobsDB, chatID, "telegram")
		var userID int64
		fmt.Sscanf(text, "%d", &userID)
		if userID == 0 {
			b.send(chatID, "Invalid user ID. Use /delete_user to try again.")
			return
		}
		if userID == chatID {
			b.send(chatID, "You cannot delete yourself.")
			return
		}
		if err := storage.RemoveAuthorizedUser(b.JobsDB, userID); err != nil {
			b.send(chatID, fmt.Sprintf("Failed to delete user: %v", err))
			return
		}
		b.send(chatID, fmt.Sprintf("User %d removed.", userID))
	}
}

// embedImages downloads images from Telegram URLs and embeds them in the issue
// body by uploading to the provider's hosting. Images that fail to upload or
// would exceed the body size limit are skipped.
func embedImages(body, images, providerName, token, repoURL string) string {
	if images == "" {
		return body
	}

	entries := strings.Split(images, ";")
	var imageMarkdown strings.Builder
	for _, entry := range entries {
		parts := strings.SplitN(entry, "|", 2)
		if len(parts) < 1 || parts[0] == "" {
			continue
		}
		url := parts[0]

		resp, err := http.Get(url)
		if err != nil {
			log.Printf("[embedImages] Failed to download image: %v", err)
			continue
		}
		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			log.Printf("[embedImages] Failed to read image data: %v", err)
			continue
		}

		var imgURL string
		switch providerName {
		case "github":
			imgURL, err = provider.UploadGitHubImage(token, repoURL, data)
		case "gitlab":
			imgURL, err = provider.UploadGitLabImage(token, repoURL, data)
		default:
			log.Printf("[embedImages] Unknown provider: %s", providerName)
			continue
		}
		if err != nil {
			log.Printf("[embedImages] Failed to upload image: %v", err)
			continue
		}

		markdown := fmt.Sprintf("\n\n![image](%s)", imgURL)

		// GitHub API accepts body up to 65536 chars — skip if we'd exceed that
		if len(body)+imageMarkdown.Len()+len(markdown) > 64000 {
			log.Printf("[embedImages] Skipping image, body would exceed size limit")
			continue
		}

		imageMarkdown.WriteString(markdown)
	}

	if imageMarkdown.Len() == 0 {
		return body
	}

	return body + imageMarkdown.String()
}

func (b *Bot) processIssueWithImages(chatID int64, issueURL, gitToken, images string) {
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
	if jobCount == 0 && conn != nil {
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

	b.startPlanMode(chatID, issueURL, token, defaultBranch, images)
}

func (b *Bot) startPlanMode(chatID int64, issueURL, gitToken, defaultBranch, images string) {
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
			pendingData := map[string]interface{}{
				"pending_action": "process_issue",
				"issue_url":     issueURL,
				"repo_url":      repoURL,
				"provider_name": providerName,
			}
			if saveErr := storage.SetConversationState(b.JobsDB, chatID, "telegram", "await_token_refresh", pendingData); saveErr != nil {
				log.Printf("[startPlanMode] Failed to save pending action: %v", saveErr)
			}
			b.send(chatID, fmt.Sprintf(
				"Authentication failed for %s. Your token may have expired or been revoked.\n\n"+
					"Use /connect to refresh your token for this repository.", providerName,
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

	if body != "" {
		body = provider.EnrichIssueBody(providerName, issueURL, gitToken, body)
	}

	userCfg, _ := storage.GetUserConfig(b.JobsDB, chatID)
	agentName := b.Config.DefaultAgent
	agentModel := b.Config.DefaultModel
	if userCfg != nil {
		if userCfg.Agent != "" {
			agentName = userCfg.Agent
		}
		if userCfg.AgentModel != "" {
			agentModel = userCfg.AgentModel
		}
	}

	agentCfg := &agent.Config{
		APIKeys:          b.Config.APIKeys,
		TimeoutMins:      b.Config.AgentTimeoutMins,
		WorkspaceBaseDir: b.Config.WorkspaceDir,
	}

	b.send(chatID, "🔍 Cloning repository for analysis...")

	planWorkspace, cloneErr := os.MkdirTemp(b.Config.WorkspaceDir, "plan-")
	if cloneErr != nil {
		b.send(chatID, fmt.Sprintf("❌ Failed to create workspace: %v", cloneErr))
		return
	}

	if cloneErr := git.Clone(repoURL, gitToken, planWorkspace, defaultBranch); cloneErr != nil {
		log.Printf("[startPlanMode] Failed to clone repo for plan context: %v", cloneErr)
	}

	b.send(chatID, "🤔 Analyzing issue to generate clarifying questions...")

	prompt := agent.BuildClarifyingQuestionsPrompt(title, body)
	output, agentErr := agent.RunPlanAgent(b.ctx, agentName, agentModel, planWorkspace, prompt, agentCfg)
	if agentErr != nil {
		log.Printf("[startPlanMode] Failed to generate questions: %v", agentErr)
	}

	questions := agent.ParseClarifyingQuestions(output)
	if len(questions) < 2 {
		questions = []string{
			"What specific behavior or output do you expect after the fix?",
			"Are there any edge cases or specific scenarios to consider?",
			"What is the expected timeline or priority for this fix?",
		}
	}

	storage.SetConversationState(b.JobsDB, chatID, "telegram", "await_clarifying_questions", map[string]interface{}{
		"issue_url":      issueURL,
		"git_token":      gitToken,
		"default_branch": defaultBranch,
		"images":         images,
		"provider":       providerName,
		"repo_url":       repoURL,
		"title":          title,
		"body":           body,
		"issue_id":       issueID,
		"agent_name":     agentName,
		"agent_model":    agentModel,
		"plan_workspace": planWorkspace,
	})

	var qb strings.Builder
	qb.WriteString("I have a few questions to better understand the issue:\n\n")
	for i, q := range questions {
		qb.WriteString(fmt.Sprintf("%d. %s\n", i+1, q))
	}
	qb.WriteString("\nPlease answer all questions in a single message.")
	b.send(chatID, qb.String())
}

func (b *Bot) proceedWithPlan(chatID int64, conv *storage.Conversation) {
	issueURL, _ := conv.Data["issue_url"].(string)
	gitToken, _ := conv.Data["git_token"].(string)
	defaultBranch, _ := conv.Data["default_branch"].(string)
	images, _ := conv.Data["images"].(string)
	providerName, _ := conv.Data["provider"].(string)
	repoURL, _ := conv.Data["repo_url"].(string)
	title, _ := conv.Data["title"].(string)
	body, _ := conv.Data["body"].(string)
	issueID, _ := conv.Data["issue_id"].(string)
	plan, _ := conv.Data["plan"].(string)

	storage.ResetConversation(b.JobsDB, chatID, "telegram")

	if ws, _ := conv.Data["plan_workspace"].(string); ws != "" {
		os.RemoveAll(ws)
	}

	fullBody := body
	if plan != "" {
		planSection := fmt.Sprintf("\n\n## Implementation Plan\n\n%s", plan)
		fullBody = body + planSection
	}

	j := &storage.Job{
		ChatID:        chatID,
		IssueID:       issueID,
		IssueTitle:    title,
		IssueBody:     fullBody,
		IssueURL:      issueURL,
		RepoURL:       repoURL,
		Provider:      providerName,
		GitToken:      gitToken,
		Agent:         b.Config.DefaultAgent,
		AgentModel:    b.Config.DefaultModel,
		DefaultBranch: defaultBranch,
		Images:        images,
		Plan:          plan,
	}

	jobID, err := storage.CreateJob(b.JobsDB, j)
	if err != nil {
		b.send(chatID, fmt.Sprintf("Failed to create job: %v", err))
		return
	}

	b.send(chatID, "✅ Plan confirmed! Starting implementation...")
	job.Run(b.ctx, jobID)
}

func (b *Bot) proceedWithMultiPlan(chatID int64, conv *storage.Conversation) {
	issueURL, _ := conv.Data["issue_url"].(string)
	title, _ := conv.Data["title"].(string)
	body, _ := conv.Data["body"].(string)
	issueID, _ := conv.Data["issue_id"].(string)
	images, _ := conv.Data["images"].(string)
	plan, _ := conv.Data["plan"].(string)
	groupID, _ := conv.Data["group_id"].(string)

	reposInterface, _ := conv.Data["repos"].([]interface{})

	storage.ResetConversation(b.JobsDB, chatID, "telegram")

	if ws, _ := conv.Data["plan_workspace"].(string); ws != "" {
		os.RemoveAll(ws)
	}

	fullBody := body
	if plan != "" {
		fullBody = body + fmt.Sprintf("\n\n## Implementation Plan\n\n%s", plan)
	}

	createdIDs := make([]int64, 0, len(reposInterface))
	for _, ri := range reposInterface {
		repo, ok := ri.(map[string]interface{})
		if !ok {
			continue
		}
		rURL, _ := repo["repo_url"].(string)
		rProvider, _ := repo["provider"].(string)
		rToken, _ := repo["token"].(string)
		rBranch, _ := repo["default_branch"].(string)
		if rBranch == "" {
			rBranch = "main"
		}

		j := &storage.Job{
			ChatID:        chatID,
			IssueID:       issueID,
			IssueTitle:    title,
			IssueBody:     fullBody,
			IssueURL:      issueURL,
			RepoURL:       rURL,
			Provider:      rProvider,
			GitToken:      rToken,
			Agent:         b.Config.DefaultAgent,
			AgentModel:    b.Config.DefaultModel,
			DefaultBranch: rBranch,
			Images:        images,
			GroupID:       groupID,
			Plan:          plan,
		}
		jobID, err := storage.CreateJob(b.JobsDB, j)
		if err != nil {
			b.send(chatID, fmt.Sprintf("Failed to create job for %s: %v", rURL, err))
			continue
		}
		createdIDs = append(createdIDs, jobID)
	}

	if len(createdIDs) == 0 {
		b.send(chatID, "Failed to create any jobs.")
		return
	}

	b.send(chatID, "✅ Plan confirmed! Starting implementation across all repositories...")
	job.RunGrouped(b.ctx, groupID, createdIDs)
}

func (b *Bot) continueIssueProcessing(chatID int64, issueURL, gitToken, defaultBranch, images string) {
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
			pendingData := map[string]interface{}{
				"pending_action": "process_issue",
				"issue_url":     issueURL,
				"repo_url":      repoURL,
				"provider_name": providerName,
			}
			if saveErr := storage.SetConversationState(b.JobsDB, chatID, "telegram", "await_token_refresh", pendingData); saveErr != nil {
				log.Printf("[continueIssueProcessing] Failed to save pending action: %v", saveErr)
			}
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

	// Enrich issue body with embedded images from markdown and comments
	if body != "" {
		body = provider.EnrichIssueBody(providerName, issueURL, gitToken, body)
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
		Images:        images,
	}

	jobID, err := storage.CreateJob(b.JobsDB, j)
	if err != nil {
		b.send(chatID, fmt.Sprintf("Failed to create job: %v", err))
		return
	}

	job.Run(b.ctx, jobID)
}

func (b *Bot) processMultiIssue(chatID int64, issueURL string, repos []map[string]interface{}, images string) {
	providerName := detectProvider(issueURL)

	// Fetch issue details from the first repo
	var title, body, issueID string
	var err error

	switch providerName {
	case "github":
		issue, e := provider.FetchGitHubIssue(issueURL, repos[0]["token"].(string))
		if e != nil {
			err = e
		} else {
			title = issue.Title
			body = issue.Body
			issueID = fmt.Sprintf("%d", issue.Number)
		}
	case "gitlab":
		issue, e := provider.FetchGitLabIssue(issueURL, repos[0]["token"].(string))
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
				"❌ Authentication failed for %s. Your token may have expired.\n"+
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

	// Enrich issue body with embedded images from comments
	if body != "" {
		body = provider.EnrichIssueBody(providerName, issueURL, repos[0]["token"].(string), body)
	}

	userCfg, _ := storage.GetUserConfig(b.JobsDB, chatID)
	agentName := b.Config.DefaultAgent
	agentModel := b.Config.DefaultModel
	if userCfg != nil {
		if userCfg.Agent != "" {
			agentName = userCfg.Agent
		}
		if userCfg.AgentModel != "" {
			agentModel = userCfg.AgentModel
		}
	}

	agentCfg := &agent.Config{
		APIKeys:          b.Config.APIKeys,
		TimeoutMins:      b.Config.AgentTimeoutMins,
		WorkspaceBaseDir: b.Config.WorkspaceDir,
	}

	b.send(chatID, "🔍 Cloning repositories for analysis...")

	planWorkspace, cloneErr := os.MkdirTemp(b.Config.WorkspaceDir, "plan-")
	var repoSources []string
	if cloneErr != nil {
		log.Printf("[processMultiIssue] Failed to create workspace: %v", cloneErr)
	} else {
		repoSources = make([]string, 0, len(repos))
		for _, repo := range repos {
			rURL, _ := repo["repo_url"].(string)
			rToken, _ := repo["token"].(string)
			rBranch, _ := repo["default_branch"].(string)
			owner, rName, err := git.OwnerRepo(rURL)
			if err != nil {
				log.Printf("[processMultiIssue] Failed to parse repo URL %s: %v", rURL, err)
				continue
			}
			subDir := fmt.Sprintf("%s-%s", owner, rName)
			clonePath := filepath.Join(planWorkspace, subDir)
			if cloneErr := git.Clone(rURL, rToken, clonePath, rBranch); cloneErr != nil {
				log.Printf("[processMultiIssue] Failed to clone %s: %v", rURL, cloneErr)
			} else {
				repoSources = append(repoSources, fmt.Sprintf("  - %s/%s (%s)", owner, rName, git.DetectProvider(rURL)))
			}
		}
	}

	b.send(chatID, "🤔 Analyzing issue to generate clarifying questions...")

	prompt := agent.BuildClarifyingQuestionsPrompt(title, body, repoSources...)
	output, agentErr := agent.RunPlanAgent(b.ctx, agentName, agentModel, planWorkspace, prompt, agentCfg)
	if agentErr != nil {
		log.Printf("[processMultiIssue] Failed to generate questions: %v", agentErr)
	}

	questions := agent.ParseClarifyingQuestions(output)
	if len(questions) < 2 {
		questions = []string{
			"What specific behavior or output do you expect after the fix?",
			"Are there any edge cases or specific scenarios to consider?",
			"What is the expected timeline or priority for this fix?",
		}
	}

	groupID := fmt.Sprintf("group_%d_%s", chatID, issueID)

	storage.SetConversationState(b.JobsDB, chatID, "telegram", "await_clarifying_questions", map[string]interface{}{
		"issue_url":      issueURL,
		"provider":       providerName,
		"title":          title,
		"body":           body,
		"issue_id":       issueID,
		"images":         images,
		"agent_name":     agentName,
		"agent_model":    agentModel,
		"group_id":       groupID,
		"multi_repo":     true,
		"repos":          repos,
		"repo_sources":   repoSources,
		"plan_workspace": planWorkspace,
	})

	var qb strings.Builder
	qb.WriteString("I have a few questions to better understand the issue (across all repositories):\n\n")
	for i, q := range questions {
		qb.WriteString(fmt.Sprintf("%d. %s\n", i+1, q))
	}
	qb.WriteString("\nPlease answer all questions in a single message.")
	b.send(chatID, qb.String())
}

func (b *Bot) send(chatID int64, text string) {
	if _, err := b.API.Send(tgbotapi.NewMessage(chatID, text)); err != nil {
		log.Printf("send to %d: %v", chatID, err)
	}
}

// sendChunked splits body into ≤4000-char messages to stay within Telegram's limit.
func (b *Bot) sendChunked(chatID int64, header, body string) {
	const maxLen = 4000
	prefix := header + "\n\n"
	remaining := strings.TrimSpace(body)
	first := true
	for remaining != "" {
		avail := maxLen
		if first {
			avail -= len(prefix)
		}
		var chunk string
		if len(remaining) <= avail {
			chunk = remaining
			remaining = ""
		} else {
			cutAt := avail
			if idx := strings.LastIndex(remaining[:cutAt], "\n"); idx > 0 {
				cutAt = idx
			}
			chunk = remaining[:cutAt]
			remaining = strings.TrimSpace(remaining[cutAt:])
		}
		text := chunk
		if first {
			text = prefix + chunk
			first = false
		}
		b.send(chatID, text)
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
				// Check if there's a pending action waiting for token refresh
				conv, convErr := storage.GetConversation(b.JobsDB, chatID, "telegram")
				if convErr == nil && conv.State == "await_token_refresh" {
					// Update the connection token and retry the pending action
					b.retryAfterTokenRefresh(chatID, p, token)
					return
				}
				// No pending action — proceed with normal flow
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

func (b *Bot) retryAfterTokenRefresh(chatID int64, providerName, token string) {
	log.Printf("[retryAfterTokenRefresh] Handling pending action for chatID=%d", chatID)
	conv, err := storage.GetConversation(b.JobsDB, chatID, "telegram")
	if err != nil {
		log.Printf("[retryAfterTokenRefresh] Failed to get conversation: %v", err)
		b.send(chatID, "Internal error. Please try again.")
		return
	}

	pendingAction, _ := conv.Data["pending_action"].(string)
	if pendingAction == "" {
		log.Printf("[retryAfterTokenRefresh] No pending action found")
		b.send(chatID, "No pending action found. Please try again.")
		storage.ResetConversation(b.JobsDB, chatID, "telegram")
		return
	}

	// Use provider from saved pending action if available, otherwise use the one from OAuth flow
	savedProvider, _ := conv.Data["provider_name"].(string)
	if savedProvider != "" {
		providerName = savedProvider
	}

	switch pendingAction {
	case "process_issue":
		issueURL, _ := conv.Data["issue_url"].(string)
		repoURL, _ := conv.Data["repo_url"].(string)
		images, _ := conv.Data["images"].(string)

		// Update the connection token
		existing, _ := storage.FindConnectionByRepo(b.JobsDB, chatID, repoURL)
		if existing != nil {
			if err := storage.UpdateConnectionToken(b.JobsDB, chatID, repoURL, token); err != nil {
				log.Printf("[retryAfterTokenRefresh] Failed to update token: %v", err)
			}
		}

		storage.ResetConversation(b.JobsDB, chatID, "telegram")
		b.send(chatID, "✅ Token refreshed! Continuing with your issue...")
		b.processIssueWithImages(chatID, issueURL, token, images)

	case "create_issue":
		title, _ := conv.Data["title"].(string)
		body, _ := conv.Data["body"].(string)
		repoURL, _ := conv.Data["repo_url"].(string)
		images, _ := conv.Data["images"].(string)

		// Update the connection token
		existing, _ := storage.FindConnectionByRepo(b.JobsDB, chatID, repoURL)
		if existing != nil {
			if err := storage.UpdateConnectionToken(b.JobsDB, chatID, repoURL, token); err != nil {
				log.Printf("[retryAfterTokenRefresh] Failed to update token: %v", err)
			}
		}

		storage.ResetConversation(b.JobsDB, chatID, "telegram")

		fullBody := embedImages(body, images, providerName, token, repoURL)
		var issueURL string
		switch providerName {
		case "github":
			issueURL, err = provider.CreateGitHubIssue(repoURL, token, title, fullBody)
		case "gitlab":
			issueURL, err = provider.CreateGitLabIssue(repoURL, token, title, fullBody)
		}

		if err != nil {
			b.send(chatID, fmt.Sprintf("Failed to create issue: %v", err))
			return
		}

		b.send(chatID, fmt.Sprintf("Issue created: %s\n\nProcessing it now...", issueURL))
		b.processIssueWithImages(chatID, issueURL, token, images)

	default:
		log.Printf("[retryAfterTokenRefresh] Unknown pending action: %s", pendingAction)
		b.send(chatID, "Unknown pending action. Please try again.")
		storage.ResetConversation(b.JobsDB, chatID, "telegram")
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
