package healthcheck

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"text/template"
	"time"

	"watchdawg/internal/models"
)

// WebhookNotifier sends webhook notifications for health check results
type WebhookNotifier struct {
	client *http.Client
}

// NewWebhookNotifier creates a new webhook notifier
func NewWebhookNotifier() *WebhookNotifier {
	return &WebhookNotifier{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// NotifySuccess sends a success webhook notification
func (w *WebhookNotifier) NotifySuccess(config *WebhookConfig, result *models.CheckResult) error {
	if config == nil {
		return nil
	}
	return w.sendWebhook(config, result)
}

// NotifyFailure sends a failure webhook notification
func (w *WebhookNotifier) NotifyFailure(config *WebhookConfig, result *models.CheckResult) error {
	if config == nil {
		return nil
	}
	return w.sendWebhook(config, result)
}

func (w *WebhookNotifier) sendWebhook(config *WebhookConfig, result *models.CheckResult) error {
	// Prepare request body
	var bodyContent string
	if config.BodyTemplate != "" {
		// Use template if provided
		tmpl, err := template.New("webhook").Parse(config.BodyTemplate)
		if err != nil {
			return fmt.Errorf("failed to parse body template: %w", err)
		}

		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, result); err != nil {
			return fmt.Errorf("failed to execute body template: %w", err)
		}
		bodyContent = buf.String()
	} else {
		// Default: send JSON-encoded result
		jsonBytes, err := json.Marshal(result)
		if err != nil {
			return fmt.Errorf("failed to marshal result to JSON: %w", err)
		}
		bodyContent = string(jsonBytes)
	}

	// Determine HTTP method
	method := config.Method
	if method == "" {
		method = "POST"
	}

	// Create request
	req, err := http.NewRequest(method, config.URL, bytes.NewBufferString(bodyContent))
	if err != nil {
		return fmt.Errorf("failed to create webhook request: %w", err)
	}

	// Add headers
	if config.Headers != nil {
		for key, value := range config.Headers {
			req.Header.Set(key, value)
		}
	}

	// Set default Content-Type if not provided
	if req.Header.Get("Content-Type") == "" {
		if config.BodyTemplate != "" {
			req.Header.Set("Content-Type", "text/plain")
		} else {
			req.Header.Set("Content-Type", "application/json")
		}
	}

	// Send request
	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned non-success status: %d", resp.StatusCode)
	}

	return nil
}

// WebhookConfig is re-exported from models for convenience
type WebhookConfig = models.WebhookConfig
