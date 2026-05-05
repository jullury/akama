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
	askpassPath, err := writeAskpass(token)
	if err != nil {
		return err
	}
	defer os.Remove(askpassPath)

	cmd := newCommand(parentDir, askpassPath, "git", "clone", "--depth=1", repoURL, destPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git clone: %w\n%s", err, output)
	}
	// Ensure internal akama files are never committed
	gitignore := filepath.Join(destPath, ".gitignore")
	appendIfMissing(gitignore, ".git-askpass")
	appendIfMissing(gitignore, ".akama-prompt.txt")
	return nil
}

func CommitPush(repoPath, branchName, token, gitName, gitEmail, commitMsg string) error {
	if commitMsg == "" {
		commitMsg = "fix: apply changes"
	}

	askpassPath, err := writeAskpass(token)
	if err != nil {
		return err
	}
	defer os.Remove(askpassPath)

	// Only override git identity if the user has explicitly configured it.
	// If empty, git falls back to the system/global git config.
	var cmds [][]string
	if gitName != "" {
		cmds = append(cmds, []string{"git", "-C", repoPath, "config", "user.name", gitName})
	}
	if gitEmail != "" {
		cmds = append(cmds, []string{"git", "-C", repoPath, "config", "user.email", gitEmail})
	}
	cmds = append(cmds,
		[]string{"git", "-C", repoPath, "add", "-A"},
		[]string{"git", "-C", repoPath, "commit", "--allow-empty", "-m", commitMsg},
		[]string{"git", "-C", repoPath, "checkout", "-B", branchName},
	)
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

func writeAskpass(token string) (string, error) {
	f, err := os.CreateTemp("", "git-askpass-*")
	if err != nil {
		return "", fmt.Errorf("write askpass: %w", err)
	}
	if err := f.Chmod(0700); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", fmt.Errorf("chmod askpass: %w", err)
	}
	if _, err := fmt.Fprintf(f, "#!/bin/sh\necho '%s'\n", token); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", fmt.Errorf("write askpass: %w", err)
	}
	f.Close()
	return f.Name(), nil
}

func appendIfMissing(path, line string) {
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), line) {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintln(f, line)
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

// DiffStat returns a human-readable summary of changes since the previous commit.
func DiffStat(repoPath string) string {
	cmd := exec.Command("git", "-C", repoPath, "diff", "HEAD~1", "--stat", "--no-color")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
