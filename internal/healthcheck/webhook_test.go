package healthcheck

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"watchdawg/internal/models"
)

// makeCheckResult builds a minimal CheckResult for webhook tests.
func makeCheckResult() *models.CheckResult {
	return &models.CheckResult{
		CheckName: "test-check",
		Timestamp: time.Now(),
		Healthy:   true,
		Message:   "all good",
		Duration:  10,
		Attempt:   1,
	}
}

// ── Nil config ────────────────────────────────────────────────────────────────

func TestWebhookNotifier_NotifySuccessNilConfig(t *testing.T) {
	if err := NewWebhookNotifier().NotifySuccess(nil, makeCheckResult()); err != nil {
		t.Fatalf("expected nil error for nil config, got: %v", err)
	}
}

func TestWebhookNotifier_NotifyFailureNilConfig(t *testing.T) {
	if err := NewWebhookNotifier().NotifyFailure(nil, makeCheckResult()); err != nil {
		t.Fatalf("expected nil error for nil config, got: %v", err)
	}
}

// ── JSON body (default) ───────────────────────────────────────────────────────

func TestWebhookNotifier_SendsJSONBody(t *testing.T) {
	var gotBody []byte
	var gotContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		gotContentType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	result := makeCheckResult()
	err := NewWebhookNotifier().NotifySuccess(&models.WebhookConfig{URL: srv.URL}, result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var decoded models.CheckResult
	if err := json.Unmarshal(gotBody, &decoded); err != nil {
		t.Fatalf("expected valid JSON body, unmarshal failed: %v", err)
	}
	if decoded.CheckName != "test-check" {
		t.Fatalf("expected CheckName='test-check', got %q", decoded.CheckName)
	}
	if gotContentType != "application/json" {
		t.Fatalf("expected Content-Type 'application/json', got %q", gotContentType)
	}
}

// NotifyFailure uses the same sendWebhook path; verify it actually sends.
func TestWebhookNotifier_NotifyFailureSendsRequest(t *testing.T) {
	var called bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	err := NewWebhookNotifier().NotifyFailure(&models.WebhookConfig{URL: srv.URL}, makeCheckResult())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("expected webhook server to be called")
	}
}

// ── Body template ─────────────────────────────────────────────────────────────

func TestWebhookNotifier_BodyTemplate(t *testing.T) {
	var gotBody string
	var gotContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		gotContentType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	err := NewWebhookNotifier().NotifySuccess(&models.WebhookConfig{
		URL:          srv.URL,
		BodyTemplate: "Check: {{.CheckName}} - Healthy: {{.Healthy}}",
	}, makeCheckResult())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotBody != "Check: test-check - Healthy: true" {
		t.Fatalf("unexpected rendered body: %q", gotBody)
	}
	if gotContentType != "text/plain" {
		t.Fatalf("expected Content-Type 'text/plain' for template, got %q", gotContentType)
	}
}

func TestWebhookNotifier_InvalidTemplate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	err := NewWebhookNotifier().NotifySuccess(&models.WebhookConfig{
		URL:          srv.URL,
		BodyTemplate: "{{.Unclosed",
	}, makeCheckResult())
	if err == nil {
		t.Fatal("expected error for invalid Go template")
	}
}

// ── HTTP method ───────────────────────────────────────────────────────────────

func TestWebhookNotifier_DefaultMethodIsPOST(t *testing.T) {
	var gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	NewWebhookNotifier().NotifySuccess(&models.WebhookConfig{URL: srv.URL}, makeCheckResult())
	if gotMethod != "POST" {
		t.Fatalf("expected method POST, got %q", gotMethod)
	}
}

func TestWebhookNotifier_CustomMethod(t *testing.T) {
	var gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	NewWebhookNotifier().NotifySuccess(&models.WebhookConfig{URL: srv.URL, Method: "PUT"}, makeCheckResult())
	if gotMethod != "PUT" {
		t.Fatalf("expected method PUT, got %q", gotMethod)
	}
}

// ── Headers ───────────────────────────────────────────────────────────────────

func TestWebhookNotifier_CustomHeaders(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	NewWebhookNotifier().NotifySuccess(&models.WebhookConfig{
		URL:     srv.URL,
		Headers: map[string]string{"Authorization": "Bearer token123"},
	}, makeCheckResult())
	if gotAuth != "Bearer token123" {
		t.Fatalf("expected Authorization='Bearer token123', got %q", gotAuth)
	}
}

func TestWebhookNotifier_CustomContentTypeOverridesDefault(t *testing.T) {
	var gotContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	NewWebhookNotifier().NotifySuccess(&models.WebhookConfig{
		URL:     srv.URL,
		Headers: map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
	}, makeCheckResult())
	if gotContentType != "application/x-www-form-urlencoded" {
		t.Fatalf("expected custom Content-Type, got %q", gotContentType)
	}
}

// ── Error cases ───────────────────────────────────────────────────────────────

func TestWebhookNotifier_Non2xxResponseReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	err := NewWebhookNotifier().NotifySuccess(&models.WebhookConfig{URL: srv.URL}, makeCheckResult())
	if err == nil {
		t.Fatal("expected error for non-2xx webhook response")
	}
}

func TestWebhookNotifier_UnreachableURLReturnsError(t *testing.T) {
	err := NewWebhookNotifier().NotifySuccess(&models.WebhookConfig{URL: "http://127.0.0.1:1"}, makeCheckResult())
	if err == nil {
		t.Fatal("expected error for unreachable webhook URL")
	}
}
