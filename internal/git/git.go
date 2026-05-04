package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func Clone(repoURL, token, destPath string) error {
	if err := os.RemoveAll(destPath); err != nil {
		return fmt.Errorf("remove existing dir: %w", err)
	}
	parentDir := filepath.Dir(destPath)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return fmt.Errorf("mkdir parent: %w", err)
	}
	// Write askpass to parent dir so destPath stays absent for git clone
	askpassPath, err := writeAskpass(token, parentDir)
	if err != nil {
		return err
	}
	defer os.Remove(askpassPath)

	cmd := newCommand(parentDir, askpassPath, "git", "clone", "--depth=1", repoURL, destPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git clone: %w\n%s", err, output)
	}
	return nil
}

func CommitPush(repoPath, branchName, token string) error {
	askpassPath, err := writeAskpass(token, repoPath)
	if err != nil {
		return err
	}
	defer os.Remove(askpassPath)

	cmds := [][]string{
		{"git", "-C", repoPath, "config", "user.email", "akama@bot"},
		{"git", "-C", repoPath, "config", "user.name", "Akama"},
		{"git", "-C", repoPath, "add", "-A"},
		{"git", "-C", repoPath, "commit", "--allow-empty", "-m", "fix: apply akama agent changes"},
		{"git", "-C", repoPath, "checkout", "-B", branchName},
	}
	for _, args := range cmds {
		cmd := newCommand(repoPath, askpassPath, args[0], args[1:]...)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("%s: %w\n%s", strings.Join(args, " "), err, output)
		}
	}

	cmd := newCommand(repoPath, askpassPath, "git", "-C", repoPath, "push", "origin", branchName, "--force-with-lease")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git push: %w\n%s", err, output)
	}
	return nil
}

func writeAskpass(token, workDir string) (string, error) {
	path := filepath.Join(workDir, ".git-askpass")
	script := fmt.Sprintf("#!/bin/sh\necho '%s'\n", token)
	if err := os.WriteFile(path, []byte(script), 0700); err != nil {
		return "", fmt.Errorf("write askpass: %w", err)
	}
	return path, nil
}

func newCommand(workDir, askpassPath string, name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), "GIT_ASKPASS="+askpassPath, "GIT_TERMINAL_PROMPT=0")
	return cmd
}

func DetectProvider(repoURL string) string {
	if strings.Contains(repoURL, "github.com") {
		return "github"
	}
	if strings.Contains(repoURL, "gitlab.com") {
		return "gitlab"
	}
	return ""
}

func OwnerRepo(repoURL string) (string, string, error) {
	repoURL = strings.TrimSuffix(repoURL, ".git")
	parts := strings.Split(repoURL, "/")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("invalid repo URL: %s", repoURL)
	}
	owner := parts[len(parts)-2]
	repo := parts[len(parts)-1]
	return owner, repo, nil
}
