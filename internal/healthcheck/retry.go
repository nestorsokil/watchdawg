package healthcheck

import (
	"context"
	"time"

	"watchdawg/internal/models"
)

type singleAttemptFn func(ctx context.Context, check *models.HealthCheck, attempt int) *models.CheckResult

// executeWithRetry runs fn up to check.Retries+1 times, returning on the first
// healthy result or the last failed result. Total elapsed time is recorded on
// the returned result's Duration field.
func executeWithRetry(ctx context.Context, check *models.HealthCheck, fn singleAttemptFn) *models.CheckResult {
	startTime := time.Now()
	maxAttempts := check.Retries + 1

	var lastResult *models.CheckResult
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		result := fn(ctx, check, attempt)
		lastResult = result

		if result.Healthy {
			result.Duration = time.Since(startTime).Milliseconds()
			return result
		}

		if attempt < maxAttempts {
			time.Sleep(time.Second)
		}
	}

	lastResult.Duration = time.Since(startTime).Milliseconds()
	return lastResult
}
