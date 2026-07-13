//go:build !windows

package bot

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// spawnDetachedRestart starts a detached helper process that waits for the
// current process to exit, pauses briefly for Telegram to drain the old
// long-poll connection, then starts a new daemon.
func spawnDetachedRestart(exePath string) error {
	script := fmt.Sprintf("while kill -0 %d 2>/dev/null; do sleep 1; done; sleep 3; '%s' start", os.Getpid(), exePath)
	helper := exec.Command("sh", "-c", script)
	helper.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return helper.Start()
}

// signalSelf sends SIGTERM to the current process to trigger a graceful shutdown.
func signalSelf() {
	proc, _ := os.FindProcess(os.Getpid())
	proc.Signal(syscall.SIGTERM)
}
