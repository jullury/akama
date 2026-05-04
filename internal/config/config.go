package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	TelegramToken   string `yaml:"telegram_token"`
	AnthropicAPIKey string `yaml:"anthropic_api_key"`
	OpenAIAPIKey    string `yaml:"openai_api_key"`
	DefaultAgent    string `yaml:"default_agent"`
	DefaultModel    string `yaml:"default_model"`
	WorkspaceDir    string `yaml:"workspace_dir"`
	DBPath          string `yaml:"db_path"`
	LogPath         string `yaml:"log_path"`
	PIDPath         string `yaml:"pid_path"`
}

func DefaultConfig() *Config {
	return &Config{
		DefaultAgent: "claude",
		WorkspaceDir: "~/.akama/workspaces",
		DBPath:       "~/.akama/akama.db",
		LogPath:      "~/.akama/akama.log",
		PIDPath:      "~/.akama/akama.pid",
	}
}

func Load(path string) (*Config, error) {
	cfg := DefaultConfig()
	
	// Expand ~ in path
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("get home dir: %w", err)
		}
		path = filepath.Join(home, path[2:])
	}
	
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	cfg.expandHome()
	return cfg, nil
}

func (c *Config) Save(path string) error {
	c.expandHome()
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func (c *Config) expandHome() {
	c.WorkspaceDir = expandHome(c.WorkspaceDir)
	c.DBPath = expandHome(c.DBPath)
	c.LogPath = expandHome(c.LogPath)
	c.PIDPath = expandHome(c.PIDPath)
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}
