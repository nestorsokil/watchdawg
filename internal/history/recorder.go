package history

import (
	"context"
	"log/slog"
	"time"

	"watchdawg/internal/models"
)

const channelBufferSize = 256

type recordJob struct {
	check     *models.HealthCheck
	result    *models.CheckResult
	retention int // effective retention, computed at dispatch time
}

// Recorder is an async, channel-backed HistoryRecorder. Record() is non-blocking;
// if the channel is full, the event is dropped and a warning is logged.
// Start() must be called to begin consuming; Stop() drains remaining jobs and closes the store.
type Recorder struct {
	store           ExecutionStore
	globalRetention int
	ch              chan recordJob
	logger          *slog.Logger
	done            chan struct{}
}

// NewRecorder creates a Recorder backed by the given store.
// globalRetention is the fallback when a check does not specify its own retention.
func NewRecorder(store ExecutionStore, globalRetention int, logger *slog.Logger) *Recorder {
	return &Recorder{
		store:           store,
		globalRetention: globalRetention,
		ch:              make(chan recordJob, channelBufferSize),
		logger:          logger,
		done:            make(chan struct{}),
	}
}

// Record enqueues a write job non-blocking. Drops and warns if the channel is full.
// Effective retention: check.Retention if > 0, otherwise globalRetention.
// Safe to call from multiple goroutines concurrently.
func (r *Recorder) Record(check *models.HealthCheck, result *models.CheckResult) {
	retention := r.globalRetention
	if check.Retention > 0 {
		retention = check.Retention
	}
	job := recordJob{check: check, result: result, retention: retention}
	select {
	case r.ch <- job:
	default:
		r.logger.Warn("History record dropped: channel full", "check", check.Name)
	}
}

// Start runs the background consumer goroutine. Call this once after creating the Recorder.
// The goroutine stops when ctx is cancelled; remaining jobs are drained before exit.
func (r *Recorder) Start(ctx context.Context) {
	defer close(r.done)
	defer func() {
		if p := recover(); p != nil {
			r.logger.Error("History recorder panicked", "panic", p)
		}
	}()

	for {
		select {
		case job, ok := <-r.ch:
			if !ok {
				return
			}
			r.writeJob(job)
		case <-ctx.Done():
			r.drain()
			return
		}
	}
}

// Stop closes the channel to signal the consumer to drain and exit, then waits up to 5s.
// The underlying store is closed after draining.
func (r *Recorder) Stop() {
	close(r.ch)
	select {
	case <-r.done:
	case <-time.After(5 * time.Second):
		r.logger.Warn("History recorder drain timed out")
	}
	r.store.Close()
}

func (r *Recorder) drain() {
	for {
		select {
		case job, ok := <-r.ch:
			if !ok {
				return
			}
			r.writeJob(job)
		default:
			return
		}
	}
}

func (r *Recorder) writeJob(job recordJob) {
	defer func() {
		if p := recover(); p != nil {
			r.logger.Error("History store Write panicked", "check", job.check.Name, "panic", p)
		}
	}()
	if err := r.store.Write(context.Background(), job.check, job.result, job.retention); err != nil {
		r.logger.Error("Failed to write execution record", "check", job.check.Name, "error", err)
	}
}
