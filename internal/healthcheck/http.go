package healthcheck

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"go.starlark.net/starlark"

	"watchdawg/internal/models"
)

// HTTPChecker executes HTTP health checks
type HTTPChecker struct {
	client *http.Client
}

// NewHTTPChecker creates a new HTTP health checker
func NewHTTPChecker() *HTTPChecker {
	return &HTTPChecker{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Execute performs an HTTP health check with retries
func (h *HTTPChecker) Execute(ctx context.Context, check *models.HealthCheck) *models.CheckResult {
	startTime := time.Now()

	var lastResult *models.CheckResult
	maxAttempts := check.Retries + 1 // retries + initial attempt

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		result := h.executeOnce(ctx, check, attempt)
		lastResult = result

		if result.Healthy {
			result.Duration = time.Since(startTime).Milliseconds()
			return result
		}

		// If not the last attempt, wait before retrying
		if attempt < maxAttempts {
			time.Sleep(1 * time.Second)
		}
	}

	lastResult.Duration = time.Since(startTime).Milliseconds()
	return lastResult
}

func (h *HTTPChecker) executeOnce(ctx context.Context, check *models.HealthCheck, attempt int) *models.CheckResult {
	result := &models.CheckResult{
		CheckName: check.Name,
		Timestamp: time.Now(),
		Attempt:   attempt,
	}

	// Create HTTP request
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

	// Add headers
	for key, value := range check.HTTP.Headers {
		req.Header.Set(key, value)
	}

	// Choose the appropriate client based on TLS verification settings
	client := h.client
	if check.HTTP.Expected.VerifyTLS != nil && !*check.HTTP.Expected.VerifyTLS {
		// Create a custom client with TLS verification disabled
		client = &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			},
		}
	}

	// Execute request
	resp, err := client.Do(req)
	if err != nil {
		result.Healthy = false
		result.Error = fmt.Sprintf("request failed: %v", err)
		result.Message = result.Error
		return result
	}
	defer resp.Body.Close()

	// Read response body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		result.Healthy = false
		result.Error = fmt.Sprintf("failed to read response body: %v", err)
		result.Message = result.Error
		return result
	}

	// Populate HTTP result
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

	// Check status code: if expected codes are configured, must match one; otherwise require 2xx
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

	// Check expected headers if specified
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

	// If there's an assertion, execute it with Starlark
	if check.HTTP.Assertion != "" {
		valid, validationMsg, err := h.validateWithStarlark(check.HTTP.Assertion, check.HTTP.Expected.Format, result.HTTPResult)
		if err != nil {
			result.Healthy = false
			result.Error = fmt.Sprintf("validation script error: %v", err)
			result.Message = result.Error
			// Log response details for debugging script errors
			bodyPreview := result.HTTPResult.Body
			if len(bodyPreview) > 500 {
				bodyPreview = bodyPreview[:500] + "..."
			}
			log.Printf("  Response details for '%s':", check.Name)
			log.Printf("    Status: %d", result.HTTPResult.StatusCode)
			log.Printf("    Body size: %d bytes", result.HTTPResult.BodySize)
			log.Printf("    Body preview: %s", bodyPreview)
			log.Printf("    Headers: %v", result.HTTPResult.Headers)
			return result
		}

		result.Healthy = valid
		if validationMsg != "" {
			result.Message = validationMsg
		} else if valid {
			result.Message = fmt.Sprintf("HTTP check passed validation: status %d", resp.StatusCode)
		} else {
			result.Message = fmt.Sprintf("HTTP validation failed: status %d", resp.StatusCode)
			// Log response details for debugging
			bodyPreview := result.HTTPResult.Body
			if len(bodyPreview) > 500 {
				bodyPreview = bodyPreview[:500] + "..."
			}
			log.Printf("  Response details for '%s':", check.Name)
			log.Printf("    Status: %d", result.HTTPResult.StatusCode)
			log.Printf("    Body size: %d bytes", result.HTTPResult.BodySize)
			log.Printf("    Body preview: %s", bodyPreview)
			log.Printf("    Headers: %v", result.HTTPResult.Headers)
		}
		return result
	}

	result.Healthy = true
	result.Message = fmt.Sprintf("HTTP check passed: status %d", resp.StatusCode)
	return result
}

// validateWithStarlark executes a Starlark validation script on HTTP response
func (h *HTTPChecker) validateWithStarlark(script string, expectedFormat models.ResponseFormat, httpResult *models.HTTPResult) (valid bool, message string, err error) {
	// Detect if this is a simple expression or a full script
	isSimpleExpr := isSimpleExpression(script)

	// If simple expression, wrap it
	if isSimpleExpr {
		script = fmt.Sprintf("valid = %s", script)
		log.Printf("  Wrapped assertion as: %s", script)
	}

	// Create Starlark thread
	thread := &starlark.Thread{
		Name: "http-validation",
	}

	// Build globals with HTTP response data
	globals := starlark.StringDict{
		"status_code": starlark.MakeInt(httpResult.StatusCode),
		"body":        starlark.String(httpResult.Body),
		"body_size":   starlark.MakeInt(httpResult.BodySize),
	}

	// Convert headers to Starlark dict
	headersDict := &starlark.Dict{}
	for key, value := range httpResult.Headers {
		headersDict.SetKey(starlark.String(key), starlark.String(value))
	}
	globals["headers"] = headersDict

	// Parse body if expected format is specified
	if expectedFormat != models.ResponseFormatNone {
		parsedResult, parseErr := parseResponseBody(httpResult.Body, expectedFormat)
		if parseErr != nil {
			return false, "", fmt.Errorf("failed to parse response as %s: %w", expectedFormat, parseErr)
		}
		globals["result"] = parsedResult
	}

	// Execute the validation script
	_, execErr := starlark.ExecFile(thread, "validation.star", script, globals)
	if execErr != nil {
		return false, "", fmt.Errorf("script execution failed: %w", execErr)
	}

	// Debug: log what's in globals after execution
	log.Printf("  Starlark globals after execution: status_code=%v, body_size=%v, 'Google' in body=%v",
		globals["status_code"], globals["body_size"],
		strings.Contains(httpResult.Body, "Google"))

	// Extract result - the script should set a 'valid' or 'healthy' variable
	// Or it should set 'result' dict (different from the parsed 'result' variable)
	if resultVal, ok := globals["result"]; ok {
		// Only treat as validation result if it's a dict with valid/healthy fields
		if dict, isDict := resultVal.(*starlark.Dict); isDict {
			if hasValidationFields(dict) {
				return parseValidationResult(resultVal)
			}
		}
	}

	// Check for 'valid' or 'healthy' boolean
	if validVal, ok := globals["valid"]; ok {
		log.Printf("  Found 'valid' variable: %v (type: %s)", validVal, validVal.Type())
		if boolVal, ok := validVal.(starlark.Bool); ok {
			valid = bool(boolVal)
		}
	} else if healthyVal, ok := globals["healthy"]; ok {
		log.Printf("  Found 'healthy' variable: %v (type: %s)", healthyVal, healthyVal.Type())
		if boolVal, ok := healthyVal.(starlark.Bool); ok {
			valid = bool(boolVal)
		}
	} else {
		log.Printf("  No 'valid' or 'healthy' variable found in globals")
	}

	// Check for optional 'message'
	if msgVal, ok := globals["message"]; ok {
		if strVal, ok := msgVal.(starlark.String); ok {
			message = string(strVal)
		}
	}

	return valid, message, nil
}

// isSimpleExpression detects if a script is a simple expression vs full Starlark script
func isSimpleExpression(script string) bool {
	script = strings.TrimSpace(script)

	// If it contains newlines, it's likely a full script
	if strings.Contains(script, "\n") {
		return false
	}

	// If it contains assignment operators, it's a full script
	if strings.Contains(script, "valid =") ||
	   strings.Contains(script, "healthy =") ||
	   strings.Contains(script, "message =") ||
	   strings.Contains(script, "def ") ||
	   strings.HasPrefix(script, "import ") {
		return false
	}

	// Otherwise, treat as simple expression
	return true
}

// parseResponseBody parses JSON or XML response body into Starlark value
func parseResponseBody(body string, format models.ResponseFormat) (starlark.Value, error) {
	switch format {
	case models.ResponseFormatJSON:
		var data interface{}
		if err := json.Unmarshal([]byte(body), &data); err != nil {
			return nil, err
		}
		return goToStarlark(data), nil

	case models.ResponseFormatXML:
		var data interface{}
		if err := xml.Unmarshal([]byte(body), &data); err != nil {
			return nil, err
		}
		return goToStarlark(data), nil

	default:
		return starlark.None, nil
	}
}

// goToStarlark converts Go values to Starlark values
func goToStarlark(v interface{}) starlark.Value {
	switch val := v.(type) {
	case nil:
		return starlark.None
	case bool:
		return starlark.Bool(val)
	case int:
		return starlark.MakeInt(val)
	case int64:
		return starlark.MakeInt64(val)
	case float64:
		return starlark.Float(val)
	case string:
		return starlark.String(val)
	case []interface{}:
		list := &starlark.List{}
		for _, item := range val {
			list.Append(goToStarlark(item))
		}
		return list
	case map[string]interface{}:
		dict := starlark.NewDict(len(val))
		for key, value := range val {
			dict.SetKey(starlark.String(key), goToStarlark(value))
		}
		return dict
	default:
		return starlark.String(fmt.Sprintf("%v", val))
	}
}

// hasValidationFields checks if a dict has validation result fields
func hasValidationFields(dict *starlark.Dict) bool {
	_, hasValid, _ := dict.Get(starlark.String("valid"))
	_, hasHealthy, _ := dict.Get(starlark.String("healthy"))
	return hasValid || hasHealthy
}

// parseValidationResult parses a validation result dict
func parseValidationResult(val starlark.Value) (valid bool, message string, err error) {
	dict, ok := val.(*starlark.Dict)
	if !ok {
		return false, "", fmt.Errorf("result must be a dict")
	}

	// Extract "valid" or "healthy" field
	if validVal, found, _ := dict.Get(starlark.String("valid")); found {
		if boolVal, ok := validVal.(starlark.Bool); ok {
			valid = bool(boolVal)
		}
	} else if healthyVal, found, _ := dict.Get(starlark.String("healthy")); found {
		if boolVal, ok := healthyVal.(starlark.Bool); ok {
			valid = bool(boolVal)
		}
	}

	// Extract optional "message" field
	if msgVal, found, _ := dict.Get(starlark.String("message")); found {
		if strVal, ok := msgVal.(starlark.String); ok {
			message = string(strVal)
		}
	}

	return valid, message, nil
}
