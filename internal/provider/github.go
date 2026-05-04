package provider

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

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
