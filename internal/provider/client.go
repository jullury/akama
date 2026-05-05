package provider

import (
	"net/http"
	"strings"
	"time"
)

// httpClient is used for all provider API calls with a 30-second timeout.
// http.DefaultClient has no timeout, which causes indefinite hangs on dead connections.
var httpClient = &http.Client{Timeout: 30 * time.Second}

// CIStatus summarises the CI check result for a branch.
type CIStatus struct {
	State string // "pending", "success", "failure", "none"
	URL   string
}

// GetCIStatus fetches CI check results for the given branch from GitHub or GitLab.
func GetCIStatus(repoURL, token, branch, providerName string) (CIStatus, error) {
	switch providerName {
	case "github":
		return GetGitHubCIStatus(repoURL, token, branch)
	case "gitlab":
		return GetGitLabCIStatus(repoURL, token, branch)
	}
	return CIStatus{State: "none"}, nil
}

// GetDefaultBranch returns the default branch for the given repository.
// Falls back to "main" if the provider is unknown or the API call fails.
func GetDefaultBranch(repoURL, token, providerName string) string {
	var branch string
	var err error
	switch providerName {
	case "github":
		branch, err = GetGitHubDefaultBranch(repoURL, token)
	case "gitlab":
		branch, err = GetGitLabDefaultBranch(repoURL, token)
	}
	if err != nil || branch == "" {
		return "main"
	}
	return branch
}

// IsAuthError returns true when the error indicates an authentication failure.
// Covers HTTP-level errors (401/403) and git-level auth failures.
func IsAuthError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	authPatterns := []string{
		"401", "403",
		"unauthorized", "forbidden",
		"bad credentials", "token expired", "token revoked",
		"authentication failed", "permission denied",
		"could not read from remote", "access denied",
		"invalid token", "token is invalid",
	}
	for _, p := range authPatterns {
		if strings.Contains(strings.ToLower(msg), p) {
			return true
		}
	}
	return false
}
