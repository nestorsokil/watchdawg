package models

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Config represents the main configuration file structure
type Config struct {
	HealthChecks []HealthCheck `json:"healthchecks"`
}

// HealthCheck represents a single health check configuration
type HealthCheck struct {
	Name     string        `json:"name"`
	Type     CheckType     `json:"type"`
	Schedule string        `json:"schedule"` // cron format or interval like "30s", "5m"
	Retries  int           `json:"retries"`
	Timeout  time.Duration `json:"timeout"`

	// HTTP specific configuration
	HTTP *HTTPCheckConfig `json:"http,omitempty"`

	// Starlark specific configuration
	Starlark *StarlarkCheckConfig `json:"starlark,omitempty"`

	// Webhook configuration for success/failure notifications
	OnSuccess *WebhookConfig `json:"on_success,omitempty"`
	OnFailure *WebhookConfig `json:"on_failure,omitempty"`
}

// CheckType defines the type of health check
type CheckType string

const (
	CheckTypeHTTP     CheckType = "http"
	CheckTypeStarlark CheckType = "starlark"
	CheckTypeGRPC     CheckType = "grpc" // Future implementation
	CheckTypeKafka    CheckType = "kafka" // Future implementation
)

// ResponseFormat defines the expected response body format
type ResponseFormat string

const (
	ResponseFormatNone ResponseFormat = ""
	ResponseFormatJSON ResponseFormat = "json"
	ResponseFormatXML  ResponseFormat = "xml"
)

// StatusCodeMatcher handles both single status code and multiple acceptable codes
type StatusCodeMatcher struct {
	Codes []int
}

// UnmarshalJSON allows status_code to be either a single int or array of ints
func (s *StatusCodeMatcher) UnmarshalJSON(data []byte) error {
	// Try to unmarshal as single int first
	var single int
	if err := json.Unmarshal(data, &single); err == nil {
		s.Codes = []int{single}
		return nil
	}

	// Try to unmarshal as array of ints
	var multiple []int
	if err := json.Unmarshal(data, &multiple); err == nil {
		s.Codes = multiple
		return nil
	}

	return fmt.Errorf("status_code must be either an integer or array of integers")
}

// MarshalJSON marshals StatusCodeMatcher back to JSON
func (s StatusCodeMatcher) MarshalJSON() ([]byte, error) {
	if len(s.Codes) == 1 {
		return json.Marshal(s.Codes[0])
	}
	return json.Marshal(s.Codes)
}

// Matches checks if a status code matches any of the acceptable codes
func (s StatusCodeMatcher) Matches(statusCode int) bool {
	for _, code := range s.Codes {
		if code == statusCode {
			return true
		}
	}
	return false
}

// String returns a string representation of acceptable status codes
func (s StatusCodeMatcher) String() string {
	if len(s.Codes) == 1 {
		return fmt.Sprintf("%d", s.Codes[0])
	}
	codes := make([]string, len(s.Codes))
	for i, code := range s.Codes {
		codes[i] = fmt.Sprintf("%d", code)
	}
	return fmt.Sprintf("[%s]", strings.Join(codes, ", "))
}

// ExpectedHTTPResponse defines expected response criteria
type ExpectedHTTPResponse struct {
	// Expected HTTP status code(s) - can be a single int or array of acceptable codes
	StatusCode StatusCodeMatcher `json:"status_code"`

	// Optional response format for automatic parsing (json, xml)
	// When set, the body will be parsed and available as 'result' variable in assertion
	Format ResponseFormat `json:"format,omitempty"`

	// Optional expected headers (all must match)
	// Key-value pairs of headers that must be present with exact values
	Headers map[string]string `json:"headers,omitempty"`

	// Optional TLS certificate verification (default: true)
	// Set to false to skip certificate validation (useful for self-signed certs in dev/test)
	VerifyTLS *bool `json:"verify_tls,omitempty"`
}

// HTTPCheckConfig contains HTTP-specific check configuration
type HTTPCheckConfig struct {
	URL     string            `json:"url"`
	Method  string            `json:"method"` // GET, POST, etc.
	Headers map[string]string `json:"headers,omitempty"`
	Body    string            `json:"body,omitempty"`

	// Expected response criteria
	Expected ExpectedHTTPResponse `json:"expected"`

	// Optional assertion - supports three modes:
	// 1. Simple expression: "result.status == 'ok'" (when expected.format is set)
	// 2. Simple expression: "status_code == 200 and 'success' in body"
	// 3. Full Starlark script with variables: valid/healthy and message
	Assertion string `json:"assertion,omitempty"`
}

// StarlarkCheckConfig contains Starlark-specific check configuration
type StarlarkCheckConfig struct {
	// Script to execute for the health check
	// The script should return a dict with: {"healthy": true/false, "message": "optional message"}
	Script string `json:"script"`

	// Optional global variables to pass to the script
	Globals map[string]interface{} `json:"globals,omitempty"`
}

// WebhookConfig defines webhook notification settings
type WebhookConfig struct {
	URL     string            `json:"url"`
	Method  string            `json:"method"` // Default: POST
	Headers map[string]string `json:"headers,omitempty"`

	// Template for the webhook body (will receive check result data)
	BodyTemplate string `json:"body_template,omitempty"`
}
