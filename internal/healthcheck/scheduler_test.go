package healthcheck

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"watchdawg/internal/models"
)

// ── parseSchedule ─────────────────────────────────────────────────────────────

func TestParseSchedule_SubMinuteSeconds(t *testing.T) {
	s := NewScheduler(testLogger(), NoopMetricsRecorder{})
	if got := s.parseSchedule("10s"); got != "*/10 * * * * *" {
		t.Fatalf("expected '*/10 * * * * *', got %q", got)
	}
}

func TestParseSchedule_ExactMinuteInSeconds(t *testing.T) {
	s := NewScheduler(testLogger(), NoopMetricsRecorder{})
	// 60s = 1 minute, so minutes path: "0 */1 * * * *"
	if got := s.parseSchedule("60s"); got != "0 */1 * * * *" {
		t.Fatalf("expected '0 */1 * * * *', got %q", got)
	}
}

func TestParseSchedule_Minutes(t *testing.T) {
	s := NewScheduler(testLogger(), NoopMetricsRecorder{})
	if got := s.parseSchedule("5m"); got != "0 */5 * * * *" {
		t.Fatalf("expected '0 */5 * * * *', got %q", got)
	}
}

func TestParseSchedule_Hours(t *testing.T) {
	s := NewScheduler(testLogger(), NoopMetricsRecorder{})
	if got := s.parseSchedule("2h"); got != "0 0 */2 * * *" {
		t.Fatalf("expected '0 0 */2 * * *', got %q", got)
	}
}

func TestParseSchedule_StandardCron5Field(t *testing.T) {
	s := NewScheduler(testLogger(), NoopMetricsRecorder{})
	if got := s.parseSchedule("*/5 * * * *"); got != "0 */5 * * * *" {
		t.Fatalf("expected '0 */5 * * * *', got %q", got)
	}
}

func TestParseSchedule_6FieldCronPassthrough(t *testing.T) {
	s := NewScheduler(testLogger(), NoopMetricsRecorder{})
	input := "0 */5 * * * *"
	if got := s.parseSchedule(input); got != input {
		t.Fatalf("expected %q unchanged, got %q", input, got)
	}
}

func TestParseSchedule_InvalidDurationPassthrough(t *testing.T) {
	s := NewScheduler(testLogger(), NoopMetricsRecorder{})
	// "xs" is not a valid duration and not a recognised cron; passes through as-is.
	if got := s.parseSchedule("xs"); got != "xs" {
		t.Fatalf("expected 'xs' passed through unchanged, got %q", got)
	}
}

func TestParseSchedule_WhitespaceTrimmed(t *testing.T) {
	s := NewScheduler(testLogger(), NoopMetricsRecorder{})
	if got := s.parseSchedule("  30s  "); got != "*/30 * * * * *" {
		t.Fatalf("expected '*/30 * * * * *' after trimming whitespace, got %q", got)
	}
}

// ── executeHealthCheck ────────────────────────────────────────────────────────

func TestExecuteHealthCheck_HTTPSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := NewScheduler(testLogger(), NoopMetricsRecorder{})
	s.executeHealthCheck(models.HealthCheck{
		Name:    "test",
		Timeout: 5 * time.Second,
		HTTP:    &models.HTTPCheckConfig{URL: srv.URL, Method: "GET"},
	}, NewHTTPChecker(testLogger(), NoopMetricsRecorder{}))
	// No panic = pass; observable side-effects are tested via webhook tests below.
}

func TestExecuteHealthCheck_StarlarkSuccess(t *testing.T) {
	s := NewScheduler(testLogger(), NoopMetricsRecorder{})
	s.executeHealthCheck(models.HealthCheck{
		Name:    "test",
		Timeout: 5 * time.Second,
		Starlark: &models.StarlarkCheckConfig{
			Script: "healthy = True",
		},
	}, NewStarlarkChecker(testLogger(), NoopMetricsRecorder{}))
}

func TestExecuteHealthCheck_SuccessWebhookFired(t *testing.T) {
	var webhookCalled int32
	webhookSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&webhookCalled, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer webhookSrv.Close()

	checkSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer checkSrv.Close()

	s := NewScheduler(testLogger(), NoopMetricsRecorder{})
	s.executeHealthCheck(models.HealthCheck{
		Name:      "test",
		Timeout:   5 * time.Second,
		HTTP:      &models.HTTPCheckConfig{URL: checkSrv.URL, Method: "GET"},
		OnSuccess: []models.HookConfig{{HTTP: &models.WebhookConfig{URL: webhookSrv.URL}}},
	}, NewHTTPChecker(testLogger(), NoopMetricsRecorder{}))

	if n := atomic.LoadInt32(&webhookCalled); n != 1 {
		t.Fatalf("expected success webhook called once, got %d", n)
	}
}

func TestExecuteHealthCheck_FailureWebhookFired(t *testing.T) {
	var webhookCalled int32
	webhookSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&webhookCalled, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer webhookSrv.Close()

	checkSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer checkSrv.Close()

	s := NewScheduler(testLogger(), NoopMetricsRecorder{})
	s.executeHealthCheck(models.HealthCheck{
		Name:      "test",
		Timeout:   5 * time.Second,
		HTTP:      &models.HTTPCheckConfig{URL: checkSrv.URL, Method: "GET"},
		OnFailure: []models.HookConfig{{HTTP: &models.WebhookConfig{URL: webhookSrv.URL}}},
	}, NewHTTPChecker(testLogger(), NoopMetricsRecorder{}))

	if n := atomic.LoadInt32(&webhookCalled); n != 1 {
		t.Fatalf("expected failure webhook called once, got %d", n)
	}
}

func TestExecuteHealthCheck_SuccessWebhookNotFiredOnFailure(t *testing.T) {
	var webhookCalled int32
	webhookSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&webhookCalled, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer webhookSrv.Close()

	checkSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer checkSrv.Close()

	s := NewScheduler(testLogger(), NoopMetricsRecorder{})
	s.executeHealthCheck(models.HealthCheck{
		Name:      "test",
		Timeout:   5 * time.Second,
		HTTP:      &models.HTTPCheckConfig{URL: checkSrv.URL, Method: "GET"},
		OnSuccess: []models.HookConfig{{HTTP: &models.WebhookConfig{URL: webhookSrv.URL}}}, // only success hook
	}, NewHTTPChecker(testLogger(), NoopMetricsRecorder{}))

	if n := atomic.LoadInt32(&webhookCalled); n != 0 {
		t.Fatal("expected success webhook NOT fired when check fails")
	}
}

func TestExecuteHealthCheck_MultipleSuccessHooksAllFired(t *testing.T) {
	var firstCalled, secondCalled int32

	firstSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&firstCalled, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer firstSrv.Close()

	secondSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&secondCalled, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer secondSrv.Close()

	checkSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer checkSrv.Close()

	s := NewScheduler(testLogger(), NoopMetricsRecorder{})
	s.executeHealthCheck(models.HealthCheck{
		Name:    "test",
		Timeout: 5 * time.Second,
		HTTP:    &models.HTTPCheckConfig{URL: checkSrv.URL, Method: "GET"},
		OnSuccess: []models.HookConfig{
			{HTTP: &models.WebhookConfig{URL: firstSrv.URL}},
			{HTTP: &models.WebhookConfig{URL: secondSrv.URL}},
		},
	}, NewHTTPChecker(testLogger(), NoopMetricsRecorder{}))

	if n := atomic.LoadInt32(&firstCalled); n != 1 {
		t.Fatalf("expected first success hook called once, got %d", n)
	}
	if n := atomic.LoadInt32(&secondCalled); n != 1 {
		t.Fatalf("expected second success hook called once, got %d", n)
	}
}

func TestExecuteHealthCheck_MultipleFailureHooksAllFired(t *testing.T) {
	var firstCalled, secondCalled int32

	firstSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&firstCalled, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer firstSrv.Close()

	secondSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&secondCalled, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer secondSrv.Close()

	checkSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer checkSrv.Close()

	s := NewScheduler(testLogger(), NoopMetricsRecorder{})
	s.executeHealthCheck(models.HealthCheck{
		Name:    "test",
		Timeout: 5 * time.Second,
		HTTP:    &models.HTTPCheckConfig{URL: checkSrv.URL, Method: "GET"},
		OnFailure: []models.HookConfig{
			{HTTP: &models.WebhookConfig{URL: firstSrv.URL}},
			{HTTP: &models.WebhookConfig{URL: secondSrv.URL}},
		},
	}, NewHTTPChecker(testLogger(), NoopMetricsRecorder{}))

	if n := atomic.LoadInt32(&firstCalled); n != 1 {
		t.Fatalf("expected first failure hook called once, got %d", n)
	}
	if n := atomic.LoadInt32(&secondCalled); n != 1 {
		t.Fatalf("expected second failure hook called once, got %d", n)
	}
}

func TestExecuteHealthCheck_FailureWebhookNotFiredOnSuccess(t *testing.T) {
	var webhookCalled int32
	webhookSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&webhookCalled, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer webhookSrv.Close()

	checkSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer checkSrv.Close()

	s := NewScheduler(testLogger(), NoopMetricsRecorder{})
	s.executeHealthCheck(models.HealthCheck{
		Name:      "test",
		Timeout:   5 * time.Second,
		HTTP:      &models.HTTPCheckConfig{URL: checkSrv.URL, Method: "GET"},
		OnFailure: []models.HookConfig{{HTTP: &models.WebhookConfig{URL: webhookSrv.URL}}}, // only failure hook
	}, NewHTTPChecker(testLogger(), NoopMetricsRecorder{}))

	if n := atomic.LoadInt32(&webhookCalled); n != 0 {
		t.Fatal("expected failure webhook NOT fired when check succeeds")
	}
}
