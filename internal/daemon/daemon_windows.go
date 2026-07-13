//go:build windows

package daemon

import (
	"fmt"
	"os"
	"os/exec"
)

func IsProcessAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Windows, try to open the process to check if it's alive.
	err = proc.Signal(os.Kill)
	return err == nil
}

func ForkDaemon() (int, error) {
	cmd := exec.Command(os.Args[0], "start", "--daemon")
	cmd.Env = os.Environ()
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("start daemon: %w", err)
	}
	return cmd.Process.Pid, nil
}

func StopDaemon(pidPath string) error {
	pid, err := ReadPID(pidPath)
	if err != nil {
		return fmt.Errorf("read pid: %w", err)
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find process: %w", err)
	}
	if err := proc.Kill(); err != nil {
		return fmt.Errorf("kill process: %w", err)
	}
	return nil
}
