package job

import (
	"os"
	"path/filepath"
	"time"
)

func CleanOldWorkspaces(workspaceDir string, maxAgeDays int) error {
	if maxAgeDays <= 0 {
		return nil
	}
	cutoff := time.Now().AddDate(0, 0, -maxAgeDays)
	entries, err := os.ReadDir(workspaceDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			os.RemoveAll(filepath.Join(workspaceDir, entry.Name()))
		}
	}
	return nil
}
