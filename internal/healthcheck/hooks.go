package healthcheck

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"text/template"
	"time"

	"watchdawg/internal/models"
)

type HookNotifier struct {
	client *http.Client
}

func NewHookNotifier() *HookNotifier {
	return &HookNotifier{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (n *HookNotifier) NotifySuccess(hooks []models.HookConfig, result *models.CheckResult) error {
	return n.executeHooks(hooks, result)
}

func (n *HookNotifier) NotifyFailure(hooks []models.HookConfig, result *models.CheckResult) error {
	return n.executeHooks(hooks, result)
}

func (n *HookNotifier) executeHooks(hooks []models.HookConfig, result *models.CheckResult) error {
	var errs []error
	for _, hook := range hooks {
		if err := n.executeHook(hook, result); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (n *HookNotifier) executeHook(hook models.HookConfig, result *models.CheckResult) error {
	switch {
	case hook.HTTP != nil:
		return n.sendWebhook(hook.HTTP, result)
	case hook.Kafka != nil:
		panic("kafka hook: unimplemented")
	default:
		return fmt.Errorf("hook has no configured type")
	}
}

func (n *HookNotifier) sendWebhook(config *models.WebhookConfig, result *models.CheckResult) error {
	var bodyContent string
	if config.BodyTemplate != "" {
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
		jsonBytes, err := json.Marshal(result)
		if err != nil {
			return fmt.Errorf("failed to marshal result to JSON: %w", err)
		}
		bodyContent = string(jsonBytes)
	}

	method := config.Method
	if method == "" {
		method = "POST"
	}

	req, err := http.NewRequest(method, config.URL, bytes.NewBufferString(bodyContent))
	if err != nil {
		return fmt.Errorf("failed to create webhook request: %w", err)
	}

	if config.Headers != nil {
		for key, value := range config.Headers {
			req.Header.Set(key, value)
		}
	}

	if req.Header.Get("Content-Type") == "" {
		if config.BodyTemplate != "" {
			req.Header.Set("Content-Type", "text/plain")
		} else {
			req.Header.Set("Content-Type", "application/json")
		}
	}

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned non-success status: %d", resp.StatusCode)
	}

	return nil
}
