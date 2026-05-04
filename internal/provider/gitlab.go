package provider

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

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
