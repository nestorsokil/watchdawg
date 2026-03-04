package healthcheck

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"text/template"
	"time"

	"watchdawg/internal/models"
)

type HookNotifier struct {
	client         *http.Client
	kafkaPublisher *KafkaPublisher
	logger         *slog.Logger
	recorder       MetricsRecorder
}

func NewHookNotifier(logger *slog.Logger, recorder MetricsRecorder) *HookNotifier {
	return &HookNotifier{
		client:         &http.Client{Timeout: 10 * time.Second},
		kafkaPublisher: NewKafkaPublisher(),
		logger:         logger,
		recorder:       recorder,
	}
}

func (n *HookNotifier) Close() {
	n.kafkaPublisher.Close()
}

func (n *HookNotifier) NotifySuccess(ctx context.Context, hooks []models.HookConfig, result *models.CheckResult) error {
	return n.executeHooks(ctx, hooks, result, "on_success")
}

func (n *HookNotifier) NotifyFailure(ctx context.Context, hooks []models.HookConfig, result *models.CheckResult) error {
	return n.executeHooks(ctx, hooks, result, "on_failure")
}

func (n *HookNotifier) executeHooks(ctx context.Context, hooks []models.HookConfig, result *models.CheckResult, trigger string) error {
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
	)
	for _, hook := range hooks {
		wg.Add(1)
		go func() {
			defer wg.Done()
			hookType, target := hookTypeAndTarget(hook)
			start := time.Now()
			err := n.executeHook(ctx, hook, result)
			durationSec := time.Since(start).Seconds()
			hookResult := "success"
			if err != nil {
				hookResult = "failure"
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
			n.recorder.RecordHookExecution(result.CheckName, hookType, target, trigger, hookResult)
			n.recorder.RecordHookDuration(result.CheckName, hookType, target, trigger, durationSec)
		}()
	}
	wg.Wait()
	return errors.Join(errs...)
}

func hookTypeAndTarget(hook models.HookConfig) (hookType, target string) {
	switch {
	case hook.HTTP != nil:
		return "http", hook.HTTP.URL
	case hook.Kafka != nil:
		return "kafka", hook.Kafka.Topic
	default:
		return "unknown", ""
	}
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
