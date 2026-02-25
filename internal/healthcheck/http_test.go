package healthcheck

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"go.starlark.net/starlark"

	"watchdawg/internal/models"
)

// makeHTTPCheck creates a minimal HealthCheck for HTTP tests.
func makeHTTPCheck(url, method string) *models.HealthCheck {
	return &models.HealthCheck{
		Name:    "test-http-check",
		Retries: 0,
		HTTP: &models.HTTPCheckConfig{
			URL:    url,
			Method: method,
		},
	}
}

// ── Basic HTTP checks ─────────────────────────────────────────────────────────

func TestHTTPChecker_BasicSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	checker := NewHTTPChecker()
	result := checker.Execute(context.Background(), makeHTTPCheck(srv.URL, "GET"))
	if !result.Healthy {
		t.Fatalf("expected healthy=true, error: %s", result.Error)
	}
}

func TestHTTPChecker_ConnectionRefused(t *testing.T) {
	checker := NewHTTPChecker()
	result := checker.Execute(context.Background(), makeHTTPCheck("http://127.0.0.1:1", "GET"))
	if result.Healthy {
		t.Fatal("expected healthy=false for connection refused")
	}
	if result.Error == "" {
		t.Fatal("expected non-empty error")
	}
}

func TestHTTPChecker_5xxWithoutExpectedCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	checker := NewHTTPChecker()
	result := checker.Execute(context.Background(), makeHTTPCheck(srv.URL, "GET"))
	if result.Healthy {
		t.Fatal("expected healthy=false for 500 response without explicit expected code")
	}
}

func TestHTTPChecker_3xxWithoutExpectedCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusFound)
	}))
	defer srv.Close()

	checker := NewHTTPChecker()
	result := checker.Execute(context.Background(), makeHTTPCheck(srv.URL, "GET"))
	if result.Healthy {
		t.Fatal("expected healthy=false for 302 response without explicit expected code")
	}
}

// ── Status code matching ──────────────────────────────────────────────────────

func TestHTTPChecker_ExpectedStatusCodeMatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	check := makeHTTPCheck(srv.URL, "GET")
	check.HTTP.Expected.StatusCode.Codes = []int{404}

	checker := NewHTTPChecker()
	result := checker.Execute(context.Background(), check)
	if !result.Healthy {
		t.Fatalf("expected healthy=true for explicitly expected 404, error: %s", result.Error)
	}
}

func TestHTTPChecker_ExpectedStatusCodeMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	check := makeHTTPCheck(srv.URL, "GET")
	check.HTTP.Expected.StatusCode.Codes = []int{201}

	checker := NewHTTPChecker()
	result := checker.Execute(context.Background(), check)
	if result.Healthy {
		t.Fatal("expected healthy=false when status code doesn't match expected")
	}
}

func TestHTTPChecker_MultipleAcceptableCodesMatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	check := makeHTTPCheck(srv.URL, "GET")
	check.HTTP.Expected.StatusCode.Codes = []int{200, 201, 204}

	checker := NewHTTPChecker()
	result := checker.Execute(context.Background(), check)
	if !result.Healthy {
		t.Fatalf("expected healthy=true for 201 in [200, 201, 204], error: %s", result.Error)
	}
}

func TestHTTPChecker_MultipleAcceptableCodesNoMatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	check := makeHTTPCheck(srv.URL, "GET")
	check.HTTP.Expected.StatusCode.Codes = []int{200, 201}

	checker := NewHTTPChecker()
	result := checker.Execute(context.Background(), check)
	if result.Healthy {
		t.Fatal("expected healthy=false for 400 not in [200, 201]")
	}
}

// ── Header checks ─────────────────────────────────────────────────────────────

func TestHTTPChecker_ExpectedHeaderMatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-App-Version", "1.0")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	check := makeHTTPCheck(srv.URL, "GET")
	check.HTTP.Expected.Headers = map[string]string{"X-App-Version": "1.0"}

	checker := NewHTTPChecker()
	result := checker.Execute(context.Background(), check)
	if !result.Healthy {
		t.Fatalf("expected healthy=true for matching header, error: %s", result.Error)
	}
}

func TestHTTPChecker_ExpectedHeaderMissing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	check := makeHTTPCheck(srv.URL, "GET")
	check.HTTP.Expected.Headers = map[string]string{"X-Required": "value"}

	checker := NewHTTPChecker()
	result := checker.Execute(context.Background(), check)
	if result.Healthy {
		t.Fatal("expected healthy=false for missing required header")
	}
}

func TestHTTPChecker_ExpectedHeaderWrongValue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-App-Version", "2.0")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	check := makeHTTPCheck(srv.URL, "GET")
	check.HTTP.Expected.Headers = map[string]string{"X-App-Version": "1.0"}

	checker := NewHTTPChecker()
	result := checker.Execute(context.Background(), check)
	if result.Healthy {
		t.Fatal("expected healthy=false for header value mismatch")
	}
}

// ── TLS ───────────────────────────────────────────────────────────────────────

func TestHTTPChecker_TLSSkipVerify(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	falseBool := false
	check := makeHTTPCheck(srv.URL, "GET")
	check.HTTP.Expected.VerifyTLS = &falseBool

	checker := NewHTTPChecker()
	result := checker.Execute(context.Background(), check)
	if !result.Healthy {
		t.Fatalf("expected healthy=true with TLS verify disabled, error: %s", result.Error)
	}
}

func TestHTTPChecker_TLSDefaultVerifyFails(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// Default client has TLS verification enabled; self-signed cert must fail.
	checker := NewHTTPChecker()
	result := checker.Execute(context.Background(), makeHTTPCheck(srv.URL, "GET"))
	if result.Healthy {
		t.Fatal("expected healthy=false for self-signed cert with default TLS verification")
	}
}

// ── Request body and custom headers ──────────────────────────────────────────

func TestHTTPChecker_CustomRequestHeaders(t *testing.T) {
	var gotHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("X-Custom")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	check := makeHTTPCheck(srv.URL, "GET")
	check.HTTP.Headers = map[string]string{"X-Custom": "myvalue"}

	NewHTTPChecker().Execute(context.Background(), check)
	if gotHeader != "myvalue" {
		t.Fatalf("expected header X-Custom=myvalue, got %q", gotHeader)
	}
}

func TestHTTPChecker_RequestBody(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		gotBody, err = io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	check := makeHTTPCheck(srv.URL, "POST")
	check.HTTP.Body = `{"key":"value"}`

	checker := NewHTTPChecker()
	result := checker.Execute(context.Background(), check)
	if !result.Healthy {
		t.Fatalf("expected healthy=true, error: %s", result.Error)
	}
	if string(gotBody) != `{"key":"value"}` {
		t.Fatalf("expected body %q, got %q", `{"key":"value"}`, gotBody)
	}
}

// ── Starlark assertions ───────────────────────────────────────────────────────

func TestHTTPChecker_StarlarkSimpleExpression(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	check := makeHTTPCheck(srv.URL, "GET")
	check.HTTP.Assertion = "status_code == 200"

	checker := NewHTTPChecker()
	result := checker.Execute(context.Background(), check)
	if !result.Healthy {
		t.Fatalf("expected healthy=true for 'status_code == 200', error: %s", result.Error)
	}
}

func TestHTTPChecker_StarlarkSimpleExpressionFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	check := makeHTTPCheck(srv.URL, "GET")
	check.HTTP.Assertion = "status_code == 201"

	checker := NewHTTPChecker()
	result := checker.Execute(context.Background(), check)
	if result.Healthy {
		t.Fatal("expected healthy=false when 'status_code == 201' is evaluated against 200 response")
	}
}

func TestHTTPChecker_StarlarkFullScript(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	check := makeHTTPCheck(srv.URL, "GET")
	check.HTTP.Assertion = "valid = status_code == 200\nmessage = \"status ok\""

	checker := NewHTTPChecker()
	result := checker.Execute(context.Background(), check)
	if !result.Healthy {
		t.Fatalf("expected healthy=true for full script, error: %s", result.Error)
	}
	if result.Message != "status ok" {
		t.Fatalf("expected message 'status ok', got %q", result.Message)
	}
}

func TestHTTPChecker_StarlarkScriptError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	check := makeHTTPCheck(srv.URL, "GET")
	check.HTTP.Assertion = `fail("intentional error")`

	checker := NewHTTPChecker()
	result := checker.Execute(context.Background(), check)
	if result.Healthy {
		t.Fatal("expected healthy=false for Starlark script error")
	}
	if result.Error == "" {
		t.Fatal("expected non-empty error")
	}
}

func TestHTTPChecker_StarlarkJSONFormat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok","count":42}`))
	}))
	defer srv.Close()

	check := makeHTTPCheck(srv.URL, "GET")
	check.HTTP.Expected.Format = models.ResponseFormatJSON
	check.HTTP.Assertion = `valid = result["status"] == "ok"`

	checker := NewHTTPChecker()
	result := checker.Execute(context.Background(), check)
	if !result.Healthy {
		t.Fatalf("expected healthy=true for JSON assertion, error: %s", result.Error)
	}
}

func TestHTTPChecker_StarlarkJSONFormatParseError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	check := makeHTTPCheck(srv.URL, "GET")
	check.HTTP.Expected.Format = models.ResponseFormatJSON
	check.HTTP.Assertion = `valid = True`

	checker := NewHTTPChecker()
	result := checker.Execute(context.Background(), check)
	if result.Healthy {
		t.Fatal("expected healthy=false for JSON parse error")
	}
	if result.Error == "" {
		t.Fatal("expected non-empty error")
	}
}

// ── Retries ───────────────────────────────────────────────────────────────────

// NOTE: incurs a ~1s sleep due to the inter-attempt delay.
func TestHTTPChecker_RetrySuccessOnSecondAttempt(t *testing.T) {
	var callCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&callCount, 1) < 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	check := makeHTTPCheck(srv.URL, "GET")
	check.Retries = 1

	checker := NewHTTPChecker()
	result := checker.Execute(context.Background(), check)
	if !result.Healthy {
		t.Fatalf("expected healthy=true after retry, error: %s", result.Error)
	}
	if result.Attempt != 2 {
		t.Fatalf("expected Attempt=2, got %d", result.Attempt)
	}
}

// NOTE: incurs a ~1s sleep due to the inter-attempt delay.
func TestHTTPChecker_RetryAllFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	check := makeHTTPCheck(srv.URL, "GET")
	check.Retries = 1

	checker := NewHTTPChecker()
	result := checker.Execute(context.Background(), check)
	if result.Healthy {
		t.Fatal("expected healthy=false after all retries fail")
	}
	if result.Attempt != 2 {
		t.Fatalf("expected Attempt=2 (last attempt), got %d", result.Attempt)
	}
}

// ── Result metadata ───────────────────────────────────────────────────────────

func TestHTTPChecker_ResultMetadata(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	checker := NewHTTPChecker()
	result := checker.Execute(context.Background(), makeHTTPCheck(srv.URL, "GET"))

	if result.CheckName != "test-http-check" {
		t.Fatalf("expected CheckName='test-http-check', got %q", result.CheckName)
	}
	if result.Duration < 0 {
		t.Fatalf("expected non-negative duration, got %d", result.Duration)
	}
	if result.Attempt != 1 {
		t.Fatalf("expected Attempt=1, got %d", result.Attempt)
	}
	if result.HTTPResult == nil {
		t.Fatal("expected non-nil HTTPResult")
	}
	if result.HTTPResult.StatusCode != 200 {
		t.Fatalf("expected StatusCode=200, got %d", result.HTTPResult.StatusCode)
	}
}

// ── isSimpleExpression ────────────────────────────────────────────────────────

func TestIsSimpleExpression_BooleanExpression(t *testing.T) {
	if !isSimpleExpression("status_code == 200") {
		t.Fatal("expected true for simple boolean expression")
	}
}

func TestIsSimpleExpression_CompoundExpression(t *testing.T) {
	if !isSimpleExpression(`status_code == 200 and "ok" in body`) {
		t.Fatal("expected true for compound expression without newlines or assignments")
	}
}

func TestIsSimpleExpression_WithValidAssignment(t *testing.T) {
	if isSimpleExpression("valid = status_code == 200") {
		t.Fatal("expected false for expression containing 'valid ='")
	}
}

func TestIsSimpleExpression_WithHealthyAssignment(t *testing.T) {
	if isSimpleExpression("healthy = True") {
		t.Fatal("expected false for expression containing 'healthy ='")
	}
}

func TestIsSimpleExpression_WithMessageAssignment(t *testing.T) {
	if isSimpleExpression("message = 'done'") {
		t.Fatal("expected false for expression containing 'message ='")
	}
}

func TestIsSimpleExpression_WithDef(t *testing.T) {
	if isSimpleExpression("def check(): return True") {
		t.Fatal("expected false for line containing 'def '")
	}
}

func TestIsSimpleExpression_WithNewline(t *testing.T) {
	if isSimpleExpression("valid = True\nmessage = 'ok'") {
		t.Fatal("expected false for multi-line script")
	}
}

// ── goToStarlark ──────────────────────────────────────────────────────────────

func TestGoToStarlark_Nil(t *testing.T) {
	if got := goToStarlark(nil); got != starlark.None {
		t.Fatalf("expected starlark.None for nil, got %v", got)
	}
}

func TestGoToStarlark_Bool(t *testing.T) {
	got := goToStarlark(true)
	b, ok := got.(starlark.Bool)
	if !ok || !bool(b) {
		t.Fatalf("expected starlark.Bool(true), got %v (%T)", got, got)
	}
}

func TestGoToStarlark_Int(t *testing.T) {
	got := goToStarlark(42)
	i, ok := got.(starlark.Int)
	if !ok {
		t.Fatalf("expected starlark.Int for int, got %T", got)
	}
	n, exact := i.Int64()
	if !exact || n != 42 {
		t.Fatalf("expected 42, got %v", got)
	}
}

func TestGoToStarlark_Int64(t *testing.T) {
	got := goToStarlark(int64(100))
	i, ok := got.(starlark.Int)
	if !ok {
		t.Fatalf("expected starlark.Int for int64, got %T", got)
	}
	n, exact := i.Int64()
	if !exact || n != 100 {
		t.Fatalf("expected 100, got %v", got)
	}
}

func TestGoToStarlark_Float64(t *testing.T) {
	got := goToStarlark(3.14)
	f, ok := got.(starlark.Float)
	if !ok || float64(f) != 3.14 {
		t.Fatalf("expected starlark.Float(3.14), got %v (%T)", got, got)
	}
}

func TestGoToStarlark_String(t *testing.T) {
	got := goToStarlark("hello")
	s, ok := got.(starlark.String)
	if !ok || string(s) != "hello" {
		t.Fatalf("expected starlark.String('hello'), got %v (%T)", got, got)
	}
}

func TestGoToStarlark_Slice(t *testing.T) {
	got := goToStarlark([]interface{}{"a", "b"})
	list, ok := got.(*starlark.List)
	if !ok {
		t.Fatalf("expected *starlark.List, got %T", got)
	}
	if list.Len() != 2 {
		t.Fatalf("expected list length 2, got %d", list.Len())
	}
}

func TestGoToStarlark_Map(t *testing.T) {
	got := goToStarlark(map[string]interface{}{"k": "v"})
	dict, ok := got.(*starlark.Dict)
	if !ok {
		t.Fatalf("expected *starlark.Dict, got %T", got)
	}
	val, found, _ := dict.Get(starlark.String("k"))
	if !found {
		t.Fatal("expected key 'k' in dict")
	}
	if string(val.(starlark.String)) != "v" {
		t.Fatalf("expected value 'v', got %v", val)
	}
}

func TestGoToStarlark_UnknownType(t *testing.T) {
	got := goToStarlark(struct{ Foo int }{Foo: 1})
	if _, ok := got.(starlark.String); !ok {
		t.Fatalf("expected starlark.String (via fmt.Sprintf) for unknown type, got %T", got)
	}
}

// ── parseResponseBody ─────────────────────────────────────────────────────────

func TestParseResponseBody_ValidJSON(t *testing.T) {
	val, err := parseResponseBody(`{"key":"value"}`, models.ResponseFormatJSON)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	dict, ok := val.(*starlark.Dict)
	if !ok {
		t.Fatalf("expected *starlark.Dict, got %T", val)
	}
	v, found, _ := dict.Get(starlark.String("key"))
	if !found {
		t.Fatal("expected key 'key' in dict")
	}
	if string(v.(starlark.String)) != "value" {
		t.Fatalf("expected 'value', got %v", v)
	}
}

func TestParseResponseBody_InvalidJSON(t *testing.T) {
	_, err := parseResponseBody(`not json`, models.ResponseFormatJSON)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestParseResponseBody_ValidXMLSucceeds(t *testing.T) {
	// xml.Unmarshal into interface{} succeeds for well-formed XML without error.
	_, err := parseResponseBody(`<root><item>value</item></root>`, models.ResponseFormatXML)
	if err != nil {
		t.Fatalf("expected no error for valid XML, got: %v", err)
	}
}

func TestParseResponseBody_InvalidXMLFails(t *testing.T) {
	_, err := parseResponseBody(`<unclosed`, models.ResponseFormatXML)
	if err == nil {
		t.Fatal("expected error for invalid XML")
	}
}

func TestParseResponseBody_NoneFormat(t *testing.T) {
	val, err := parseResponseBody(`anything`, models.ResponseFormatNone)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != starlark.None {
		t.Fatalf("expected starlark.None for no format, got %v", val)
	}
}

// ── hasValidationFields ───────────────────────────────────────────────────────

func TestHasValidationFields_WithValidKey(t *testing.T) {
	d := &starlark.Dict{}
	d.SetKey(starlark.String("valid"), starlark.True)
	if !hasValidationFields(d) {
		t.Fatal("expected true for dict with 'valid' key")
	}
}

func TestHasValidationFields_WithHealthyKey(t *testing.T) {
	d := &starlark.Dict{}
	d.SetKey(starlark.String("healthy"), starlark.True)
	if !hasValidationFields(d) {
		t.Fatal("expected true for dict with 'healthy' key")
	}
}

func TestHasValidationFields_Neither(t *testing.T) {
	d := &starlark.Dict{}
	d.SetKey(starlark.String("status"), starlark.String("ok"))
	if hasValidationFields(d) {
		t.Fatal("expected false for dict without 'valid' or 'healthy' keys")
	}
}

// ── parseValidationResult ─────────────────────────────────────────────────────

func TestParseValidationResult_WithValidField(t *testing.T) {
	d := &starlark.Dict{}
	d.SetKey(starlark.String("valid"), starlark.True)
	valid, msg, err := parseValidationResult(d)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !valid || msg != "" {
		t.Fatalf("expected valid=true, msg=''; got valid=%v, msg=%q", valid, msg)
	}
}

func TestParseValidationResult_WithHealthyField(t *testing.T) {
	d := &starlark.Dict{}
	d.SetKey(starlark.String("healthy"), starlark.False)
	valid, _, err := parseValidationResult(d)
	if err != nil || valid {
		t.Fatalf("expected valid=false, err=nil; got valid=%v, err=%v", valid, err)
	}
}

func TestParseValidationResult_WithMessage(t *testing.T) {
	d := &starlark.Dict{}
	d.SetKey(starlark.String("valid"), starlark.True)
	d.SetKey(starlark.String("message"), starlark.String("all ok"))
	_, msg, err := parseValidationResult(d)
	if err != nil || msg != "all ok" {
		t.Fatalf("expected msg='all ok', err=nil; got msg=%q, err=%v", msg, err)
	}
}

func TestParseValidationResult_NotDict(t *testing.T) {
	_, _, err := parseValidationResult(starlark.String("not a dict"))
	if err == nil {
		t.Fatal("expected error for non-dict input")
	}
}
