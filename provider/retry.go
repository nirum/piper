package provider

import (
	"context"
	"math/rand/v2"
	"net/http"
	"time"
)

const defaultMaxRetries = 3

// isRetryable returns true for transient HTTP status codes.
func isRetryable(status int) bool {
	return status == http.StatusTooManyRequests ||
		(status >= 500 && status != http.StatusNotImplemented)
}

// retryDelay returns the backoff duration for the given attempt (0-indexed).
// It uses exponential backoff with up to 25% random jitter.
func retryDelay(attempt int) time.Duration {
	base := time.Duration(1<<uint(attempt)) * time.Second
	jitter := time.Duration(rand.Float64() * float64(base) * 0.25)
	return base + jitter
}

// withRetry executes fn, retrying up to maxRetries times on transient errors.
// It respects context cancellation between attempts.
func withRetry(ctx context.Context, maxRetries int, fn func() (*http.Response, error)) (*http.Response, error) {
	var (
		resp *http.Response
		err  error
	)
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			delay := retryDelay(attempt - 1)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		resp, err = fn()
		if err != nil {
			// Network errors are not retried — they may indicate the context
			// was cancelled or the request body was already consumed.
			return nil, err
		}

		if !isRetryable(resp.StatusCode) {
			return resp, nil
		}

		// Drain and close the body before retrying.
		resp.Body.Close()
	}
	return resp, nil
}
