package job

import (
	"context"
	"errors"
	"testing"
)

func TestWithRetry_SucceedsOnFirstTry(t *testing.T) {
	calls := 0
	err := withRetry(context.Background(), "test", 3, func() error {
		calls++
		return nil
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected fn to be called exactly once, got %d", calls)
	}
}

func TestWithRetry_RetriesOnFailure(t *testing.T) {
	// Use a context with cancel so we can cancel mid-sleep to speed things up.
	// The function retries with 5s/15s backoff — we cancel after the 2nd failure
	// to avoid slow tests. Instead, we make fn succeed on the 3rd call.
	// We set maxAttempts=3; fn fails twice then succeeds.
	// The sleeps between attempt 1→2 and 2→3 would be 5s and 15s.
	// To avoid that, run with a context that stays valid (no cancel), but
	// override by using a very small maxAttempts and forcing success fast.

	// Use a cancellable context: cancel only AFTER fn returns nil (i.e., never needed).
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	calls := 0
	sentinelErr := errors.New("temporary failure")

	// We cheat the sleep by having fn succeed on the 2nd call with maxAttempts=2.
	// This means only one sleep (5s). To avoid that we set maxAttempts=1 so fn
	// only gets one chance — but we need it to retry. Use maxAttempts=3 but succeed
	// on attempt 1 (0-indexed) — the 2nd call.
	// There's one 5s sleep between attempt 0 and attempt 1. To skip it we cancel
	// ctx after fn returns nil... but cancel is deferred. Let's just test with
	// maxAttempts=1 (no sleep possible) for the "retry" scenario using a direct check.

	// Actually: succeed on the 3rd call with maxAttempts=3 would require 2 sleeps.
	// The cleanest approach: succeed on attempt index 1 (2nd call), maxAttempts=3.
	// Only 1 sleep of 5s. Use a short-circuit: if ctx is already cancelled before sleep,
	// it returns immediately. We can't easily avoid the sleep without patching the function.
	// Instead, use maxAttempts=2 so fn fails once, then succeeds. One 5s sleep.
	// To avoid that sleep entirely: pre-cancel ctx after fn returns nil. But cancel()
	// runs in the select — if fn returns nil on 2nd call, no sleep happens anyway.

	// With maxAttempts=2: attempt 0 fails → sleep 5s → attempt 1 succeeds.
	// The sleep IS triggered. This test would take 5s. Instead use maxAttempts=1: only 1 try.
	// That doesn't test retry. So let's use maxAttempts=3 and succeed on attempt 2 (3rd).
	// That would incur 5s + 15s = 20s sleep. Unacceptable.

	// Best approach given the fixed backoff: succeed on the 2nd attempt (1 sleep of 5s).
	// Mark this test with a 10s timeout guard and accept the 5s delay.
	// OR: pre-cancel ctx right after 2nd fn call returns nil using a side effect.
	// The select in withRetry: if ctx is Done, returns ctx.Err(). We need ctx to NOT
	// be done when fn returns nil, but to avoid sleeping we can't.

	// Compromise: succeed on attempt 0 (1st call) with a different fn that fails then succeeds,
	// but do it with maxAttempts=2 (1 retry). Accept the 5s sleep in tests.
	// Mark it with t.Skip if -short is set.
	if testing.Short() {
		t.Skip("skipping retry sleep test in short mode")
	}

	calls = 0
	err := withRetry(ctx, "test-retry", 2, func() error {
		calls++
		if calls < 2 {
			return sentinelErr
		}
		return nil
	})
	if err != nil {
		t.Fatalf("expected nil error after retry, got %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected fn to be called 2 times, got %d", calls)
	}
}

func TestWithRetry_AbortsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	calls := 0
	err := withRetry(ctx, "test-cancel", 3, func() error {
		calls++
		// Cancel the context after the first failed attempt so the sleep
		// select fires ctx.Done() immediately.
		cancel()
		return errors.New("always fails")
	})

	if err == nil {
		t.Fatal("expected error when context is cancelled, got nil")
	}
	// Should have returned ctx.Err() (context.Canceled)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestWithRetry_AllFail(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping all-fail retry test in short mode (involves sleeps)")
	}

	sentinelErr := errors.New("persistent failure")
	calls := 0

	err := withRetry(context.Background(), "test-all-fail", 3, func() error {
		calls++
		return sentinelErr
	})

	if err == nil {
		t.Fatal("expected error when all attempts fail, got nil")
	}
	if !errors.Is(err, sentinelErr) {
		t.Fatalf("expected sentinel error, got %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}
