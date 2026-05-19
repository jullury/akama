package knowledge

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/jullury/akama/internal/storage"
)

type ollamaEmbedResponse struct {
	Embedding []float32 `json:"embedding"`
}

func EmbedJob(ctx context.Context, db *sql.DB, ollamaURL string, job storage.Job) error {
	text := buildEmbeddingText(job)
	embedding, err := getEmbedding(ctx, ollamaURL, text)
	if err != nil {
		log.Printf("embed job %d: %v", job.ID, err)
		return nil
	}
	if embedding == nil {
		return nil
	}

	vecStr := formatVector(embedding)
	_, err = db.Exec(`
		INSERT INTO job_embeddings (job_id, embedding, indexed_at)
		VALUES ($1, $2::vector, NOW())
		ON CONFLICT (job_id) DO UPDATE SET
			embedding = EXCLUDED.embedding,
			indexed_at = NOW()
	`, job.ID, vecStr)
	if err != nil {
		log.Printf("store embedding for job %d: %v", job.ID, err)
	}
	return nil
}

func buildEmbeddingText(job storage.Job) string {
	var parts []string
	if job.IssueTitle != "" {
		parts = append(parts, "Issue: "+job.IssueTitle)
	}
	if job.IssueBody != "" {
		body := job.IssueBody
		if len(body) > 2000 {
			body = body[:2000]
		}
		parts = append(parts, body)
	}
	if job.Plan != "" {
		plan := job.Plan
		if len(plan) > 1000 {
			plan = plan[:1000]
		}
		parts = append(parts, "Plan: "+plan)
	}
	result := ""
	if job.PRURL != "" {
		result = "Result: " + job.PRURL
	}
	if job.BranchName != "" {
		if result != "" {
			result += " on branch " + job.BranchName
		} else {
			result = "Branch: " + job.BranchName
		}
	}
	if result != "" {
		parts = append(parts, result)
	}
	return strings.Join(parts, "\n\n")
}

func getEmbedding(ctx context.Context, ollamaURL, text string) ([]float32, error) {
	if text == "" {
		return nil, nil
	}

	body := map[string]interface{}{
		"model":  "nomic-embed-text",
		"prompt": text,
	}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", ollamaURL+"/api/embeddings", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("ollama returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result ollamaEmbedResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return result.Embedding, nil
}

func formatVector(embedding []float32) string {
	var b strings.Builder
	b.WriteString("[")
	for i, v := range embedding {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(fmt.Sprintf("%f", v))
	}
	b.WriteString("]")
	return b.String()
}
