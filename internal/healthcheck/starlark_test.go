package healthcheck

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"watchdawg/internal/models"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// helper to build a minimal HealthCheck for Starlark tests
func makeCheck(script string) *models.HealthCheck {
	return &models.HealthCheck{
		Name:    "test-check",
		Retries: 0,
		Starlark: &models.StarlarkCheckConfig{
			Script: script,
		},
	}
}

func makeCheckWithGlobals(script string, globals map[string]interface{}) *models.HealthCheck {
	return &models.HealthCheck{
		Name:    "test-check",
		Retries: 0,
		Starlark: &models.StarlarkCheckConfig{
			Script:  script,
			Globals: globals,
		},
	}
}

// ── check() function returning a boolean ──────────────────────────────────────

func TestExecute_CheckFunctionReturnsBoolTrue(t *testing.T) {
	checker := NewStarlarkChecker(testLogger(), NoopMetricsRecorder{})
	result := checker.Execute(context.Background(), makeCheck(`
def check():
    return True
`))
	if !result.Healthy {
		t.Fatalf("expected healthy=true, got false (error: %s)", result.Error)
	}
}

func TestExecute_CheckFunctionReturnsBoolFalse(t *testing.T) {
	checker := NewStarlarkChecker(testLogger(), NoopMetricsRecorder{})
	result := checker.Execute(context.Background(), makeCheck(`
def check():
    return False
`))
	if result.Healthy {
		t.Fatal("expected healthy=false, got true")
	}
}

// ── check() function returning a dict ─────────────────────────────────────────

func TestExecute_CheckFunctionReturnsDictHealthy(t *testing.T) {
	checker := NewStarlarkChecker(testLogger(), NoopMetricsRecorder{})
	result := checker.Execute(context.Background(), makeCheck(`
def check():
    return {"healthy": True, "message": "all good"}
`))
	if !result.Healthy {
		t.Fatalf("expected healthy=true, error: %s", result.Error)
	}
	if result.Message != "all good" {
		t.Fatalf("expected message 'all good', got %q", result.Message)
	}
}

func TestExecute_CheckFunctionReturnsDictUnhealthy(t *testing.T) {
	checker := NewStarlarkChecker(testLogger(), NoopMetricsRecorder{})
	result := checker.Execute(context.Background(), makeCheck(`
def check():
    return {"healthy": False, "message": "something broke"}
`))
	if result.Healthy {
		t.Fatal("expected healthy=false, got true")
	}
	if result.Message != "something broke" {
		t.Fatalf("expected message 'something broke', got %q", result.Message)
	}
}

func TestExecute_CheckFunctionReturnsDictWithoutMessage(t *testing.T) {
	checker := NewStarlarkChecker(testLogger(), NoopMetricsRecorder{})
	result := checker.Execute(context.Background(), makeCheck(`
def check():
    return {"healthy": True}
`))
	if !result.Healthy {
		t.Fatalf("expected healthy=true, error: %s", result.Error)
	}
}

// ── check() function error cases ──────────────────────────────────────────────

func TestExecute_CheckFunctionMissingHealthyField(t *testing.T) {
	checker := NewStarlarkChecker(testLogger(), NoopMetricsRecorder{})
	result := checker.Execute(context.Background(), makeCheck(`
def check():
    return {"status": "ok"}
`))
	if result.Healthy {
		t.Fatal("expected healthy=false when 'healthy' field is missing")
	}
	if !strings.Contains(result.Error, "healthy") {
		t.Fatalf("expected error mentioning 'healthy' field, got: %s", result.Error)
	}
}

func TestExecute_CheckFunctionHealthyFieldNotBool(t *testing.T) {
	checker := NewStarlarkChecker(testLogger(), NoopMetricsRecorder{})
	result := checker.Execute(context.Background(), makeCheck(`
def check():
    return {"healthy": "yes"}
`))
	if result.Healthy {
		t.Fatal("expected healthy=false when 'healthy' is not a bool")
	}
	if result.Error == "" {
		t.Fatal("expected a non-empty error")
	}
}

func TestExecute_CheckFunctionReturnsWrongType(t *testing.T) {
	checker := NewStarlarkChecker(testLogger(), NoopMetricsRecorder{})
	result := checker.Execute(context.Background(), makeCheck(`
def check():
    return 42
`))
	if result.Healthy {
		t.Fatal("expected healthy=false when check() returns an int")
	}
	if result.Error == "" {
		t.Fatal("expected a non-empty error")
	}
}

func TestExecute_CheckIsNotCallable(t *testing.T) {
	checker := NewStarlarkChecker(testLogger(), NoopMetricsRecorder{})
	result := checker.Execute(context.Background(), makeCheck(`
check = "not a function"
`))
	if result.Healthy {
		t.Fatal("expected healthy=false when 'check' is not callable")
	}
	if !strings.Contains(result.Error, "not a function") {
		t.Fatalf("expected error about 'check' not being a function, got: %s", result.Error)
	}
}

func TestExecute_CheckFunctionRaisesError(t *testing.T) {
	checker := NewStarlarkChecker(testLogger(), NoopMetricsRecorder{})
	result := checker.Execute(context.Background(), makeCheck(`
def check():
    fail("intentional failure")
`))
	if result.Healthy {
		t.Fatal("expected healthy=false when check() raises an error")
	}
	if result.Error == "" {
		t.Fatal("expected a non-empty error")
	}
}

// ── script errors ─────────────────────────────────────────────────────────────

func TestExecute_ScriptSyntaxError(t *testing.T) {
	checker := NewStarlarkChecker(testLogger(), NoopMetricsRecorder{})
	result := checker.Execute(context.Background(), makeCheck(`
def check(
    # intentionally broken syntax
`))
	if result.Healthy {
		t.Fatal("expected healthy=false for syntax error")
	}
	if result.Error == "" {
		t.Fatal("expected a non-empty error for syntax error")
	}
}

func TestExecute_ScriptRuntimeError(t *testing.T) {
	checker := NewStarlarkChecker(testLogger(), NoopMetricsRecorder{})
	// Accessing an undefined variable triggers a runtime error
	result := checker.Execute(context.Background(), makeCheck(`
x = undefined_variable
`))
	if result.Healthy {
		t.Fatal("expected healthy=false for runtime error")
	}
	if result.Error == "" {
		t.Fatal("expected a non-empty error for runtime error")
	}
}

// ── fallback: global variable mode ───────────────────────────────────────────

func TestExecute_GlobalResultDict(t *testing.T) {
	checker := NewStarlarkChecker(testLogger(), NoopMetricsRecorder{})
	result := checker.Execute(context.Background(), makeCheck(`
result = {"healthy": True, "message": "via global result"}
`))
	if !result.Healthy {
		t.Fatalf("expected healthy=true, error: %s", result.Error)
	}
	if result.Message != "via global result" {
		t.Fatalf("expected message 'via global result', got %q", result.Message)
	}
}

func TestExecute_GlobalResultDictUnhealthy(t *testing.T) {
	checker := NewStarlarkChecker(testLogger(), NoopMetricsRecorder{})
	result := checker.Execute(context.Background(), makeCheck(`
result = {"healthy": False, "message": "unhealthy via global"}
`))
	if result.Healthy {
		t.Fatal("expected healthy=false")
	}
	if result.Message != "unhealthy via global" {
		t.Fatalf("expected 'unhealthy via global', got %q", result.Message)
	}
}

func TestExecute_GlobalHealthyAndMessageVars(t *testing.T) {
	checker := NewStarlarkChecker(testLogger(), NoopMetricsRecorder{})
	result := checker.Execute(context.Background(), makeCheck(`
healthy = False
message = "explicit globals"
`))
	if result.Healthy {
		t.Fatal("expected healthy=false")
	}
	if result.Message != "explicit globals" {
		t.Fatalf("expected 'explicit globals', got %q", result.Message)
	}
}

func TestExecute_NoExplicitResult_DefaultsToSuccess(t *testing.T) {
	checker := NewStarlarkChecker(testLogger(), NoopMetricsRecorder{})
	// An empty script with no check() function and no result/healthy/message
	// globals should default to healthy=true.
	result := checker.Execute(context.Background(), makeCheck(`x = 1 + 1`))
	if !result.Healthy {
		t.Fatalf("expected healthy=true for script with no explicit result, error: %s", result.Error)
	}
}

// ── global variable injection ─────────────────────────────────────────────────

func TestExecute_GlobalsInjectedIntoScript(t *testing.T) {
	checker := NewStarlarkChecker(testLogger(), NoopMetricsRecorder{})
	check := makeCheckWithGlobals(`
def check():
    if threshold > 90:
        return {"healthy": False, "message": "over threshold"}
    return {"healthy": True, "message": "ok"}
`, map[string]interface{}{"threshold": 95})

	result := checker.Execute(context.Background(), check)
	if result.Healthy {
		t.Fatal("expected healthy=false when threshold=95")
	}
	if result.Message != "over threshold" {
		t.Fatalf("expected 'over threshold', got %q", result.Message)
	}
}

func TestExecute_StringGlobal(t *testing.T) {
	checker := NewStarlarkChecker(testLogger(), NoopMetricsRecorder{})
	check := makeCheckWithGlobals(`
def check():
    return {"healthy": True, "message": greeting}
`, map[string]interface{}{"greeting": "hello"})

	result := checker.Execute(context.Background(), check)
	if !result.Healthy {
		t.Fatalf("expected healthy=true, error: %s", result.Error)
	}
	if result.Message != "hello" {
		t.Fatalf("expected 'hello', got %q", result.Message)
	}
}

func TestExecute_BoolGlobal(t *testing.T) {
	checker := NewStarlarkChecker(testLogger(), NoopMetricsRecorder{})
	check := makeCheckWithGlobals(`
def check():
    return enabled
`, map[string]interface{}{"enabled": true})

	result := checker.Execute(context.Background(), check)
	if !result.Healthy {
		t.Fatalf("expected healthy=true when enabled=true, error: %s", result.Error)
	}
}

func TestExecute_MapGlobal(t *testing.T) {
	checker := NewStarlarkChecker(testLogger(), NoopMetricsRecorder{})
	check := makeCheckWithGlobals(`
def check():
    return cfg["ok"]
`, map[string]interface{}{"cfg": map[string]interface{}{"ok": true}})

	result := checker.Execute(context.Background(), check)
	if !result.Healthy {
		t.Fatalf("expected healthy=true, error: %s", result.Error)
	}
}

func TestExecute_ListGlobal(t *testing.T) {
	checker := NewStarlarkChecker(testLogger(), NoopMetricsRecorder{})
	check := makeCheckWithGlobals(`
def check():
    return len(items) > 0
`, map[string]interface{}{"items": []interface{}{"a", "b"}})

	result := checker.Execute(context.Background(), check)
	if !result.Healthy {
		t.Fatalf("expected healthy=true, error: %s", result.Error)
	}
}

// ── toStarlarkValue ──────────────────────────────────────────────────────────

func TestToStarlarkValue_UnknownType(t *testing.T) {
	checker := NewStarlarkChecker(testLogger(), NoopMetricsRecorder{})
	// An unrecognised type should produce starlark.None, which scripts treat as falsy.
	check := makeCheckWithGlobals(`
def check():
    if val == None:
        return True
    return False
`, map[string]interface{}{"val": struct{}{}})

	result := checker.Execute(context.Background(), check)
	if !result.Healthy {
		t.Fatalf("expected healthy=true for None-mapped unknown type, error: %s", result.Error)
	}
}

// ── result metadata ───────────────────────────────────────────────────────────

func TestExecute_SetsCheckName(t *testing.T) {
	checker := NewStarlarkChecker(testLogger(), NoopMetricsRecorder{})
	result := checker.Execute(context.Background(), makeCheck(`healthy = True`))
	if result.CheckName != "test-check" {
		t.Fatalf("expected CheckName 'test-check', got %q", result.CheckName)
	}
}

func TestExecute_SetsDurationMs(t *testing.T) {
	checker := NewStarlarkChecker(testLogger(), NoopMetricsRecorder{})
	result := checker.Execute(context.Background(), makeCheck(`healthy = True`))
	if result.Duration < 0 {
		t.Fatalf("expected non-negative duration, got %d", result.Duration)
	}
}

func TestExecute_SetsAttemptNumber(t *testing.T) {
	checker := NewStarlarkChecker(testLogger(), NoopMetricsRecorder{})
	result := checker.Execute(context.Background(), makeCheck(`healthy = True`))
	if result.Attempt != 1 {
		t.Fatalf("expected Attempt=1, got %d", result.Attempt)
	}
}

// ── retries ───────────────────────────────────────────────────────────────────

// NOTE: this test incurs a ~1 s sleep (the inter-attempt delay in Execute).
func TestExecute_ReturnsLastAttemptOnAllFailures(t *testing.T) {
	checker := NewStarlarkChecker(testLogger(), NoopMetricsRecorder{})
	check := &models.HealthCheck{
		Name:    "retry-check",
		Retries: 1, // 2 total attempts
		Starlark: &models.StarlarkCheckConfig{
			Script: `healthy = False
message = "always failing"`,
		},
	}

	result := checker.Execute(context.Background(), check)
	if result.Healthy {
		t.Fatal("expected healthy=false after exhausted retries")
	}
	if result.Attempt != 2 {
		t.Fatalf("expected Attempt=2 (last attempt), got %d", result.Attempt)
	}
}
