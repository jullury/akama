package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

var Version = "dev"
var BuildTime = "unknown"
var BuildPlatform = "unknown"

type Config struct {
	TelegramToken    string            `yaml:"telegram_token"`
	APIKeys          map[string]string `yaml:"api_keys"`
	DefaultAgent     string `yaml:"default_agent"`
	DefaultModel     string `yaml:"default_model"`
	AgentTimeoutMins int    `yaml:"agent_timeout_mins"`
	WorkspaceDir     string `yaml:"workspace_dir"`
	DBPath           string `yaml:"db_path"`
	LogPath          string `yaml:"log_path"`
	PIDPath          string `yaml:"pid_path"`
	AdminUserID      int64  `yaml:"admin_user_id"`
}

// GetAPIKey returns the API key for the given provider, or empty string.
func (c *Config) GetAPIKey(provider string) string {
	if c.APIKeys == nil {
		return ""
	}
	return c.APIKeys[provider]
}

// SetAPIKey sets the API key for the given provider.
func (c *Config) SetAPIKey(provider, key string) {
	if c.APIKeys == nil {
		c.APIKeys = make(map[string]string)
	}
	c.APIKeys[provider] = key
}

func DefaultConfig() *Config {
	return &Config{
		APIKeys:          make(map[string]string),
		DefaultAgent:     "claude",
		AgentTimeoutMins: 30,
		WorkspaceDir:     "~/.akama/workspaces",
		DBPath:           "~/.akama/akama.db",
		LogPath:          "~/.akama/akama.log",
		PIDPath:          "~/.akama/akama.pid",
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

	// First pass: check for old-format keys to migrate
	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err == nil {
		if cfg.APIKeys == nil {
			cfg.APIKeys = make(map[string]string)
		}
		migrated := false
		if v, ok := raw["anthropic_api_key"].(string); ok && v != "" {
			cfg.APIKeys["anthropic"] = v
			migrated = true
		}
		if v, ok := raw["openai_api_key"].(string); ok && v != "" {
			cfg.APIKeys["openai"] = v
			migrated = true
		}
		// Second pass: unmarshal into struct (api_keys map will be populated)
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, err
		}
		// If we migrated, save the new format
		if migrated {
			cfg.Save(path)
		}
	} else {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, err
		}
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
