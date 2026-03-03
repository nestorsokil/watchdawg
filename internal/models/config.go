package models

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type Config struct {
	HealthChecks []HealthCheck `json:"healthchecks"`
}

type HealthCheck struct {
	Name     string        `json:"name"`
	Type     CheckType     `json:"type"`
	Schedule string        `json:"schedule"` // cron format or interval like "30s", "5m"
	Retries  int           `json:"retries"`
	Timeout  time.Duration `json:"timeout"`

	HTTP      *HTTPCheckConfig     `json:"http,omitempty"`
	Starlark  *StarlarkCheckConfig `json:"starlark,omitempty"`
	Kafka     *KafkaCheckConfig    `json:"kafka,omitempty"`
	GRPC      *GRPCCheckConfig     `json:"grpc,omitempty"`
	OnSuccess []HookConfig         `json:"on_success,omitempty"`
	OnFailure []HookConfig         `json:"on_failure,omitempty"`
}

type CheckType string

const (
	CheckTypeHTTP     CheckType = "http"
	CheckTypeStarlark CheckType = "starlark"
	CheckTypeGRPC     CheckType = "grpc" // Future implementation
	CheckTypeKafka    CheckType = "kafka"
)

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
	var single int
	if err := json.Unmarshal(data, &single); err == nil {
		s.Codes = []int{single}
		return nil
	}

	var multiple []int
	if err := json.Unmarshal(data, &multiple); err == nil {
		s.Codes = multiple
		return nil
	}

	return fmt.Errorf("status_code must be either an integer or array of integers")
}

func (s StatusCodeMatcher) MarshalJSON() ([]byte, error) {
	if len(s.Codes) == 1 {
		return json.Marshal(s.Codes[0])
	}
	return json.Marshal(s.Codes)
}

func (s StatusCodeMatcher) Matches(statusCode int) bool {
	for _, code := range s.Codes {
		if code == statusCode {
			return true
		}
	}
	return false
}

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

type ExpectedHTTPResponse struct {
	StatusCode StatusCodeMatcher `json:"status_code"`

	// When set, the body will be parsed and available as 'result' variable in assertion
	Format ResponseFormat `json:"format,omitempty"`

	// All specified headers must be present with exact values
	Headers map[string]string `json:"headers,omitempty"`

	// Optional TLS certificate verification (default: true)
	// Set to false to skip certificate validation (useful for self-signed certs in dev/test)
	VerifyTLS *bool `json:"verify_tls,omitempty"`
}

type HTTPCheckConfig struct {
	URL     string            `json:"url"`
	Method  string            `json:"method"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    string            `json:"body,omitempty"`

	Expected ExpectedHTTPResponse `json:"expected"`

	// Assertion supports three modes:
	// 1. Simple expression: "result.status == 'ok'" (when expected.format is set)
	// 2. Simple expression: "status_code == 200 and 'success' in body"
	// 3. Full Starlark script with variables: valid/healthy and message
	Assertion string `json:"assertion,omitempty"`
}

type StarlarkCheckConfig struct {
	// Script should return a dict: {"healthy": true/false, "message": "optional message"}
	Script  string                 `json:"script"`
	Globals map[string]interface{} `json:"globals,omitempty"`
}

// KafkaCheckConfig defines a Kafka consumer health check.
// The check passes if at least one message was received on the configured topic
// within the schedule interval. While no messages have arrived since startup,
// the check reports healthy (waiting for first message).
type KafkaCheckConfig struct {
	Brokers []string `json:"brokers"`
	Topic   string   `json:"topic"`

	// GroupID is the Kafka consumer group ID. Defaults to "watchdawg-<check-name>".
	GroupID string `json:"group_id,omitempty"`

	// Format optionally parses the message value for assertion scripts.
	// Supported: "json". When set, the parsed value is available as 'result'.
	Format ResponseFormat `json:"format,omitempty"`

	// Assertion is an optional Starlark script validated against the most recently
	// received message. Available variables: value (string), key (string),
	// headers (dict), result (parsed value when format is set).
	// Supports the same simple-expression and full-script modes as HTTP checks.
	Assertion string `json:"assertion,omitempty"`
}

// GRPCCheckConfig defines a standard gRPC health check using grpc.health.v1.Health/Check.
// An empty Service performs a server-level check; a non-empty Service checks that named service.
type GRPCCheckConfig struct {
	Target    string `json:"target"`               // "host:port"
	PlainText bool   `json:"plaintext,omitempty"`  // skip TLS (common for internal services)
	VerifyTLS *bool  `json:"verify_tls,omitempty"` // false = accept self-signed certs

	// Service is the fully-qualified gRPC service name to check.
	// Leave empty for a server-level health check.
	Service string `json:"service,omitempty"`
}

type WebhookConfig struct {
	URL     string            `json:"url"`
	Method  string            `json:"method"` // Default: POST
	Headers map[string]string `json:"headers,omitempty"`

	// Template for the webhook body (will receive check result data)
	BodyTemplate string `json:"body_template,omitempty"`
}

// KafkaHookConfig defines a Kafka message to publish as a hook notification.
type KafkaHookConfig struct {
	Brokers         []string `json:"brokers"`
	Topic           string   `json:"topic"`
	MessageTemplate string   `json:"message_template,omitempty"`
}

// HookConfig is a tagged union: exactly one type key must be present.
//
//	{"http": {"url": "...", "method": "POST"}}
//	{"kafka": {"brokers": ["localhost:9092"], "topic": "alerts"}}
type HookConfig struct {
	HTTP  *WebhookConfig   `json:"http,omitempty"`
	Kafka *KafkaHookConfig `json:"kafka,omitempty"`
}
