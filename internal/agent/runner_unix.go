//go:build !windows

package agent

import (
	"context"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"syscall"
)

// workerGroups returns the supplementary GIDs for uid 1000. Without these,
// setgroups(0,nil) strips all groups before exec, which causes EACCES when a
// path component or the binary itself is only accessible via a supplementary
// group the worker user normally holds.
func workerGroups() []uint32 {
	u, err := user.LookupId("1000")
	if err != nil {
		return nil
	}
	gids, err := u.GroupIds()
	if err != nil {
		return nil
	}
	groups := make([]uint32, 0, len(gids))
	for _, g := range gids {
		id, err := strconv.ParseUint(g, 10, 32)
		if err == nil {
			groups = append(groups, uint32(id))
		}
	}
	return groups
}

// agentCmd builds an exec.Cmd for running an agent binary directly. When the
// current process is root (inside the daemon container), privileges are dropped
// to uid/gid 1000 with full supplementary groups so that agent binaries that
// refuse to run as root (claude --dangerously-skip-permissions) are satisfied,
// and so that all file permissions the worker user holds are preserved.
func agentCmd(ctx context.Context, bin string, args []string, workspacePath string, cfg *Config) *exec.Cmd {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = workspacePath
	if os.Getuid() == 0 {
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Credential: &syscall.Credential{
				Uid:    1000,
				Gid:    1000,
				Groups: workerGroups(),
			},
		}
		cmd.Env = []string{
			"HOME=/home/worker",
			"PATH=/home/worker/.npm-global/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		}
	} else {
		cmd.Env = os.Environ()
	}
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

// dropPrivileges sets SysProcAttr on cmd to drop to uid/gid 1000 when running
// as root. No-op for non-root processes.
func dropPrivileges(cmd *exec.Cmd) {
	if os.Getuid() == 0 {
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Credential: &syscall.Credential{Uid: 1000, Gid: 1000},
		}
	}
}
