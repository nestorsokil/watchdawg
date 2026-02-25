package healthcheck

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"watchdawg/internal/models"
)

func makeHookResult(healthy bool) *models.CheckResult {
	return &models.CheckResult{
		CheckName: "test-check",
		Timestamp: time.Now(),
		Healthy:   healthy,
		Message:   "test message",
		Duration:  42,
		Attempt:   1,
	}
}

// httpHook creates a HookConfig with an HTTP webhook pointing to the given URL.
func httpHook(url string) models.HookConfig {
	return models.HookConfig{
		HTTP: &models.WebhookConfig{
			URL:    url,
			Method: "POST",
		},
	}
}

// webhookHooks wraps a WebhookConfig in a single-element HookConfig list.
func webhookHooks(cfg *models.WebhookConfig) []models.HookConfig {
	return []models.HookConfig{{HTTP: cfg}}
}

// ── List dispatch behaviour ───────────────────────────────────────────────────

func TestHookNotifier_EmptyList_ReturnsNil(t *testing.T) {
	n := NewHookNotifier()
	for _, hooks := range [][]models.HookConfig{nil, {}} {
		if err := n.NotifySuccess(hooks, makeHookResult(true)); err != nil {
			t.Fatalf("NotifySuccess: expected nil for empty list, got: %v", err)
		}
		if err := n.NotifyFailure(hooks, makeHookResult(false)); err != nil {
			t.Fatalf("NotifyFailure: expected nil for empty list, got: %v", err)
		}
	}
}

func TestHookNotifier_MultipleHTTPHooks_AllExecuteInOrder(t *testing.T) {
	var mu sync.Mutex
	var calls []string

	makeServer := func(name string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			calls = append(calls, name)
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		}))
	}

	srv1 := makeServer("first")
	defer srv1.Close()
	srv2 := makeServer("second")
	defer srv2.Close()

	n := NewHookNotifier()
	if err := n.NotifySuccess(
		[]models.HookConfig{httpHook(srv1.URL), httpHook(srv2.URL)},
		makeHookResult(true),
	); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("expected 2 hooks called, got %d", len(calls))
	}
	if calls[0] != "first" || calls[1] != "second" {
		t.Fatalf("unexpected call order: %v", calls)
	}
}

func TestHookNotifier_FailedHookDoesNotBlockSubsequent(t *testing.T) {
	n := NewHookNotifier()
	// Both hooks point to an unreachable address. If the second were skipped
	// after the first fails, errors.Join would wrap only one error; wrapping
	// two proves both were attempted.
	err := n.NotifyFailure(
		[]models.HookConfig{
			httpHook("http://localhost:0"),
			httpHook("http://localhost:0"),
		},
		makeHookResult(false),
	)
	if err == nil {
		t.Fatal("expected errors from failing hooks, got nil")
	}
	var multiErr interface{ Unwrap() []error }
	if !errors.As(err, &multiErr) || len(multiErr.Unwrap()) != 2 {
		t.Fatalf("expected both hooks to be attempted; got error: %v", err)
	}
}

func TestHookNotifier_EmptyHookConfig_ReturnsError(t *testing.T) {
	n := NewHookNotifier()
	err := n.executeHook(models.HookConfig{}, makeHookResult(true))
	if err == nil {
		t.Fatal("expected error for hook with no configured type, got nil")
	}
	if !strings.Contains(err.Error(), "no configured type") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

// ── HTTP webhook body ─────────────────────────────────────────────────────────

func TestHookNotifier_SendsJSONBody(t *testing.T) {
	var gotBody []byte
	var gotContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		gotContentType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	result := makeHookResult(true)
	if err := NewHookNotifier().NotifySuccess(webhookHooks(&models.WebhookConfig{URL: srv.URL}), result); err != nil {
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

func TestHookNotifier_BodyTemplate(t *testing.T) {
	var gotBody string
	var gotContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		gotContentType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := NewHookNotifier().NotifySuccess(webhookHooks(&models.WebhookConfig{
		URL:          srv.URL,
		BodyTemplate: "Check: {{.CheckName}} - Healthy: {{.Healthy}}",
	}), makeHookResult(true)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotBody != "Check: test-check - Healthy: true" {
		t.Fatalf("unexpected rendered body: %q", gotBody)
	}
	if gotContentType != "text/plain" {
		t.Fatalf("expected Content-Type 'text/plain' for template, got %q", gotContentType)
	}
}

func TestHookNotifier_InvalidTemplate_ReturnsError(t *testing.T) {
	// Template parsing fails before any network I/O; no real server needed.
	err := NewHookNotifier().NotifySuccess(webhookHooks(&models.WebhookConfig{
		URL:          "http://example.com",
		BodyTemplate: "{{.Unclosed",
	}), makeHookResult(true))
	if err == nil {
		t.Fatal("expected error for invalid Go template")
	}
}

// ── HTTP method ───────────────────────────────────────────────────────────────

func TestHookNotifier_DefaultMethodIsPOST(t *testing.T) {
	var gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	NewHookNotifier().NotifySuccess(webhookHooks(&models.WebhookConfig{URL: srv.URL}), makeHookResult(true))
	if gotMethod != "POST" {
		t.Fatalf("expected method POST, got %q", gotMethod)
	}
}

func TestHookNotifier_CustomMethod(t *testing.T) {
	var gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	NewHookNotifier().NotifySuccess(webhookHooks(&models.WebhookConfig{URL: srv.URL, Method: "PUT"}), makeHookResult(true))
	if gotMethod != "PUT" {
		t.Fatalf("expected method PUT, got %q", gotMethod)
	}
}

// ── HTTP headers ──────────────────────────────────────────────────────────────

func TestHookNotifier_CustomHeaders(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	NewHookNotifier().NotifySuccess(webhookHooks(&models.WebhookConfig{
		URL:     srv.URL,
		Headers: map[string]string{"Authorization": "Bearer token123"},
	}), makeHookResult(true))
	if gotAuth != "Bearer token123" {
		t.Fatalf("expected Authorization='Bearer token123', got %q", gotAuth)
	}
}

func TestHookNotifier_CustomContentTypeOverridesDefault(t *testing.T) {
	var gotContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	NewHookNotifier().NotifySuccess(webhookHooks(&models.WebhookConfig{
		URL:     srv.URL,
		Headers: map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
	}), makeHookResult(true))
	if gotContentType != "application/x-www-form-urlencoded" {
		t.Fatalf("expected custom Content-Type, got %q", gotContentType)
	}
}

// ── HTTP error cases ──────────────────────────────────────────────────────────

func TestHookNotifier_Non2xxResponse_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	err := NewHookNotifier().NotifySuccess(webhookHooks(&models.WebhookConfig{URL: srv.URL}), makeHookResult(true))
	if err == nil {
		t.Fatal("expected error for non-2xx webhook response")
	}
}

func TestHookNotifier_UnreachableURL_ReturnsError(t *testing.T) {
	err := NewHookNotifier().NotifySuccess(webhookHooks(&models.WebhookConfig{URL: "http://127.0.0.1:1"}), makeHookResult(true))
	if err == nil {
		t.Fatal("expected error for unreachable webhook URL")
	}
}
