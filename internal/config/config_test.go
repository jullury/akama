package config

import (
	"strings"
	"testing"
)

func TestValidate_MissingTelegramToken(t *testing.T) {
	cfg := &Config{
		TelegramToken:    "",
		AgentTimeoutMins: 30,
		DefaultAgent:     "claude",
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing telegram_token, got nil")
	}
	if !strings.Contains(err.Error(), "telegram_token") {
		t.Fatalf("error should mention 'telegram_token', got: %v", err)
	}
}

func TestValidate_InvalidTimeout(t *testing.T) {
	cfg := &Config{
		TelegramToken:    "sometoken",
		AgentTimeoutMins: 0,
		DefaultAgent:     "claude",
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for agent_timeout_mins = 0, got nil")
	}
	if !strings.Contains(err.Error(), "agent_timeout_mins") {
		t.Fatalf("error should mention 'agent_timeout_mins', got: %v", err)
	}
}

func TestValidate_MissingDefaultAgent(t *testing.T) {
	cfg := &Config{
		TelegramToken:    "sometoken",
		AgentTimeoutMins: 1,
		DefaultAgent:     "",
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing default_agent, got nil")
	}
	if !strings.Contains(err.Error(), "default_agent") {
		t.Fatalf("error should mention 'default_agent', got: %v", err)
	}
}

func TestValidate_Valid(t *testing.T) {
	cfg := &Config{
		TelegramToken:    "sometoken",
		AgentTimeoutMins: 30,
		DefaultAgent:     "claude",
	}
	err := cfg.Validate()
	if err != nil {
		t.Fatalf("expected nil for valid config, got: %v", err)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.AgentTimeoutMins != 30 {
		t.Errorf("expected AgentTimeoutMins = 30, got %d", cfg.AgentTimeoutMins)
	}
	if cfg.DefaultAgent != "claude" {
		t.Errorf("expected DefaultAgent = 'claude', got %q", cfg.DefaultAgent)
	}
	if cfg.MaxConcurrentJobs != 5 {
		t.Errorf("expected MaxConcurrentJobs = 5, got %d", cfg.MaxConcurrentJobs)
	}
	if cfg.MaxWorkspaceAgeDays != 7 {
		t.Errorf("expected MaxWorkspaceAgeDays = 7, got %d", cfg.MaxWorkspaceAgeDays)
	}
}
