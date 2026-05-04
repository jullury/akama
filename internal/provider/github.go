package provider

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

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
		"scope":     "repo",
	})
	req, err := http.NewRequest("POST", "https://github.com/login/device/code", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
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

	resp, err := http.DefaultClient.Do(req)
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

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch issue: %w", err)
	}
	defer resp.Body.Close()

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

func CreateGitHubPR(repoURL, token, title, branch, body string) (string, error) {
	owner, repo, _, err := parseGitHubIssueURL(repoURL)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls", owner, repo)
	prReq := GitHubPRRequest{
		Title: title,
		Head:  branch,
		Base:  "main",
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

	resp, err := http.DefaultClient.Do(req)
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

func parseGitHubIssueURL(repoURL string) (owner, repo string, issueNum int, err error) {
	repoURL = strings.TrimSuffix(repoURL, ".git")
	parts := strings.Split(repoURL, "/")
	if len(parts) < 2 {
		err = fmt.Errorf("invalid GitHub URL: %s", repoURL)
		return
	}
	owner = parts[len(parts)-2]
	repo = parts[len(parts)-1]

	issueParts := strings.Split(repoURL, "/issues/")
	if len(issueParts) != 2 {
		err = fmt.Errorf("invalid issue URL: %s", repoURL)
		return
	}
	fmt.Sscanf(issueParts[1], "%d", &issueNum)
	return
}
