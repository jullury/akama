package provider

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var markdownImageRe = regexp.MustCompile(`!\[.*?\]\(([^\s\)]+)\)`)

// embedIssueImages downloads images referenced in markdown text and replaces
// their URLs with base64 data URIs. Handles both absolute and relative URLs.
// Images larger than 5 MB are skipped. Non-image content types are skipped.
func embedIssueImages(body, repoURL string) string {
	if body == "" {
		return body
	}

	matches := markdownImageRe.FindAllStringSubmatch(body, -1)
	if len(matches) == 0 {
		return body
	}

	imgClient := &http.Client{Timeout: 15 * time.Second}
	result := body
	for _, match := range matches {
		url := match[1]

		// Resolve relative URLs against the repo URL's origin
		if strings.HasPrefix(url, "/") {
			base := extractBaseOrigin(repoURL)
			if base == "" {
				continue
			}
			url = base + url
		}

		// Only handle HTTP(S) URLs
		if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
			continue
		}

		resp, err := imgClient.Get(url)
		if err != nil {
			continue
		}
		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			continue
		}

		// Skip very large images (> 5 MB)
		if len(data) > 5*1024*1024 {
			continue
		}

		mimeType := http.DetectContentType(data)
		if !strings.HasPrefix(mimeType, "image/") {
			continue
		}

		encoded := base64.StdEncoding.EncodeToString(data)
		dataURI := "data:" + mimeType + ";base64," + encoded
		result = strings.Replace(result, match[1], dataURI, 1)
	}

	return result
}

// httpClient is used for all provider API calls with a 30-second timeout.
// http.DefaultClient has no timeout, which causes indefinite hangs on dead connections.
var httpClient = &http.Client{Timeout: 30 * time.Second}

// CIStatus summarises the CI check result for a branch.
type CIStatus struct {
	State string // "pending", "success", "failure", "none"
	URL   string
}

// extractRepoURL extracts the base repository URL from an issue URL
// by stripping the issue path segment.
func extractRepoURL(issueURL string) string {
	for _, sep := range []string{"/-/issues/", "/-/work_items/", "/issues/"} {
		if idx := strings.Index(issueURL, sep); idx != -1 {
			return issueURL[:idx]
		}
	}
	return issueURL
}

// extractBaseOrigin extracts the scheme and host from a repo URL for resolving
// relative image URLs (e.g. https://github.com from https://github.com/owner/repo).
func extractBaseOrigin(repoURL string) string {
	repoURL = strings.TrimSuffix(repoURL, ".git")
	if idx := strings.Index(repoURL, "/issues/"); idx != -1 {
		repoURL = repoURL[:idx]
	}
	for _, sep := range []string{"/-/issues/", "/-/work_items/", "/issues/"} {
		if idx := strings.Index(repoURL, sep); idx != -1 {
			repoURL = repoURL[:idx]
			break
		}
	}
	parts := strings.SplitN(repoURL, "/", 4)
	if len(parts) >= 3 && strings.HasPrefix(parts[0], "http") {
		return parts[0] + "//" + parts[2]
	}
	return ""
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

// EnrichIssueBody downloads images referenced in the issue body and comments,
// converts them to base64 data URIs, and returns the enriched body with
// comment content appended.
func EnrichIssueBody(providerName, issueURL, token, body string) string {
	repoURL := extractRepoURL(issueURL)
	body = embedIssueImages(body, repoURL)

	switch providerName {
	case "github":
		owner, repo, issueNum, err := parseGitHubIssueURL(issueURL)
		if err != nil {
			return body
		}
		comments := fetchGitHubIssueComments(owner, repo, issueNum, token)
		for _, c := range comments {
			c = embedIssueImages(c, repoURL)
			body += "\n\n---\n" + c
		}
	case "gitlab":
		projectPath, issueIID, err := parseGitLabIssueURL(issueURL)
		if err != nil {
			return body
		}
		notes := fetchGitLabIssueNotes(projectPath, issueIID, token)
		for _, n := range notes {
			n = embedIssueImages(n, repoURL)
			body += "\n\n---\n" + n
		}
	}

	return body
}

// PostIssueComment posts a comment on the given issue.
func PostIssueComment(providerName, issueURL, token, comment string) error {
	switch providerName {
	case "github":
		return PostGitHubComment(issueURL, token, comment)
	case "gitlab":
		return PostGitLabComment(issueURL, token, comment)
	}
	return fmt.Errorf("unsupported provider: %s", providerName)
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
