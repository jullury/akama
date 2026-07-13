//go:build windows

package agent

import (
	"context"
	"os"
	"os/exec"
)

// agentCmd builds an exec.Cmd for running an agent binary directly.
// On Windows, no privilege dropping is performed.
func agentCmd(ctx context.Context, bin string, args []string, workspacePath string, cfg *Config) *exec.Cmd {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = workspacePath
	cmd.Env = os.Environ()
	for _, pair := range []struct{ k, env string }{
		{"anthropic", "ANTHROPIC_API_KEY"},
		{"openai", "OPENAI_API_KEY"},
	} {
		if v, ok := cfg.APIKeys[pair.k]; ok && v != "" {
			cmd.Env = append(cmd.Env, pair.env+"="+v)
		}
	}
	return cmd
}

// dropPrivileges is a no-op on Windows.
func dropPrivileges(_ *exec.Cmd) {}
