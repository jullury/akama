package knowledge

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"
)

// KnowledgeUsage tracks how knowledge base is used for each job
type KnowledgeUsage struct {
	ID                int64
	JobID             int64
	KnowledgeFilePath string
	SimilarJobsFound  int
	SimilarJobIDs     string // Comma-separated list of similar job IDs
	AgentReferenced   bool   // Whether the agent actually referenced the knowledge
	CreatedAt         time.Time
}

// TrackUsage records that knowledge base was used for a job
func TrackUsage(ctx context.Context, db *sql.DB, jobID int64, knowledgePath string, similarJobs []int64) error {
	if db == nil || knowledgePath == "" {
		return nil
	}

	similarJobIDs := ""
	for i, id := range similarJobs {
		if i > 0 {
			similarJobIDs += ","
		}
		similarJobIDs += fmt.Sprintf("%d", id)
	}

	_, err := db.ExecContext(ctx, `
		INSERT INTO knowledge_usage (job_id, knowledge_file_path, similar_jobs_found, similar_job_ids, created_at)
		VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (job_id) DO UPDATE SET
			knowledge_file_path = EXCLUDED.knowledge_file_path,
			similar_jobs_found = EXCLUDED.similar_jobs_found,
			similar_job_ids = EXCLUDED.similar_job_ids
	`, jobID, knowledgePath, len(similarJobs), similarJobIDs)

	if err != nil {
		log.Printf("track knowledge usage for job %d: %v", jobID, err)
		return err
	}
	return nil
}

// MarkAgentReferenced marks that the agent actually used the knowledge base
func MarkAgentReferenced(ctx context.Context, db *sql.DB, jobID int64) error {
	if db == nil {
		return nil
	}

	_, err := db.ExecContext(ctx, `
		UPDATE knowledge_usage 
		SET agent_referenced = true 
		WHERE job_id = $1
	`, jobID)

	if err != nil {
		log.Printf("mark agent referenced for job %d: %v", jobID, err)
		return err
	}
	return nil
}

// GetUsageStats returns statistics about knowledge base usage
func GetUsageStats(ctx context.Context, db *sql.DB) (map[string]interface{}, error) {
	if db == nil {
		return nil, nil
	}

	stats := make(map[string]interface{})

	// Total jobs that used knowledge
	var totalUsed int
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM knowledge_usage
	`).Scan(&totalUsed)
	if err != nil {
		return nil, err
	}
	stats["total_jobs_with_knowledge"] = totalUsed

	// Jobs where agent actually referenced knowledge
	var agentReferenced int
	err = db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM knowledge_usage WHERE agent_referenced = true
	`).Scan(&agentReferenced)
	if err != nil {
		return nil, err
	}
	stats["agent_referenced_knowledge"] = agentReferenced

	// Average similar jobs found
	var avgSimilar float64
	err = db.QueryRowContext(ctx, `
		SELECT COALESCE(AVG(similar_jobs_found), 0) FROM knowledge_usage
	`).Scan(&avgSimilar)
	if err != nil {
		return nil, err
	}
	stats["avg_similar_jobs_found"] = avgSimilar

	// Success rate of jobs that used knowledge vs those that didn't
	var withKnowledgeSuccess int
	err = db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM knowledge_usage ku
		JOIN jobs j ON j.id = ku.job_id
		WHERE j.status IN ('done', 'pr_created')
	`).Scan(&withKnowledgeSuccess)
	if err != nil {
		return nil, err
	}
	stats["jobs_with_knowledge_succeeded"] = withKnowledgeSuccess

	return stats, nil
}

// GetMostEffectiveSimilarJobs returns similar jobs that led to successful fixes
func GetMostEffectiveSimilarJobs(ctx context.Context, db *sql.DB, limit int) ([]int64, error) {
	if db == nil {
		return nil, nil
	}

	rows, err := db.QueryContext(ctx, `
		SELECT DISTINCT j.id
		FROM jobs j
		JOIN job_embeddings je ON je.job_id = j.id
		WHERE j.status IN ('done', 'pr_created')
		AND j.agent_output IS NOT NULL
		AND j.agent_output != ''
		ORDER BY j.updated_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
