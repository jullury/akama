package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

const pidFilePerm = 0644

func IsRunning(pidPath string) bool {
	pid, err := ReadPID(pidPath)
	if err != nil {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}

func ForkDaemon(logPath string) (int, error) {
	cmd := exec.Command(os.Args[0], "start", "--daemon")
	cmd.Env = os.Environ()

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return 0, fmt.Errorf("open log: %w", err)
	}
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Stdin = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("start daemon: %w", err)
	}
	return cmd.Process.Pid, nil
}

func WritePID(pidPath string, pid int) error {
	if err := os.MkdirAll(filepath.Dir(pidPath), 0755); err != nil {
		return err
	}
	return os.WriteFile(pidPath, []byte(fmt.Sprintf("%d", pid)), pidFilePerm)
}

func ReadPID(pidPath string) (int, error) {
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return 0, err
	}
	var pid int
	fmt.Sscanf(string(data), "%d", &pid)
	return pid, nil
}

func RemovePID(pidPath string) error {
	return os.Remove(pidPath)
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
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("send SIGTERM: %w", err)
	}
	// Do NOT remove the PID file here. The daemon removes it via defer on exit.
	// Removing it early would allow a new daemon to start before this one finishes
	// its graceful shutdown (job drain), causing two instances to poll Telegram simultaneously.
	return nil
}
