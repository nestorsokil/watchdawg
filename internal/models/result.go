package models

import "time"

type CheckResult struct {
	CheckName string    `json:"check_name"`
	Timestamp time.Time `json:"timestamp"`
	Healthy   bool      `json:"healthy"`
	Message   string    `json:"message,omitempty"`
	Duration  int64     `json:"duration_ms"`
	Attempt   int       `json:"attempt"` // 1-based retry attempt number

	HTTPResult *HTTPResult `json:"http_result,omitempty"`
	GRPCResult *GRPCResult `json:"grpc_result,omitempty"`
	Error      string      `json:"error,omitempty"`
}

type HTTPResult struct {
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers,omitempty"`
	Body       string            `json:"body,omitempty"`
	BodySize   int               `json:"body_size"`
}

type GRPCResult struct {
	// HealthStatus is the grpc.health.v1 status string: "SERVING", "NOT_SERVING",
	// "SERVICE_UNKNOWN", or "UNKNOWN".
	HealthStatus string `json:"health_status,omitempty"`
}
