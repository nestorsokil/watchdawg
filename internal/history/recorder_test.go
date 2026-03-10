package history

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"watchdawg/internal/models"
)

// fakeStore is a test double for ExecutionStore.
type fakeStore struct {
	written []recordJob
	writeErr error
	closeCalled bool
}

func (f *fakeStore) Write(_ context.Context, check *models.HealthCheck, result *models.CheckResult, retention int) error {
	if f.writeErr != nil {
		return f.writeErr
	}
	f.written = append(f.written, recordJob{check: check, result: result, retention: retention})
	return nil
}

func (f *fakeStore) QueryCheck(_ context.Context, _ string, _ int) ([]Record, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeStore) QueryAll(_ context.Context, _ int) (map[string][]Record, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeStore) Close() error {
	f.closeCalled = true
	return nil
}

func startRecorder(store ExecutionStore) (*Recorder, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	r := NewRecorder(store, 1000, slog.Default())
	go r.Start(ctx)
	return r, cancel
}

func TestRecorder_RecordDispatchesToStore(t *testing.T) {
	store := &fakeStore{}
	r, cancel := startRecorder(store)

	check := makeCheck("api")
	result := makeResult(true, 10, "")
	r.Record(check, result)

	cancel()
	r.Stop()

	if len(store.written) != 1 {
		t.Fatalf("expected 1 written record, got %d", len(store.written))
	}
	if store.written[0].check.Name != "api" {
		t.Errorf("unexpected check name: %s", store.written[0].check.Name)
	}
}

func TestRecorder_GlobalRetentionUsedWhenCheckRetentionZero(t *testing.T) {
	store := &fakeStore{}
	r, cancel := startRecorder(store)

	check := makeCheck("test")
	check.Retention = 0 // should fall back to globalRetention=1000
	r.Record(check, makeResult(true, 5, ""))

	cancel()
	r.Stop()

	if len(store.written) == 0 {
		t.Fatal("no records written")
	}
	if store.written[0].retention != 1000 {
		t.Errorf("expected retention=1000 (global), got %d", store.written[0].retention)
	}
}

func TestRecorder_PerCheckRetentionOverridesGlobal(t *testing.T) {
	store := &fakeStore{}
	r, cancel := startRecorder(store)

	check := makeCheck("test")
	check.Retention = 250
	r.Record(check, makeResult(true, 5, ""))

	cancel()
	r.Stop()

	if len(store.written) == 0 {
		t.Fatal("no records written")
	}
	if store.written[0].retention != 250 {
		t.Errorf("expected retention=250 (per-check), got %d", store.written[0].retention)
	}
}

func TestRecorder_DropOnFull(t *testing.T) {
	store := &fakeStore{}
	// Create recorder without starting consumer — channel fills up
	r := NewRecorder(store, 1000, slog.Default())

	check := makeCheck("flood")
	result := makeResult(true, 1, "")

	// Fill past the buffer capacity
	dropped := false
	for i := 0; i <= channelBufferSize+10; i++ {
		before := len(r.ch)
		r.Record(check, result)
		after := len(r.ch)
		if after == before && i >= channelBufferSize {
			dropped = true
			break
		}
	}
	if !dropped {
		t.Error("expected at least one record to be dropped when channel is full")
	}
	// Drain to allow Stop to close cleanly
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		r.Start(ctx)
		close(done)
	}()
	cancel()
	<-done
	// Close channel after Start returns (Stop normally does this)
	// We already tested the drop behaviour; just verify no panic occurred.
}

func TestRecorder_StopDrainsPendingJobs(t *testing.T) {
	var writeCount atomic.Int32
	store := &countingStore{count: &writeCount}

	r := NewRecorder(store, 1000, slog.Default())
	ctx, cancel := context.WithCancel(context.Background())
	go r.Start(ctx)

	check := makeCheck("drain-test")
	result := makeResult(true, 1, "")
	const n = 20
	for i := 0; i < n; i++ {
		r.Record(check, result)
	}

	cancel()
	r.Stop()

	if int(writeCount.Load()) != n {
		t.Errorf("expected %d writes after drain, got %d", n, writeCount.Load())
	}
}

func TestRecorder_PanicInStoreIsRecovered(t *testing.T) {
	store := &panicStore{}
	r, cancel := startRecorder(store)

	check := makeCheck("panic-test")
	r.Record(check, makeResult(true, 1, ""))

	cancel()
	// Should not panic; Stop should complete
	done := make(chan struct{})
	go func() {
		r.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Error("Stop did not complete after panic in store.Write")
	}
}

// countingStore counts Write calls atomically for concurrency-safe testing.
type countingStore struct {
	count *atomic.Int32
}

func (c *countingStore) Write(_ context.Context, _ *models.HealthCheck, _ *models.CheckResult, _ int) error {
	c.count.Add(1)
	return nil
}
func (c *countingStore) QueryCheck(_ context.Context, _ string, _ int) ([]Record, error) {
	return nil, nil
}
func (c *countingStore) QueryAll(_ context.Context, _ int) (map[string][]Record, error) {
	return nil, nil
}
func (c *countingStore) Close() error { return nil }

// panicStore panics on Write to test recovery.
type panicStore struct{}

func (p *panicStore) Write(_ context.Context, _ *models.HealthCheck, _ *models.CheckResult, _ int) error {
	panic("deliberate test panic")
}
func (p *panicStore) QueryCheck(_ context.Context, _ string, _ int) ([]Record, error) {
	return nil, nil
}
func (p *panicStore) QueryAll(_ context.Context, _ int) (map[string][]Record, error) {
	return nil, nil
}
func (p *panicStore) Close() error { return nil }
