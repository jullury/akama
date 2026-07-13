package daemon

import (
	"fmt"
	"os"
	"path/filepath"
)

const pidFilePerm = 0644

func IsRunning(pidPath string) bool {
	pid, err := ReadPID(pidPath)
	if err != nil {
		return false
	}
	return IsProcessAlive(pid)
}

func WritePID(pidPath string, pid int) error {
	if err := os.MkdirAll(filepath.Dir(pidPath), 0755); err != nil {
		return err
	}
	return os.WriteFile(pidPath, []byte(fmt.Sprintf("%d", pid)), pidFilePerm)
}

func ClaimPIDFile(pidPath string, pid int) error {
	if err := os.MkdirAll(filepath.Dir(pidPath), 0755); err != nil {
		return err
	}
	fd, err := os.OpenFile(pidPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, pidFilePerm)
	if err != nil {
		return err
	}
	defer fd.Close()
	_, err = fd.WriteString(fmt.Sprintf("%d", pid))
	return err
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
