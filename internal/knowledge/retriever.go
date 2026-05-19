package knowledge

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jullury/akama/internal/storage"
)

func FindSimilar(ctx context.Context, db *sql.DB, ollamaURL string, text string, limit int) ([]storage.Job, error) {
	if text == "" {
		return nil, nil
	}

	embedding, err := getEmbedding(ctx, ollamaURL, text)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	if embedding == nil {
		return nil, nil
	}

	vecStr := formatVector(embedding)
	query := `
		SELECT j.id, j.issue_title, j.issue_body, j.plan, j.agent_output, j.pr_url, j.branch_name, j.repo_url,
		       j.created_at, j.updated_at, j.last_review_check_at
		FROM job_embeddings e
		JOIN jobs j ON j.id = e.job_id
		WHERE j.status IN ('done', 'pr_created')
		ORDER BY e.embedding <=> $1::vector
		LIMIT $2
	`
	rows, err := db.QueryContext(ctx, query, vecStr, limit)
	if err != nil {
		return nil, fmt.Errorf("similarity search: %w", err)
	}
	defer rows.Close()

	var jobs []storage.Job
	for rows.Next() {
		var j storage.Job
		var createdAt, updatedAt time.Time
		var lastReview sql.NullTime
		err := rows.Scan(&j.ID, &j.IssueTitle, &j.IssueBody, &j.Plan, &j.AgentOutput,
			&j.PRURL, &j.BranchName, &j.RepoURL, &createdAt, &updatedAt, &lastReview)
		if err != nil {
			return nil, fmt.Errorf("scan similar job: %w", err)
		}
		j.CreatedAt = createdAt
		j.UpdatedAt = updatedAt
		if lastReview.Valid {
			j.LastReviewCheckAt = &lastReview.Time
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}
