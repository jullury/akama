package provider

import (
	"net/http"
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
