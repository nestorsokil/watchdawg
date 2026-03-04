package healthcheck

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"go.starlark.net/starlark"

	"watchdawg/internal/models"
	"watchdawg/internal/starlarkeval"
)

type HTTPChecker struct {
	NoOpInitializer
	client   *http.Client
	logger   *slog.Logger
	recorder MetricsRecorder
}

func NewHTTPChecker(logger *slog.Logger, recorder MetricsRecorder) *HTTPChecker {
	return &HTTPChecker{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger:   logger,
		recorder: recorder,
	}
}

func (k *HTTPChecker) IsMatching(check *models.HealthCheck) bool { return check.HTTP != nil }

func (h *HTTPChecker) Execute(ctx context.Context, check *models.HealthCheck) *models.CheckResult {
	return executeWithRetry(ctx, check, h.executeOnce)
}

func (h *HTTPChecker) executeOnce(ctx context.Context, check *models.HealthCheck, attempt int) *models.CheckResult {
	attemptStart := time.Now()
	result := &models.CheckResult{
		CheckName: check.Name,
		Timestamp: attemptStart,
		Attempt:   attempt,
	}
	defer func() {
		h.recorder.RecordCheckAttempt(check.Name, result.Healthy, time.Since(attemptStart).Seconds())
	}()

	var bodyReader io.Reader
	if check.HTTP.Body != "" {
		bodyReader = bytes.NewBufferString(check.HTTP.Body)
	}

	req, err := http.NewRequestWithContext(ctx, check.HTTP.Method, check.HTTP.URL, bodyReader)
	if err != nil {
		result.Healthy = false
		result.Error = fmt.Sprintf("failed to create request: %v", err)
		result.Message = result.Error
		return result
	}

	for key, value := range check.HTTP.Headers {
		req.Header.Set(key, value)
	}

	client := h.client
	if check.HTTP.Expected.VerifyTLS != nil && !*check.HTTP.Expected.VerifyTLS {
		client = &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			},
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		result.Healthy = false
		result.Error = fmt.Sprintf("request failed: %v", err)
		result.Message = result.Error
		return result
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		result.Healthy = false
		result.Error = fmt.Sprintf("failed to read response body: %v", err)
		result.Message = result.Error
		return result
	}

	result.HTTPResult = &models.HTTPResult{
		StatusCode: resp.StatusCode,
		Body:       string(bodyBytes),
		BodySize:   len(bodyBytes),
		Headers:    make(map[string]string),
	}

	for key, values := range resp.Header {
		if len(values) > 0 {
			result.HTTPResult.Headers[key] = values[0]
		}
	}

	// if expected codes are configured, must match one; otherwise require 2xx
	if len(check.HTTP.Expected.StatusCode.Codes) > 0 {
		if !check.HTTP.Expected.StatusCode.Matches(resp.StatusCode) {
			result.Healthy = false
			result.Message = fmt.Sprintf("unexpected status code: got %d, expected %s",
				resp.StatusCode, check.HTTP.Expected.StatusCode.String())
			return result
		}
	} else if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		result.Healthy = false
		result.Message = fmt.Sprintf("unexpected status code: got %d, expected 2xx", resp.StatusCode)
		return result
	}

	if len(check.HTTP.Expected.Headers) > 0 {
		for expectedKey, expectedValue := range check.HTTP.Expected.Headers {
			actualValue, found := result.HTTPResult.Headers[expectedKey]
			if !found {
				result.Healthy = false
				result.Message = fmt.Sprintf("missing expected header: %s", expectedKey)
				return result
			}
			if actualValue != expectedValue {
				result.Healthy = false
				result.Message = fmt.Sprintf("header %s mismatch: got %q, expected %q",
					expectedKey, actualValue, expectedValue)
				return result
			}
		}
	}

	if check.HTTP.Assertion != "" {
		valid, validationMsg, err := h.validateWithStarlark(ctx, check.HTTP.Assertion, check.HTTP.Expected.Format, result.HTTPResult)
		if err != nil {
			result.Healthy = false
			result.Error = fmt.Sprintf("validation script error: %v", err)
			result.Message = result.Error
			bodyPreview := result.HTTPResult.Body
			if len(bodyPreview) > 500 {
				bodyPreview = bodyPreview[:500] + "..."
			}
			h.logger.Debug("Assertion script error, response details",
				"check", check.Name,
				"status", result.HTTPResult.StatusCode,
				"body_size", result.HTTPResult.BodySize,
				"body_preview", bodyPreview,
				"headers", result.HTTPResult.Headers,
			)
			return result
		}

		result.Healthy = valid
		if validationMsg != "" {
			result.Message = validationMsg
		} else if valid {
			result.Message = fmt.Sprintf("HTTP check passed validation: status %d", resp.StatusCode)
		} else {
			result.Message = fmt.Sprintf("HTTP validation failed: status %d", resp.StatusCode)
			bodyPreview := result.HTTPResult.Body
			if len(bodyPreview) > 500 {
				bodyPreview = bodyPreview[:500] + "..."
			}
			h.logger.Debug("Assertion validation failed, response details",
				"check", check.Name,
				"status", result.HTTPResult.StatusCode,
				"body_size", result.HTTPResult.BodySize,
				"body_preview", bodyPreview,
				"headers", result.HTTPResult.Headers,
			)
		}
		return result
	}

	result.Healthy = true
	result.Message = fmt.Sprintf("HTTP check passed: status %d", resp.StatusCode)
	return result
}

func (h *HTTPChecker) validateWithStarlark(ctx context.Context, script string, expectedFormat models.ResponseFormat, httpResult *models.HTTPResult) (valid bool, message string, err error) {
	globals := starlark.StringDict{
		"status_code": starlark.MakeInt(httpResult.StatusCode),
		"body":        starlark.String(httpResult.Body),
		"body_size":   starlark.MakeInt(httpResult.BodySize),
	}

	headersDict := &starlark.Dict{}
	for key, value := range httpResult.Headers {
		headersDict.SetKey(starlark.String(key), starlark.String(value))
	}
	globals["headers"] = headersDict

	if expectedFormat != models.ResponseFormatNone {
		parsedResult, parseErr := starlarkeval.ParseResponseBody(httpResult.Body, expectedFormat)
		if parseErr != nil {
			return false, "", fmt.Errorf("failed to parse response as %s: %w", expectedFormat, parseErr)
		}
		globals["result"] = parsedResult
	}

	return starlarkeval.RunAssertionScript(ctx, "http-validation", "validation.star", script, globals)
}
