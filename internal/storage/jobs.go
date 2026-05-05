package storage

import (
	"database/sql"
	"fmt"
	"time"
)

type Job struct {
	ID                int64
	ChatID            int64
	IssueID           string
	IssueTitle        string
	IssueBody         string
	IssueURL          string
	RepoURL           string
	Provider          string
	GitToken          string
	Agent             string
	AgentModel        string
	Status            string
	WorkspacePath     string
	BranchName        string
	PRURL             string
	NotificationMsgID int64
	ErrorMsg          string
	AgentOutput       string
	DefaultBranch     string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func CreateJob(db *sql.DB, j *Job) (int64, error) {
	res, err := db.Exec(`
		INSERT INTO jobs (chat_id, issue_id, issue_title, issue_body, issue_url, repo_url, provider, git_token, agent, agent_model, default_branch)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		j.ChatID, j.IssueID, j.IssueTitle, j.IssueBody, j.IssueURL, j.RepoURL, j.Provider, j.GitToken, j.Agent, j.AgentModel, j.DefaultBranch)
	if err != nil {
		return 0, fmt.Errorf("create job: %w", err)
	}
	return res.LastInsertId()
}

const jobColumns = `id, chat_id, issue_id, issue_title, issue_body, issue_url, repo_url, provider, git_token, agent, agent_model, status, workspace_path, branch_name, pr_url, notification_msg_id, error_msg, agent_output, default_branch, created_at, updated_at`

func GetJob(db *sql.DB, id int64) (*Job, error) {
	row := db.QueryRow(`SELECT `+jobColumns+` FROM jobs WHERE id = ?`, id)
	return scanJob(row)
}

func GetJobByNotifMsgID(db *sql.DB, notifMsgID int64) (*Job, error) {
	row := db.QueryRow(`SELECT `+jobColumns+` FROM jobs WHERE notification_msg_id = ?`, notifMsgID)
	return scanJob(row)
}

func scanJob(row *sql.Row) (*Job, error) {
	j := &Job{}
	var createdAt, updatedAt string
	err := row.Scan(&j.ID, &j.ChatID, &j.IssueID, &j.IssueTitle, &j.IssueBody, &j.IssueURL,
		&j.RepoURL, &j.Provider, &j.GitToken, &j.Agent, &j.AgentModel, &j.Status,
		&j.WorkspacePath, &j.BranchName, &j.PRURL, &j.NotificationMsgID, &j.ErrorMsg,
		&j.AgentOutput, &j.DefaultBranch, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	j.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	j.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt)
	return j, nil
}

func SetJobRunning(db *sql.DB, id int64, workspacePath string) error {
	_, err := db.Exec(`UPDATE jobs SET status = 'running', workspace_path = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		workspacePath, id)
	return err
}

func SetJobPRCreated(db *sql.DB, id int64, branch, prURL string) error {
	_, err := db.Exec(`UPDATE jobs SET status = 'pr_created', branch_name = ?, pr_url = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		branch, prURL, id)
	return err
}

// RecoverInterruptedJobs marks jobs left in running/awaiting_input as failed
// (they were orphaned by a daemon restart) and resets any blocked conversation states.
func RecoverInterruptedJobs(db *sql.DB) error {
	rows, err := db.Query(`SELECT chat_id FROM jobs WHERE status = 'awaiting_input'`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var chatID int64
		if rows.Scan(&chatID) == nil {
			ResetConversation(db, chatID, "telegram")
		}
	}
	_, err = db.Exec(`UPDATE jobs SET status = 'failed', error_msg = 'interrupted by daemon restart', updated_at = CURRENT_TIMESTAMP WHERE status IN ('running','awaiting_input')`)
	if err != nil {
		return err
	}
	// Reset conversations stuck in await_branch_confirm (no job created yet)
	db.Exec(`UPDATE conversations SET state = 'idle', data = '{}', updated_at = CURRENT_TIMESTAMP WHERE state = 'await_branch_confirm'`)
	return nil
}

func FindActiveJobByIssue(db *sql.DB, chatID int64, issueURL string) *Job {
	row := db.QueryRow(`SELECT `+jobColumns+` FROM jobs WHERE chat_id = ? AND issue_url = ? AND status IN ('pending','running','awaiting_input') ORDER BY created_at DESC LIMIT 1`,
		chatID, issueURL)
	j, _ := scanJob(row)
	return j
}

func SetJobAgentOutput(db *sql.DB, id int64, output string) error {
	_, err := db.Exec(`UPDATE jobs SET agent_output = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, output, id)
	return err
}

func SetJobAwaitingInput(db *sql.DB, id int64, agentOutput string) error {
	_, err := db.Exec(`UPDATE jobs SET status = 'awaiting_input', agent_output = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		agentOutput, id)
	return err
}

func SetJobFailed(db *sql.DB, id int64, errMsg string) error {
	_, err := db.Exec(`UPDATE jobs SET status = 'failed', error_msg = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		errMsg, id)
	return err
}

func SetJobStatus(db *sql.DB, id int64, status string) error {
	_, err := db.Exec(`UPDATE jobs SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		status, id)
	return err
}

func SetJobNotifMsgID(db *sql.DB, id int64, msgID int64) error {
	_, err := db.Exec(`UPDATE jobs SET notification_msg_id = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		msgID, id)
	return err
}

func UpdateJobToken(db *sql.DB, id int64, token string) error {
	_, err := db.Exec(`UPDATE jobs SET git_token = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		token, id)
	return err
}

func CountJobsByRepo(db *sql.DB, chatID int64, repoURL string) (int, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE chat_id = ? AND repo_url = ?`, chatID, repoURL).Scan(&count)
	return count, err
}

func CountActiveJobs(db *sql.DB) (int, error) {
	row := db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE status IN ('pending', 'running', 'updating')`)
	var count int
	err := row.Scan(&count)
	return count, err
}

func ListJobsByChatID(db *sql.DB, chatID int64, limit int) ([]*Job, error) {
	rows, err := db.Query(`SELECT `+jobColumns+` FROM jobs WHERE chat_id = ? ORDER BY created_at DESC LIMIT ?`, chatID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var jobs []*Job
	for rows.Next() {
		j := &Job{}
		var createdAt, updatedAt string
		err := rows.Scan(&j.ID, &j.ChatID, &j.IssueID, &j.IssueTitle, &j.IssueBody, &j.IssueURL,
			&j.RepoURL, &j.Provider, &j.GitToken, &j.Agent, &j.AgentModel, &j.Status,
			&j.WorkspacePath, &j.BranchName, &j.PRURL, &j.NotificationMsgID, &j.ErrorMsg,
			&j.AgentOutput, &j.DefaultBranch, &createdAt, &updatedAt)
		if err != nil {
			return nil, err
		}
		j.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		j.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt)
		jobs = append(jobs, j)
	}
	return jobs, nil
}

func ListJobs(db *sql.DB, limit int) ([]*Job, error) {
	rows, err := db.Query(`SELECT `+jobColumns+` FROM jobs ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var jobs []*Job
	for rows.Next() {
		j := &Job{}
		var createdAt, updatedAt string
		err := rows.Scan(&j.ID, &j.ChatID, &j.IssueID, &j.IssueTitle, &j.IssueBody, &j.IssueURL,
			&j.RepoURL, &j.Provider, &j.GitToken, &j.Agent, &j.AgentModel, &j.Status,
			&j.WorkspacePath, &j.BranchName, &j.PRURL, &j.NotificationMsgID, &j.ErrorMsg,
			&j.AgentOutput, &j.DefaultBranch, &createdAt, &updatedAt)
		if err != nil {
			return nil, err
		}
		j.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		j.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt)
		jobs = append(jobs, j)
	}
	return jobs, nil
}
