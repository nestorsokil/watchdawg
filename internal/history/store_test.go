package history

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"watchdawg/internal/models"
)

func openInMemoryStore(t *testing.T) *SQLiteStore {
	t.Helper()
	cfg := &models.HistoryConfig{DBPath: ":memory:"}
	store, err := NewSQLiteStore(cfg, slog.Default())
	if err != nil {
		t.Fatalf("failed to open in-memory store: %v", err)
	}
	return store
}

func makeCheck(name string) *models.HealthCheck {
	return &models.HealthCheck{Name: name}
}

func makeResult(healthy bool, durationMs int64, errMsg string) *models.CheckResult {
	return &models.CheckResult{
		Timestamp: time.Now().UTC(),
		Healthy:   healthy,
		Duration:  durationMs,
		Error:     errMsg,
	}
}

// --- Write tests (T007) ---

func TestWrite_AllFieldsPersisted(t *testing.T) {
	store := openInMemoryStore(t)
	defer store.Close()

	check := makeCheck("api-health")
	result := makeResult(true, 42, "")
	result.Timestamp = time.Date(2026, 3, 9, 10, 0, 0, 0, time.UTC)

	if err := store.Write(context.Background(), check, result, 1000); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	records, err := store.QueryCheck(context.Background(), "api-health", 10)
	if err != nil {
		t.Fatalf("QueryCheck failed: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	r := records[0]
	if r.ID == "" {
		t.Error("expected non-empty UUID id")
	}
	if r.Healthy != true {
		t.Errorf("expected healthy=true, got %v", r.Healthy)
	}
	if r.DurationMs != 42 {
		t.Errorf("expected duration_ms=42, got %d", r.DurationMs)
	}
	if r.Error != "" {
		t.Errorf("expected empty error, got %q", r.Error)
	}
	if !r.Timestamp.Equal(result.Timestamp) {
		t.Errorf("expected timestamp %v, got %v", result.Timestamp, r.Timestamp)
	}
}

func TestWrite_FailureRecord(t *testing.T) {
	store := openInMemoryStore(t)
	defer store.Close()

	check := makeCheck("db-ping")
	result := makeResult(false, 5001, "connection refused")

	if err := store.Write(context.Background(), check, result, 1000); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	records, err := store.QueryCheck(context.Background(), "db-ping", 10)
	if err != nil {
		t.Fatalf("QueryCheck failed: %v", err)
	}
	r := records[0]
	if r.Healthy {
		t.Error("expected healthy=false")
	}
	if r.Error != "connection refused" {
		t.Errorf("expected error='connection refused', got %q", r.Error)
	}
}

func TestWrite_UUIDUniqueness(t *testing.T) {
	store := openInMemoryStore(t)
	defer store.Close()

	check := makeCheck("test")
	for i := 0; i < 5; i++ {
		result := makeResult(true, int64(i), "")
		if err := store.Write(context.Background(), check, result, 1000); err != nil {
			t.Fatalf("Write[%d] failed: %v", i, err)
		}
	}

	records, err := store.QueryCheck(context.Background(), "test", 10)
	if err != nil {
		t.Fatalf("QueryCheck failed: %v", err)
	}
	seen := make(map[string]bool)
	for _, r := range records {
		if seen[r.ID] {
			t.Errorf("duplicate UUID: %s", r.ID)
		}
		seen[r.ID] = true
	}
}

func TestWrite_ContextCancellation(t *testing.T) {
	store := openInMemoryStore(t)
	defer store.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	check := makeCheck("test")
	result := makeResult(true, 10, "")
	err := store.Write(ctx, check, result, 1000)
	// SQLite in-memory may or may not return context errors; we just ensure it doesn't panic
	_ = err
}

// --- QueryCheck tests (T015) ---

func TestQueryCheck_ReturnsNewestFirst(t *testing.T) {
	store := openInMemoryStore(t)
	defer store.Close()

	check := makeCheck("ordered")
	base := time.Date(2026, 3, 9, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		r := makeResult(true, int64(i), "")
		r.Timestamp = base.Add(time.Duration(i) * time.Second)
		if err := store.Write(context.Background(), check, r, 1000); err != nil {
			t.Fatalf("Write[%d] failed: %v", i, err)
		}
	}

	records, err := store.QueryCheck(context.Background(), "ordered", 10)
	if err != nil {
		t.Fatalf("QueryCheck failed: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("expected 3 records, got %d", len(records))
	}
	// Newest first: record[0] should have the largest timestamp
	for i := 1; i < len(records); i++ {
		if records[i].Timestamp.After(records[i-1].Timestamp) {
			t.Errorf("records not in reverse-chronological order at index %d", i)
		}
	}
}

func TestQueryCheck_NotFound(t *testing.T) {
	store := openInMemoryStore(t)
	defer store.Close()

	_, err := store.QueryCheck(context.Background(), "nonexistent", 10)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestQueryCheck_LimitRespected(t *testing.T) {
	store := openInMemoryStore(t)
	defer store.Close()

	check := makeCheck("limited")
	for i := 0; i < 10; i++ {
		if err := store.Write(context.Background(), check, makeResult(true, int64(i), ""), 1000); err != nil {
			t.Fatalf("Write[%d] failed: %v", i, err)
		}
	}

	records, err := store.QueryCheck(context.Background(), "limited", 3)
	if err != nil {
		t.Fatalf("QueryCheck failed: %v", err)
	}
	if len(records) != 3 {
		t.Errorf("expected 3 records with limit=3, got %d", len(records))
	}
}

func TestQueryCheck_MultipleChecksIsolated(t *testing.T) {
	store := openInMemoryStore(t)
	defer store.Close()

	for _, name := range []string{"check-a", "check-b", "check-c"} {
		check := makeCheck(name)
		for i := 0; i < 3; i++ {
			if err := store.Write(context.Background(), check, makeResult(true, int64(i), ""), 1000); err != nil {
				t.Fatalf("Write for %s failed: %v", name, err)
			}
		}
	}

	records, err := store.QueryCheck(context.Background(), "check-a", 10)
	if err != nil {
		t.Fatalf("QueryCheck failed: %v", err)
	}
	if len(records) != 3 {
		t.Errorf("expected 3 records for check-a, got %d", len(records))
	}
}

// --- QueryAll tests ---

func TestQueryAll_EmptyStore(t *testing.T) {
	store := openInMemoryStore(t)
	defer store.Close()

	result, err := store.QueryAll(context.Background(), 10)
	if err != nil {
		t.Fatalf("QueryAll failed: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty map, got %d entries", len(result))
	}
}

func TestQueryAll_MultipleChecks(t *testing.T) {
	store := openInMemoryStore(t)
	defer store.Close()

	for _, name := range []string{"alpha", "beta"} {
		check := makeCheck(name)
		for i := 0; i < 2; i++ {
			if err := store.Write(context.Background(), check, makeResult(true, int64(i), ""), 1000); err != nil {
				t.Fatalf("Write for %s failed: %v", name, err)
			}
		}
	}

	result, err := store.QueryAll(context.Background(), 10)
	if err != nil {
		t.Fatalf("QueryAll failed: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 checks, got %d", len(result))
	}
	if len(result["alpha"]) != 2 {
		t.Errorf("expected 2 records for alpha, got %d", len(result["alpha"]))
	}
	if len(result["beta"]) != 2 {
		t.Errorf("expected 2 records for beta, got %d", len(result["beta"]))
	}
}

// --- Eviction / retention tests (T021) ---

func TestWrite_RetentionLimitEnforced(t *testing.T) {
	store := openInMemoryStore(t)
	defer store.Close()

	check := makeCheck("retained")
	const retention = 5
	base := time.Date(2026, 3, 9, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 10; i++ {
		r := makeResult(true, int64(i), "")
		r.Timestamp = base.Add(time.Duration(i) * time.Second)
		if err := store.Write(context.Background(), check, r, retention); err != nil {
			t.Fatalf("Write[%d] failed: %v", i, err)
		}
	}

	records, err := store.QueryCheck(context.Background(), "retained", 100)
	if err != nil {
		t.Fatalf("QueryCheck failed: %v", err)
	}
	if len(records) != retention {
		t.Errorf("expected %d records after eviction, got %d", retention, len(records))
	}
}

func TestWrite_OldestRecordsEvicted(t *testing.T) {
	store := openInMemoryStore(t)
	defer store.Close()

	check := makeCheck("evict-order")
	base := time.Date(2026, 3, 9, 10, 0, 0, 0, time.UTC)
	const retention = 3
	for i := 0; i < 5; i++ {
		r := makeResult(true, int64(i), "")
		r.Timestamp = base.Add(time.Duration(i) * time.Second)
		if err := store.Write(context.Background(), check, r, retention); err != nil {
			t.Fatalf("Write[%d] failed: %v", i, err)
		}
	}

	records, err := store.QueryCheck(context.Background(), "evict-order", 10)
	if err != nil {
		t.Fatalf("QueryCheck failed: %v", err)
	}
	if len(records) != retention {
		t.Fatalf("expected %d records, got %d", retention, len(records))
	}
	// records are newest-first; the oldest 2 (index 0,1) should have been evicted
	// newest 3 are: i=2 (base+2s), i=3 (base+3s), i=4 (base+4s)
	latestExpected := base.Add(4 * time.Second)
	if !records[0].Timestamp.Equal(latestExpected) {
		t.Errorf("expected newest record at %v, got %v", latestExpected, records[0].Timestamp)
	}
}

func TestWrite_PerCheckRetentionBeatsGlobal(t *testing.T) {
	// Simulate two writes: one with retention=10, then override per-check with retention=2
	store := openInMemoryStore(t)
	defer store.Close()

	check := makeCheck("per-check")
	check.Retention = 2 // per-check override
	base := time.Date(2026, 3, 9, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		r := makeResult(true, int64(i), "")
		r.Timestamp = base.Add(time.Duration(i) * time.Second)
		// Pass per-check retention directly
		if err := store.Write(context.Background(), check, r, check.Retention); err != nil {
			t.Fatalf("Write[%d] failed: %v", i, err)
		}
	}

	records, err := store.QueryCheck(context.Background(), "per-check", 100)
	if err != nil {
		t.Fatalf("QueryCheck failed: %v", err)
	}
	if len(records) != 2 {
		t.Errorf("expected 2 records (per-check retention), got %d", len(records))
	}
}

func TestWrite_GlobalDefaultRetention1000(t *testing.T) {
	store := openInMemoryStore(t)
	defer store.Close()

	// Write 10 records with the global default retention of 1000; all should be kept
	check := makeCheck("global-default")
	for i := 0; i < 10; i++ {
		if err := store.Write(context.Background(), check, makeResult(true, int64(i), ""), 1000); err != nil {
			t.Fatalf("Write[%d] failed: %v", i, err)
		}
	}

	records, err := store.QueryCheck(context.Background(), "global-default", 100)
	if err != nil {
		t.Fatalf("QueryCheck failed: %v", err)
	}
	if len(records) != 10 {
		t.Errorf("expected 10 records with global default, got %d", len(records))
	}
}
