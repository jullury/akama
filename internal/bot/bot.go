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

	resp, err := api.Request(tgbotapi.DeleteWebhookConfig{DropPendingUpdates: false})
	if err != nil {
		return nil, fmt.Errorf("delete webhook: %w", err)
	}
	if !resp.Ok {
		return nil, fmt.Errorf("delete webhook: %s", resp.Description)
	}
	log.Printf("webhook cleared")

	commands := []tgbotapi.BotCommand{
		{Command: "start", Description: "Welcome message"},
		{Command: "connect", Description: "Connect repository account"},
		{Command: "connections", Description: "List saved repo connections"},
		{Command: "disconnect", Description: "Delete all connections for this chat"},
		{Command: "config", Description: "Configure git name, email, and model"},
		{Command: "newissue", Description: "Create a new issue"},
		{Command: "issues", Description: "List completed issues"},
		{Command: "queue", Description: "List pending and running jobs"},
		{Command: "status", Description: "Show last 5 jobs"},
		{Command: "done", Description: "Mark job done and clean up"},
		{Command: "retry", Description: "Retry a failed job"},
		{Command: "cancel", Description: "Reset conversation state"},
		{Command: "update-agents", Description: "Update agents to latest version"},
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
	updates := b.API.GetUpdatesChan(u)

	for {
		select {
		case <-ctx.Done():
			log.Println("bot context cancelled, stopping")
			return
		case update := <-updates:
			go b.handleUpdate(update)
		}
	}
}

func (b *Bot) handleUpdate(update tgbotapi.Update) {
	log.Printf("update %d: msg=%v callback=%v", update.UpdateID, update.Message != nil, update.CallbackQuery != nil)
	if update.Message != nil {
		b.handleMessage(update.Message)
	} else if update.CallbackQuery != nil {
		b.handleCallback(update.CallbackQuery)
	}
}
