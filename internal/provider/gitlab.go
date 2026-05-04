package provider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type GitLabDeviceCode struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

// StartGitLabDeviceFlow initiates GitLab device flow.
// Requires a GitLab Application with device flow / native app option enabled.
func StartGitLabDeviceFlow(clientID string) (*GitLabDeviceCode, error) {
	body, _ := json.Marshal(map[string]string{
		"client_id": clientID,
		"scope":     "api",
	})
	req, err := http.NewRequest("POST", "https://gitlab.com/oauth/authorize_device", bytes.NewReader(body))
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

	var dc GitLabDeviceCode
	if err := json.Unmarshal(raw, &dc); err != nil {
		return nil, fmt.Errorf("decode device code: %w", err)
	}
	if dc.DeviceCode == "" {
		return nil, fmt.Errorf("empty device code from GitLab: %s", raw)
	}
	return &dc, nil
}

// PollGitLabToken polls for the access token after the user authorizes.
func PollGitLabToken(clientID, clientSecret, deviceCode string) (string, error, int) {
	body, _ := json.Marshal(map[string]string{
		"client_id":     clientID,
		"client_secret": clientSecret,
		"device_code":   deviceCode,
		"grant_type":    "urn:ietf:params:oauth:grant-type:device_code",
	})
	req, err := http.NewRequest("POST", "https://gitlab.com/oauth/token", bytes.NewReader(body))
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

	var result struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode token response: %w", err), 0
	}
	switch result.Error {
	case "":
		if result.AccessToken == "" {
			return "", fmt.Errorf("empty access token in response"), 0
		}
		return result.AccessToken, nil, 0
	case "authorization_pending", "slow_down":
		return "", ErrAuthPending, 0
	case "expired_token":
		return "", ErrAuthExpired, 0
	default:
		return "", fmt.Errorf("GitLab OAuth error: %s", result.Error), 0
	}
}

type GitLabIssue struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	IID         int    `json:"iid"`
}

type GitLabMR struct {
	WebURL string `json:"web_url"`
}

type GitLabMRRequest struct {
	Title         string `json:"title"`
	SourceBranch   string `json:"source_branch"`
	TargetBranch   string `json:"target_branch"`
	Description    string `json:"description"`
}

func FetchGitLabIssue(repoURL, token string) (*GitLabIssue, error) {
	projectPath, issueIID, err := parseGitLabIssueURL(repoURL)
	if err != nil {
		return nil, err
	}

	encodedPath := strings.ReplaceAll(projectPath, "/", "%2F")
	url := fmt.Sprintf("https://gitlab.com/api/v4/projects/%s/issues/%d", encodedPath, issueIID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("PRIVATE-TOKEN", token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch issue: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitLab API error %d: %s", resp.StatusCode, body)
	}

	var issue GitLabIssue
	if err := json.NewDecoder(resp.Body).Decode(&issue); err != nil {
		return nil, fmt.Errorf("decode issue: %w", err)
	}
	return &issue, nil
}

func CreateGitLabMR(repoURL, token, title, branch, body string) (string, error) {
	projectPath, _, err := parseGitLabIssueURL(repoURL)
	if err != nil {
		return "", err
	}

	encodedPath := strings.ReplaceAll(projectPath, "/", "%2F")
	url := fmt.Sprintf("https://gitlab.com/api/v4/projects/%s/merge_requests", encodedPath)
	mrReq := GitLabMRRequest{
		Title:       title,
		SourceBranch: branch,
		TargetBranch: "main",
		Description:  body,
	}

	data, err := json.Marshal(mrReq)
	if err != nil {
		return "", fmt.Errorf("marshal MR request: %w", err)
	}

	req, err := http.NewRequest("POST", url, strings.NewReader(string(data)))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("PRIVATE-TOKEN", token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("create MR: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("GitLab API error %d: %s", resp.StatusCode, body)
	}

	var mr GitLabMR
	if err := json.NewDecoder(resp.Body).Decode(&mr); err != nil {
		return "", fmt.Errorf("decode MR: %w", err)
	}
	return mr.WebURL, nil
}

func parseGitLabIssueURL(repoURL string) (projectPath string, issueIID int, err error) {
	repoURL = strings.TrimSuffix(repoURL, ".git")
	parts := strings.Split(repoURL, "/")
	if len(parts) < 2 {
		err = fmt.Errorf("invalid GitLab URL: %s", repoURL)
		return
	}

	issueParts := strings.Split(repoURL, "/issues/")
	if len(issueParts) != 2 {
		err = fmt.Errorf("invalid issue URL: %s", repoURL)
		return
	}

	projectPath = strings.TrimPrefix(issueParts[0], "https://gitlab.com/")
	fmt.Sscanf(issueParts[1], "%d", &issueIID)
	return
}
