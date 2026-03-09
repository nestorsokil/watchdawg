package healthcheck

import "watchdawg/internal/models"

// HistoryRecorder is called once per top-level check execution (after all retries).
// Implementations MUST be non-blocking; Record is called in the check execution hot-path.
type HistoryRecorder interface {
	Record(check *models.HealthCheck, result *models.CheckResult)
}
