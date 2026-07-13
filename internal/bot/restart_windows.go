//go:build windows

package bot

import (
	"fmt"
	"os"
	"os/exec"
)

// spawnDetachedRestart starts a detached helper process that waits for the
// current process to exit, pauses briefly, then starts a new daemon.
func spawnDetachedRestart(exePath string) error {
	script := fmt.Sprintf("ping -n 4 127.0.0.1 >nul 2>&1 & \"%s\" start", exePath)
	helper := exec.Command("cmd", "/C", "start", "/B", "cmd", "/C", script)
	return helper.Start()
}

// signalSelf kills the current process to trigger a shutdown.
func signalSelf() {
	proc, _ := os.FindProcess(os.Getpid())
	proc.Kill()
}
