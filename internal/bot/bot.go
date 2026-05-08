package bot

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/jullury/akama/internal/config"
	"github.com/jullury/akama/internal/storage"
)

type Bot struct {
	API    *tgbotapi.BotAPI
	JobsDB *sql.DB
	Config *config.Config
	ctx    context.Context
}

func New(token string) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("create bot: %w", err)
	}
	// Telegram's long-poll timeout is 60 s; give the HTTP client a 90 s hard cap
	// so a dropped connection never hangs the bot indefinitely.
	api.Client = &http.Client{Timeout: 90 * time.Second}
	log.Printf("authorized on account %s", api.Self.UserName)

	resp, err := api.Request(tgbotapi.DeleteWebhookConfig{DropPendingUpdates: true})
	if err != nil {
		return nil, fmt.Errorf("delete webhook: %w", err)
	}
	if !resp.Ok {
		return nil, fmt.Errorf("delete webhook: %s", resp.Description)
	}
	log.Printf("webhook cleared")

	// Flush any stale long-poll session from a previous daemon instance.
	// A short-poll (timeout=0) supersedes any previous long-poll and
	// leaves no active session behind. Retry on 409: a competing session
	// may have beaten our flush; each retry reclaims the slot.
	log.Printf("clearing stale polling session...")
	for attempt := 1; attempt <= 5; attempt++ {
		flush := tgbotapi.NewUpdate(0)
		flush.Timeout = 0
		if _, err := api.GetUpdates(flush); err != nil {
			if attempt < 5 {
				log.Printf("  attempt %d/5: %v — retry in 2s", attempt, err)
				time.Sleep(2 * time.Second)
				continue
			}
			log.Printf("  warning: could not clear stale session after 5 attempts: %v", err)
		} else {
			log.Printf("  stale polling session cleared")
		}
		break
	}

	// Give Telegram's server time to fully release the previous session
	// before starting our long-poll. Without this delay, the new poll can
	// conflict with a ghost session from a container that was just killed.
	time.Sleep(3 * time.Second)

	commands := []tgbotapi.BotCommand{
		{Command: "cancel", Description: "Reset conversation state"},
		{Command: "config", Description: "Configure git name, email, and model"},
		{Command: "connect", Description: "Connect repository account"},
		{Command: "connections", Description: "List saved repo connections"},
		{Command: "delete_connection", Description: "Delete a specific connection"},
		{Command: "disconnect", Description: "Delete all connections for this chat"},
		{Command: "done", Description: "Mark job done and clean up"},
		{Command: "followup", Description: "Continue working on a job"},
		{Command: "issues", Description: "List completed issues"},
		{Command: "logs", Description: "View agent output for a job"},
		{Command: "newissue", Description: "Create a new issue"},
		{Command: "queue", Description: "List pending and running jobs"},
		{Command: "retry", Description: "Retry a failed job"},
		{Command: "skills", Description: "Browse and install skillhub.club skills"},
		{Command: "start", Description: "Welcome message"},
		{Command: "status", Description: "Show last 10 jobs"},
		{Command: "update_agents", Description: "Update agents to latest version"},
		{Command: "update", Description: "Update Akama server binary to the latest version"},
		{Command: "users", Description: "List authorized users (admin only)"},
		{Command: "add_user", Description: "Add a user by Telegram user ID (admin only)"},
		{Command: "delete_user", Description: "Delete a user by Telegram user ID (admin only)"},
	}

	_, cmdErr := api.Request(tgbotapi.NewSetMyCommands(commands...))
	if cmdErr != nil {
		log.Printf("warning: failed to set commands: %v", cmdErr)
	}

	return &Bot{API: api}, nil
}

func (b *Bot) RunCtx(ctx context.Context) {
	b.ctx = ctx
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	u.AllowedUpdates = []string{"message", "callback_query"}

	consecutiveErrors := 0

	for {
		select {
		case <-ctx.Done():
			log.Println("bot context cancelled, stopping")
			return
		default:
		}

		updates, err := b.API.GetUpdates(u)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			consecutiveErrors++
			log.Printf("getUpdates error: %v, retrying in 3s...", err)

			// After 3 consecutive failures (likely a competing instance), re-flush
			// to reclaim the polling slot rather than just waiting and retrying.
			if consecutiveErrors%3 == 0 {
				log.Printf("persistent conflict, re-flushing to reclaim polling slot...")
				flush := tgbotapi.NewUpdate(0)
				flush.Timeout = 0
				b.API.GetUpdates(flush) //nolint:errcheck
			}

			select {
			case <-ctx.Done():
				return
			case <-time.After(3 * time.Second):
			}
			continue
		}

		consecutiveErrors = 0
		for _, update := range updates {
			u.Offset = update.UpdateID + 1
			go b.handleUpdate(update)
		}
	}
}

func (b *Bot) handleUpdate(update tgbotapi.Update) {
	log.Printf("update %d: msg=%v callback=%v", update.UpdateID, update.Message != nil, update.CallbackQuery != nil)

	var chatID int64
	if update.Message != nil {
		chatID = update.Message.Chat.ID
	} else if update.CallbackQuery != nil && update.CallbackQuery.Message != nil {
		chatID = update.CallbackQuery.Message.Chat.ID
	} else {
		return
	}

	if authorized, _ := storage.IsAuthorized(b.JobsDB, chatID); !authorized {
		msg := tgbotapi.NewMessage(chatID, "You are not authorized to use this bot. Contact the instance admin to gain access.")
		b.API.Send(msg)
		return
	}

	if update.Message != nil {
		b.handleMessage(update.Message)
	} else if update.CallbackQuery != nil {
		b.handleCallback(update.CallbackQuery)
	}
}
