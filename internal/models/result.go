package models

import "time"

// CheckResult represents the result of a health check execution
type CheckResult struct {
	CheckName string    `json:"check_name"`
	Timestamp time.Time `json:"timestamp"`
	Healthy   bool      `json:"healthy"`
	Message   string    `json:"message,omitempty"`
	Duration  int64     `json:"duration_ms"`
	Attempt   int       `json:"attempt"` // Which retry attempt (1-based)

	// HTTP-specific result data
	HTTPResult *HTTPResult `json:"http_result,omitempty"`

	// Error information
	Error string `json:"error,omitempty"`
}

// HTTPResult contains HTTP-specific result data
type HTTPResult struct {
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers,omitempty"`
	Body       string            `json:"body,omitempty"`
	BodySize   int               `json:"body_size"`
}
