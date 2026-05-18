package provider

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

func UploadGitHubImage(token, repoURL string, imageData []byte) (string, error) {
	owner, repo, err := parseRepoURL(repoURL)
	if err != nil {
		return "", err
	}

	branch, err := GetGitHubDefaultBranch(repoURL, token)
	if err != nil {
		return "", err
	}

	mimeType := http.DetectContentType(imageData)
	ext := ".png"
	switch {
	case strings.HasPrefix(mimeType, "image/jpeg"):
		ext = ".jpg"
	case strings.HasPrefix(mimeType, "image/gif"):
		ext = ".gif"
	case strings.HasPrefix(mimeType, "image/webp"):
		ext = ".webp"
	}

	filename := fmt.Sprintf(".akama/assets/%d%s", time.Now().UnixNano(), ext)

	encoded := base64.StdEncoding.EncodeToString(imageData)
	payload := map[string]string{
		"message": "Add image for issue",
		"content": encoded,
		"branch":  branch,
	}
	body, _ := json.Marshal(payload)

	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s", owner, repo, filename)
	req, err := http.NewRequest("PUT", url, strings.NewReader(string(body)))
	if err != nil {
		return "", fmt.Errorf("create upload request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("upload image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("GitHub API error %d: %s", resp.StatusCode, b)
	}

	return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/%s", owner, repo, branch, filename), nil
}

// ErrAuthPending is returned while the user hasn't completed the device flow yet.
var ErrAuthPending = errors.New("authorization pending")

// ErrAuthExpired is returned when the device code has expired.
var ErrAuthExpired = errors.New("device code expired")

type GitHubDeviceCode struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

type GitHubTokenResponse struct {
	AccessToken string `json:"access_token"`
	Error       string `json:"error"`
	ErrorDesc   string `json:"error_description"`
	Interval    int    `json:"interval"` // For slow_down response
}

// StartGitHubDeviceFlow initiates GitHub device flow. Returns the code info to show the user.
// Requires a GitHub OAuth App with device flow enabled.
func StartGitHubDeviceFlow(clientID string) (*GitHubDeviceCode, error) {
	body, _ := json.Marshal(map[string]string{
		"client_id": clientID,
		"scope":     "repo workflow",
	})
	req, err := http.NewRequest("POST", "https://github.com/login/device/code", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("device code request: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	var dc GitHubDeviceCode
	if err := json.Unmarshal(raw, &dc); err != nil {
		return nil, fmt.Errorf("decode device code: %w", err)
	}
	if dc.DeviceCode == "" {
		return nil, fmt.Errorf("empty device code from GitHub: %s", raw)
	}
	return &dc, nil
}

// PollGitHubToken polls for the access token after the user authorizes.
// Returns (token, nil, 0) on success, ("", ErrAuthPending, interval) if still waiting,
// or ("", ErrAuthExpired, 0) / ("", error, 0) on failure.
func PollGitHubToken(clientID, clientSecret, deviceCode string) (string, error, int) {
	body, _ := json.Marshal(map[string]string{
		"client_id":     clientID,
		"client_secret": clientSecret,
		"device_code":   deviceCode,
		"grant_type":    "urn:ietf:params:oauth:grant-type:device_code",
	})
	req, err := http.NewRequest("POST", "https://github.com/login/oauth/access_token", bytes.NewReader(body))
	if err != nil {
		return "", err, 0
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("poll token: %w", err), 0
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	log.Printf("[PollGitHubToken] Response status=%d, body=%s", resp.StatusCode, string(raw))

	var result GitHubTokenResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("decode token response: %w", err), 0
	}
	log.Printf("[PollGitHubToken] Parsed: access_token=%t, error=%q, error_description=%q, interval=%d",
		result.AccessToken != "", result.Error, result.ErrorDesc, result.Interval)
	switch result.Error {
	case "":
		if result.AccessToken == "" {
			return "", fmt.Errorf("empty access token in response: %s", string(raw)), 0
		}
		return result.AccessToken, nil, 0
	case "authorization_pending":
		return "", ErrAuthPending, result.Interval
	case "slow_down":
		// GitHub sends a new interval to use
		log.Printf("[PollGitHubToken] slow_down received, new interval=%d", result.Interval)
		return "", ErrAuthPending, result.Interval
	case "expired_token":
		return "", ErrAuthExpired, 0
	default:
		return "", fmt.Errorf("GitHub OAuth error: %s - %s", result.Error, result.ErrorDesc), 0
	}
 }

type GitHubIssue struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	Number int    `json:"number"`
}

type GitHubPR struct {
	HTMLURL string `json:"html_url"`
}

type GitHubPRRequest struct {
	Title string `json:"title"`
	Head  string `json:"head"`
	Base  string `json:"base"`
	Body  string `json:"body"`
}

type GitHubIssueRequest struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

type GitHubIssueResponse struct {
	HTMLURL string `json:"html_url"`
	Number  int    `json:"number"`
}

func CreateGitHubIssue(repoURL, token, title, body string) (string, error) {
	owner, repo, err := parseRepoURL(repoURL)
	if err != nil {
		return "", err
	}
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues", owner, repo)
	data, _ := json.Marshal(GitHubIssueRequest{Title: title, Body: body})
	req, err := http.NewRequest("POST", url, strings.NewReader(string(data)))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("create issue: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("GitHub API error %d: %s", resp.StatusCode, b)
	}
	var issue GitHubIssueResponse
	if err := json.NewDecoder(resp.Body).Decode(&issue); err != nil {
		return "", fmt.Errorf("decode issue: %w", err)
	}
	return issue.HTMLURL, nil
}

// fetchGitHubIssueComments fetches the comments for a GitHub issue.
func fetchGitHubIssueComments(owner, repo string, issueNum int, token string) []string {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues/%d/comments", owner, repo, issueNum)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	var comments []struct {
		Body string `json:"body"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&comments); err != nil {
		return nil
	}

	var bodies []string
	for _, c := range comments {
		if c.Body != "" {
			bodies = append(bodies, c.Body)
		}
	}
	return bodies
}

func FetchGitHubIssue(repoURL, token string) (*GitHubIssue, error) {
	owner, repo, issueNum, err := parseGitHubIssueURL(repoURL)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues/%d", owner, repo, issueNum)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	log.Printf("[FetchGitHubIssue] Fetching %s with token prefix: %s...", url, token[:10])
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch issue: %w", err)
	}
	defer resp.Body.Close()

	log.Printf("[FetchGitHubIssue] Response status: %d", resp.StatusCode)
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API error %d: %s", resp.StatusCode, body)
	}

	var issue GitHubIssue
	if err := json.NewDecoder(resp.Body).Decode(&issue); err != nil {
		return nil, fmt.Errorf("decode issue: %w", err)
	}
	return &issue, nil
}

func GetGitHubDefaultBranch(repoURL, token string) (string, error) {
	owner, repo, err := parseRepoURL(repoURL)
	if err != nil {
		return "", err
	}
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s", owner, repo)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch repo: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("GitHub API error %d: %s", resp.StatusCode, b)
	}
	var result struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode repo: %w", err)
	}
	if result.DefaultBranch == "" {
		return "main", nil
	}
	return result.DefaultBranch, nil
}

func CreateGitHubPR(repoURL, token, title, branch, baseBranch, body string) (string, error) {
	owner, repo, err := parseRepoURL(repoURL)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls", owner, repo)
	prReq := GitHubPRRequest{
		Title: title,
		Head:  branch,
		Base:  baseBranch,
		Body:  body,
	}

	data, err := json.Marshal(prReq)
	if err != nil {
		return "", fmt.Errorf("marshal PR request: %w", err)
	}

	req, err := http.NewRequest("POST", url, strings.NewReader(string(data)))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("create PR: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("GitHub API error %d: %s", resp.StatusCode, body)
	}

	var pr GitHubPR
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return "", fmt.Errorf("decode PR: %w", err)
	}
	return pr.HTMLURL, nil
}

// IsPRAlreadyExists returns true when the provider rejected PR creation because one exists.
func IsPRAlreadyExists(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "already exists") || strings.Contains(msg, "422")
}

// FindExistingPR fetches the URL of an open PR/MR for the given branch.
func FindExistingPR(repoURL, token, branch, providerName string) (string, error) {
	switch providerName {
	case "github":
		return findGitHubPR(repoURL, token, branch)
	case "gitlab":
		return findGitLabMR(repoURL, token, branch)
	}
	return "", fmt.Errorf("unsupported provider: %s", providerName)
}

func findGitHubPR(repoURL, token, branch string) (string, error) {
	owner, repo, err := parseRepoURL(repoURL)
	if err != nil {
		return "", err
	}
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls?head=%s:%s&state=open", owner, repo, owner, branch)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("find PR: %w", err)
	}
	defer resp.Body.Close()
	var prs []GitHubPR
	if err := json.NewDecoder(resp.Body).Decode(&prs); err != nil || len(prs) == 0 {
		return "", fmt.Errorf("no open PR found for branch %s", branch)
	}
	return prs[0].HTMLURL, nil
}

// parseRepoURL extracts owner and repo from a plain repo URL
// (e.g. https://github.com/owner/repo or https://github.com/owner/repo/issues/1).
func parseRepoURL(rawURL string) (owner, repo string, err error) {
	rawURL = strings.TrimSuffix(rawURL, ".git")
	if idx := strings.Index(rawURL, "/issues/"); idx != -1 {
		rawURL = rawURL[:idx]
	}
	parts := strings.Split(rawURL, "/")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("invalid repo URL: %s", rawURL)
	}
	return parts[len(parts)-2], parts[len(parts)-1], nil
}

// GetGitHubCIStatus polls the GitHub Checks API for CI results on a branch.
func GetGitHubCIStatus(repoURL, token, branch string) (CIStatus, error) {
	owner, repo, err := parseRepoURL(repoURL)
	if err != nil {
		return CIStatus{}, err
	}
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/commits/%s/check-runs", owner, repo, branch)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return CIStatus{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return CIStatus{}, fmt.Errorf("check runs: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		TotalCount int `json:"total_count"`
		CheckRuns  []struct {
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
			HTMLURL    string `json:"html_url"`
		} `json:"check_runs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return CIStatus{}, fmt.Errorf("decode check runs: %w", err)
	}

	if result.TotalCount == 0 {
		return CIStatus{State: "none"}, nil
	}

	var checkURL string
	allDone := true
	anyFailed := false
	for _, cr := range result.CheckRuns {
		if checkURL == "" {
			checkURL = cr.HTMLURL
		}
		if cr.Status != "completed" {
			allDone = false
		}
		switch cr.Conclusion {
		case "failure", "timed_out", "cancelled", "action_required":
			anyFailed = true
		}
	}

	if !allDone {
		return CIStatus{State: "pending", URL: checkURL}, nil
	}
	if anyFailed {
		return CIStatus{State: "failure", URL: checkURL}, nil
	}
	return CIStatus{State: "success", URL: checkURL}, nil
}

func PostGitHubComment(issueURL, token, comment string) error {
	owner, repo, issueNum, err := parseGitHubIssueURL(issueURL)
	if err != nil {
		return fmt.Errorf("parse issue URL: %w", err)
	}
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues/%d/comments", owner, repo, issueNum)
	data, _ := json.Marshal(map[string]string{"body": comment})
	req, err := http.NewRequest("POST", url, strings.NewReader(string(data)))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("post comment: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GitHub API error %d: %s", resp.StatusCode, b)
	}
	return nil
}

func parseGitHubIssueURL(issueURL string) (owner, repo string, issueNum int, err error) {
	owner, repo, err = parseRepoURL(issueURL)
	if err != nil {
		return
	}
	issueURL = strings.TrimSuffix(issueURL, ".git")

	// Parse the issue number from the original URL
	issueParts := strings.Split(issueURL, "/issues/")
	if len(issueParts) != 2 {
		err = fmt.Errorf("invalid issue URL: %s", issueURL)
		return
	}
	fmt.Sscanf(issueParts[1], "%d", &issueNum)
	return
}
