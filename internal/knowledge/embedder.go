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
	"time"

	"github.com/jullury/akama/internal/storage"
)

const EmbeddingModel = "nomic-embed-text"

type ollamaEmbedResponse struct {
	Embedding []float32 `json:"embedding"`
}

// EnsureModel waits for Ollama to be ready and pulls the named model
// if it is not already available. Intended to run once at daemon startup.
func EnsureModel(ctx context.Context, ollamaURL, model string) {
	if ollamaURL == "" || model == "" {
		return
	}

	// Wait for Ollama to become reachable.
	log.Printf("[ollama] Waiting for Ollama at %s ...", ollamaURL)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, ollamaURL+"/api/tags", nil)
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		time.Sleep(2 * time.Second)
	}
	log.Printf("[ollama] Ollama is reachable")

	// Check if the model is already pulled.
	type tagsResponse struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ollamaURL+"/api/tags", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("[ollama] Failed to list models: %v", err)
		return
	}
	defer resp.Body.Close()
	var tags tagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		log.Printf("[ollama] Failed to decode model list: %v", err)
		return
	}
	for _, m := range tags.Models {
		if m.Name == model || m.Name == model+":latest" {
			log.Printf("[ollama] Model %s already pulled", model)
			return
		}
	}

	// Pull the model.
	log.Printf("[ollama] Pulling model %s (this may take a while)...", model)
	pullBody, _ := json.Marshal(map[string]string{"name": model})
	pullReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, ollamaURL+"/api/pull", bytes.NewReader(pullBody))
	pullReq.Header.Set("Content-Type", "application/json")
	pullResp, err := http.DefaultClient.Do(pullReq)
	if err != nil {
		log.Printf("[ollama] Failed to pull model: %v", err)
		return
	}
	defer pullResp.Body.Close()
	// Read the streaming response to completion.
	io.Copy(io.Discard, pullResp.Body)
	if pullResp.StatusCode != http.StatusOK {
		log.Printf("[ollama] Pull returned status %d", pullResp.StatusCode)
		return
	}
	log.Printf("[ollama] Model %s pulled successfully", model)
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

	// Repository context for better matching
	if job.RepoURL != "" {
		parts = append(parts, "Repository: "+job.RepoURL)
	}
	if job.Provider != "" {
		parts = append(parts, "Provider: "+job.Provider)
	}

	// Issue details
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

	// Implementation plan
	if job.Plan != "" {
		plan := job.Plan
		if len(plan) > 1000 {
			plan = plan[:1000]
		}
		parts = append(parts, "Plan: "+plan)
	}

	// Agent output contains reasoning and steps taken
	if job.AgentOutput != "" {
		output := job.AgentOutput
		if len(output) > 1500 {
			output = output[:1500]
		}
		parts = append(parts, "AgentReasoning: "+output)
	}

	// Error context if job failed
	if job.ErrorMsg != "" {
		parts = append(parts, "Error: "+job.ErrorMsg)
	}

	// Result context
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
		"model":  EmbeddingModel,
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
