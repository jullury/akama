package job

import (
	"context"
	"log"
	"time"
)

// withRetry calls fn up to maxAttempts times, backing off between attempts.
// Returns immediately if ctx is cancelled. Logs each retry attempt.
func withRetry(ctx context.Context, label string, maxAttempts int, fn func() error) error {
	backoff := []time.Duration{5 * time.Second, 15 * time.Second, 45 * time.Second}
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		lastErr = fn()
		if lastErr == nil {
			return nil
		}
		if attempt+1 < maxAttempts {
			wait := backoff[min(attempt, len(backoff)-1)]
			log.Printf("%s failed (attempt %d/%d), retrying in %s: %v", label, attempt+1, maxAttempts, wait, lastErr)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(wait):
			}
		}
	}
	return lastErr
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
