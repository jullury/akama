package knowledge

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jullury/akama/internal/storage"
)

// FindSimilar searches for similar completed jobs using vector similarity.
// It returns up to `limit` jobs ordered by relevance, with same-repo matches weighted higher.
func FindSimilar(ctx context.Context, db *sql.DB, ollamaURL string, text string, limit int) ([]storage.Job, error) {
	return FindSimilarForRepo(ctx, db, ollamaURL, text, "", limit)
}

// FindSimilarForRepo searches for similar completed jobs, optionally filtering by repository.
// When repoURL is provided, same-repo matches are weighted higher in the results.
func FindSimilarForRepo(ctx context.Context, db *sql.DB, ollamaURL string, text string, repoURL string, limit int) ([]storage.Job, error) {
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

	// Use a relevance score that weights same-repo matches higher
	// Cosine distance: 0 = identical, 2 = opposite
	// We boost same-repo matches by subtracting 0.5 from their distance
	query := `
		SELECT j.id, j.issue_title, j.issue_body, j.plan, j.agent_output, j.pr_url, j.branch_name, j.repo_url,
		       j.created_at, j.updated_at, j.last_review_check_at
		FROM job_embeddings e
		JOIN jobs j ON j.id = e.job_id
		WHERE j.status IN ('done', 'pr_created')
		ORDER BY 
			CASE WHEN j.repo_url = $2 THEN 
				(e.embedding <=> $1::vector) - 0.5
			ELSE 
				e.embedding <=> $1::vector
			END
		LIMIT $3
	`
	rows, err := db.QueryContext(ctx, query, vecStr, repoURL, limit)
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
