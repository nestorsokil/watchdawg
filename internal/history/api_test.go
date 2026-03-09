package history

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"watchdawg/internal/models"
)

// stubStore implements ExecutionStore for API handler tests without a real DB.
type stubStore struct {
	checkData map[string][]Record
	err       error
}

func (s *stubStore) Write(_ context.Context, _ *models.HealthCheck, _ *models.CheckResult, _ int) error {
	return s.err
}

func (s *stubStore) QueryCheck(_ context.Context, checkName string, limit int) ([]Record, error) {
	if s.err != nil {
		return nil, s.err
	}
	records, ok := s.checkData[checkName]
	if !ok || len(records) == 0 {
		return nil, fmt.Errorf("%w %q", ErrNotFound, checkName) //nolint:goerr113
	}
	if limit < len(records) {
		records = records[:limit]
	}
	return records, nil
}

func (s *stubStore) QueryAll(_ context.Context, limit int) (map[string][]Record, error) {
	if s.err != nil {
		return nil, s.err
	}
	result := make(map[string][]Record)
	for name, records := range s.checkData {
		if limit < len(records) {
			records = records[:limit]
		}
		result[name] = records
	}
	return result, nil
}

func (s *stubStore) Close() error { return nil }

func makeRecords(n int) []Record {
	base := time.Date(2026, 3, 9, 10, 0, 0, 0, time.UTC)
	records := make([]Record, n)
	for i := 0; i < n; i++ {
		records[i] = Record{
			ID:         "uuid-" + string(rune('a'+i)),
			Timestamp:  base.Add(time.Duration(n-i-1) * time.Second), // newest first
			Healthy:    true,
			DurationMs: int64(i * 10),
			Error:      "",
		}
	}
	return records
}

func newHandler(store ExecutionStore) *Handler {
	return NewHandler(store, slog.Default())
}

func TestHandlerGetCheck_200(t *testing.T) {
	store := &stubStore{checkData: map[string][]Record{
		"api-health": makeRecords(3),
	}}
	h := newHandler(store)
	
	

	req := httptest.NewRequest("GET", "/history/api-health", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp checksResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Checks["api-health"]) != 3 {
		t.Errorf("expected 3 records, got %d", len(resp.Checks["api-health"]))
	}
}

func TestHandlerGetCheck_404NotFound(t *testing.T) {
	store := &stubStore{checkData: map[string][]Record{}}
	h := newHandler(store)
	
	

	req := httptest.NewRequest("GET", "/history/missing-check", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
	var resp errorResponse
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp.Error == "" {
		t.Error("expected non-empty error message in 404 response")
	}
}

func TestHandlerGetCheck_400InvalidLimit(t *testing.T) {
	store := &stubStore{checkData: map[string][]Record{}}
	h := newHandler(store)
	
	

	req := httptest.NewRequest("GET", "/history/any?limit=0", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestHandlerGetCheck_LimitApplied(t *testing.T) {
	store := &stubStore{checkData: map[string][]Record{
		"svc": makeRecords(10),
	}}
	h := newHandler(store)
	
	

	req := httptest.NewRequest("GET", "/history/svc?limit=3", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp checksResponse
	json.NewDecoder(rr.Body).Decode(&resp)
	if len(resp.Checks["svc"]) != 3 {
		t.Errorf("expected 3 records with limit=3, got %d", len(resp.Checks["svc"]))
	}
}

func TestHandlerGetAll_200EmptyMap(t *testing.T) {
	store := &stubStore{checkData: map[string][]Record{}}
	h := newHandler(store)
	
	

	req := httptest.NewRequest("GET", "/history/*", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp checksResponse
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp.Checks == nil {
		t.Error("expected non-nil checks map")
	}
	if len(resp.Checks) != 0 {
		t.Errorf("expected empty checks map, got %d entries", len(resp.Checks))
	}
}

func TestHandlerGetAll_200WithRecords(t *testing.T) {
	store := &stubStore{checkData: map[string][]Record{
		"alpha": makeRecords(2),
		"beta":  makeRecords(3),
	}}
	h := newHandler(store)
	
	

	req := httptest.NewRequest("GET", "/history/*", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp checksResponse
	json.NewDecoder(rr.Body).Decode(&resp)
	if len(resp.Checks) != 2 {
		t.Errorf("expected 2 checks, got %d", len(resp.Checks))
	}
}

func TestHandlerGetAll_DefaultLimitApplied(t *testing.T) {
	records := makeRecords(150)
	store := &stubStore{checkData: map[string][]Record{"big": records}}
	h := newHandler(store)
	
	

	req := httptest.NewRequest("GET", "/history/*", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var resp checksResponse
	json.NewDecoder(rr.Body).Decode(&resp)
	if len(resp.Checks["big"]) != defaultLimit {
		t.Errorf("expected default limit %d, got %d", defaultLimit, len(resp.Checks["big"]))
	}
}
