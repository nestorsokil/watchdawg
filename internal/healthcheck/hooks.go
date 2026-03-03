package healthcheck

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"text/template"
	"time"

	"watchdawg/internal/models"
)

type HookNotifier struct {
	client         *http.Client
	kafkaPublisher *KafkaPublisher
	logger         *slog.Logger
}

func NewHookNotifier(logger *slog.Logger) *HookNotifier {
	return &HookNotifier{
		client:         &http.Client{Timeout: 10 * time.Second},
		kafkaPublisher: NewKafkaPublisher(),
		logger:         logger,
	}
}

func (n *HookNotifier) NotifySuccess(ctx context.Context, hooks []models.HookConfig, result *models.CheckResult) error {
	return n.executeHooks(ctx, hooks, result)
}

func (n *HookNotifier) NotifyFailure(ctx context.Context, hooks []models.HookConfig, result *models.CheckResult) error {
	return n.executeHooks(ctx, hooks, result)
}

func (n *HookNotifier) executeHooks(ctx context.Context, hooks []models.HookConfig, result *models.CheckResult) error {
	var errs []error
	for _, hook := range hooks {
		if err := n.executeHook(ctx, hook, result); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (n *HookNotifier) executeHook(ctx context.Context, hook models.HookConfig, result *models.CheckResult) error {
	switch {
	case hook.HTTP != nil:
		return n.sendWebhook(ctx, hook.HTTP, result)
	case hook.Kafka != nil:
		return n.sendKafkaMessage(ctx, hook.Kafka, result)
	default:
		return fmt.Errorf("hook has no configured type")
	}
}

func (n *HookNotifier) sendWebhook(ctx context.Context, config *models.WebhookConfig, result *models.CheckResult) (err error) {
	var bodyContent string
	if config.BodyTemplate != "" {
		if bodyContent, err = buildFromTemplate(config.BodyTemplate, "webhook", result); err != nil {
			return err
		}
	} else {
		var jsonBytes []byte
		if jsonBytes, err = json.Marshal(result); err != nil {
			return fmt.Errorf("failed to marshal result to JSON: %w", err)
		}
		bodyContent = string(jsonBytes)
	}

	method := config.Method
	if method == "" {
		method = "POST"
	}

	req, err := http.NewRequestWithContext(ctx, method, config.URL, bytes.NewBufferString(bodyContent))
	if err != nil {
		return fmt.Errorf("failed to create webhook request: %w", err)
	}

	if config.Headers != nil {
		for key, value := range config.Headers {
			req.Header.Set(key, value)
		}
	}

	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	n.logger.Debug("Sending webhook", "url", config.URL, "method", method, "check", result.CheckName)

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

// sendKafkaMessage publishes a notification message to the configured Kafka topic.
// The message body is either rendered from MessageTemplate (Go template with
// CheckResult context) or the full CheckResult marshaled as JSON.
func (n *HookNotifier) sendKafkaMessage(ctx context.Context, config *models.KafkaHookConfig, result *models.CheckResult) (err error) {
	var messageBody string
	if config.MessageTemplate != "" {
		if messageBody, err = buildFromTemplate(config.MessageTemplate, "kafka_hook", result); err != nil {
			return err
		}
	} else {
		var jsonBytes []byte
		if jsonBytes, err = json.Marshal(result); err != nil {
			return fmt.Errorf("failed to marshal result to JSON for kafka hook: %w", err)
		}
		messageBody = string(jsonBytes)
	}

	if err := n.kafkaPublisher.Publish(ctx, config.Brokers, config.Topic, []byte(messageBody)); err != nil {
		return fmt.Errorf("failed to write kafka hook message to topic '%s': %w", config.Topic, err)
	}

	return nil
}

func buildFromTemplate(tmpl, tmplname string, result *models.CheckResult) (string, error) {
	t, err := template.New(tmplname).Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("failed to parse message template '%s': %w", tmplname, err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, result); err != nil {
		return "", fmt.Errorf("failed to execute message template '%s': %w", tmplname, err)
	}
	return buf.String(), nil
}
