package starlarkeval

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.starlark.net/starlark"
)

const defaultMaxBytes = 10 * 1024 * 1024

// callHTTPRequest calls the http_request builtin with positional url and optional kwargs.
func callHTTPRequest(t *testing.T, ctx context.Context, client *http.Client, maxBytes int, url string, kwargs ...starlark.Tuple) *starlark.Dict {
	t.Helper()
	builtin := NewHTTPRequestBuiltin(ctx, client, maxBytes)
	thread := &starlark.Thread{Name: "test"}
	val, err := builtin.CallInternal(thread, starlark.Tuple{starlark.String(url)}, kwargs)
	if err != nil {
		t.Fatalf("http_request raised: %v", err)
	}
	d, ok := val.(*starlark.Dict)
	if !ok {
		t.Fatalf("expected dict, got %T", val)
	}
	return d
}

func dictString(t *testing.T, d *starlark.Dict, key string) string {
	t.Helper()
	v, found, err := d.Get(starlark.String(key))
	if err != nil || !found {
		t.Fatalf("key %q not found in dict", key)
	}
	s, ok := v.(starlark.String)
	if !ok {
		t.Fatalf("key %q: expected string, got %T (%v)", key, v, v)
	}
	return string(s)
}

func dictInt(t *testing.T, d *starlark.Dict, key string) int {
	t.Helper()
	v, found, err := d.Get(starlark.String(key))
	if err != nil || !found {
		t.Fatalf("key %q not found in dict", key)
	}
	i, ok := v.(starlark.Int)
	if !ok {
		t.Fatalf("key %q: expected int, got %T (%v)", key, v, v)
	}
	n, _ := i.Int64()
	return int(n)
}

func dictIsNone(t *testing.T, d *starlark.Dict, key string) bool {
	t.Helper()
	v, found, err := d.Get(starlark.String(key))
	if err != nil || !found {
		t.Fatalf("key %q not found in dict", key)
	}
	return v == starlark.None
}

// ── Success cases ─────────────────────────────────────────────────────────────

func TestHTTPRequestBuiltin_SuccessfulGET(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.Header().Set("X-Custom", "hello")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("pong"))
	}))
	defer srv.Close()

	d := callHTTPRequest(t, context.Background(), srv.Client(), defaultMaxBytes, srv.URL)

	if sc := dictInt(t, d, "status_code"); sc != 200 {
		t.Errorf("status_code: got %d, want 200", sc)
	}
	if body := dictString(t, d, "body"); body != "pong" {
		t.Errorf("body: got %q, want %q", body, "pong")
	}
	if !dictIsNone(t, d, "error") {
		t.Errorf("error should be None on success")
	}

	// Check response headers forwarded.
	headersVal, _, _ := d.Get(starlark.String("headers"))
	headersDict, ok := headersVal.(*starlark.Dict)
	if !ok {
		t.Fatalf("headers: expected dict")
	}
	xCustom, found, _ := headersDict.Get(starlark.String("X-Custom"))
	if !found {
		t.Fatal("X-Custom header not forwarded")
	}
	if string(xCustom.(starlark.String)) != "hello" {
		t.Errorf("X-Custom: got %v, want %q", xCustom, "hello")
	}
}

func TestHTTPRequestBuiltin_SuccessfulPOST(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type: got %q, want application/json", ct)
		}
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	kwargs := []starlark.Tuple{
		{starlark.String("method"), starlark.String("POST")},
		{starlark.String("body"), starlark.String(`{"x":1}`)},
		{starlark.String("headers"), func() *starlark.Dict {
			hd := starlark.NewDict(1)
			hd.SetKey(starlark.String("Content-Type"), starlark.String("application/json"))
			return hd
		}()},
	}
	d := callHTTPRequest(t, context.Background(), srv.Client(), defaultMaxBytes, srv.URL, kwargs...)

	if sc := dictInt(t, d, "status_code"); sc != 201 {
		t.Errorf("status_code: got %d, want 201", sc)
	}
	if !dictIsNone(t, d, "error") {
		t.Errorf("error should be None on success")
	}
}

func TestHTTPRequestBuiltin_Non2xxStatusReturnsNoError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("not found"))
	}))
	defer srv.Close()

	d := callHTTPRequest(t, context.Background(), srv.Client(), defaultMaxBytes, srv.URL)

	if sc := dictInt(t, d, "status_code"); sc != 404 {
		t.Errorf("status_code: got %d, want 404", sc)
	}
	// Non-2xx is not an error from the builtin's perspective.
	if !dictIsNone(t, d, "error") {
		t.Errorf("error should be None for non-2xx responses: got %v", func() starlark.Value {
			v, _, _ := d.Get(starlark.String("error"))
			return v
		}())
	}
}

func TestHTTPRequestBuiltin_MalformedURLSetsErrorField(t *testing.T) {
	d := callHTTPRequest(t, context.Background(), http.DefaultClient, defaultMaxBytes, "not-a-url")
	if sc := dictInt(t, d, "status_code"); sc != 0 {
		t.Errorf("status_code: got %d, want 0", sc)
	}
	if dictIsNone(t, d, "error") {
		t.Error("error should be set for malformed URL")
	}
}

func TestHTTPRequestBuiltin_UnsupportedSchemeSetsErrorField(t *testing.T) {
	d := callHTTPRequest(t, context.Background(), http.DefaultClient, defaultMaxBytes, "ftp://example.com/file")
	if sc := dictInt(t, d, "status_code"); sc != 0 {
		t.Errorf("status_code: got %d, want 0", sc)
	}
	if dictIsNone(t, d, "error") {
		t.Error("error should be set for unsupported scheme")
	}
	if errMsg := dictString(t, d, "error"); !strings.Contains(errMsg, "scheme") {
		t.Errorf("error should mention scheme, got %q", errMsg)
	}
}

// ── Truncation (T007) ─────────────────────────────────────────────────────────

func TestHTTPRequestBuiltin_BodyTruncation(t *testing.T) {
	const maxBytes = 10
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("0123456789OVERFLOW")) // 18 bytes > 10
	}))
	defer srv.Close()

	d := callHTTPRequest(t, context.Background(), srv.Client(), maxBytes, srv.URL)

	if sc := dictInt(t, d, "status_code"); sc != 200 {
		t.Errorf("status_code: got %d, want 200", sc)
	}
	body := dictString(t, d, "body")
	if len(body) != maxBytes {
		t.Errorf("body length: got %d, want %d", len(body), maxBytes)
	}
	if body != "0123456789" {
		t.Errorf("truncated body: got %q, want %q", body, "0123456789")
	}
	if dictIsNone(t, d, "error") {
		t.Error("error should be set when body is truncated")
	}
	errMsg := dictString(t, d, "error")
	if !strings.Contains(errMsg, "truncated") {
		t.Errorf("error should mention truncation, got %q", errMsg)
	}
}

// ── Timeout / context (T016, T017) ────────────────────────────────────────────

func TestHTTPRequestBuiltin_TimeoutCancelsRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate a slow server.
		select {
		case <-r.Context().Done():
		case <-time.After(500 * time.Millisecond):
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	d := callHTTPRequest(t, ctx, srv.Client(), defaultMaxBytes, srv.URL)
	elapsed := time.Since(start)

	if elapsed > 200*time.Millisecond {
		t.Errorf("request took %v, expected < 200ms", elapsed)
	}
	if sc := dictInt(t, d, "status_code"); sc != 0 {
		t.Errorf("status_code: got %d, want 0 on timeout", sc)
	}
	if dictIsNone(t, d, "error") {
		t.Error("error should be set on timeout")
	}
}

func TestHTTPRequestBuiltin_AlreadyCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before call

	d := callHTTPRequest(t, ctx, http.DefaultClient, defaultMaxBytes, "http://127.0.0.1:19999/never")

	if sc := dictInt(t, d, "status_code"); sc != 0 {
		t.Errorf("status_code: got %d, want 0 for cancelled context", sc)
	}
	if dictIsNone(t, d, "error") {
		t.Error("error should be set for cancelled context")
	}
}
