# Implementation Plan: Execution History & Reporting

**Branch**: `007-execution-history` | **Date**: 2026-03-06 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/007-execution-history/spec.md`

## Summary

Add opt-in execution history recording to WatchDawg: each scheduled check execution (after
all retries) can be persisted to a local SQLite database and queried via a REST API co-hosted
on the Prometheus metrics port. Recording is decoupled from the check hot-path via an async
channel. The feature is implemented as a plugin — the `internal/history` package is wired in
from `main.go` with minimal changes to existing code.

## Technical Context

**Language/Version**: Go 1.24+
**Primary Dependencies**: `modernc.org/sqlite` (new; pure-Go SQLite, no CGO)
**Storage**: SQLite (local file, WAL mode)
**Testing**: `go test ./...` (unit); pytest + Docker Compose (integration)
**Target Platform**: Linux/macOS server (single binary)
**Project Type**: CLI daemon
**Performance Goals**: API response < 1s with thousands of records (SC-001); recording adds
no measurable latency to check execution (SC-002; async channel write)
**Constraints**: localhost-only API; single binary (no CGO)
**Scale/Scope**: O(checks × retention) rows; default 1000 records/check

## Constitution Check

| Gate | Principle | Status |
|------|-----------|--------|
| Config errors are enumerated and path-scoped (index + check name + field) | I. Operator-First Configuration | ✅ Covered — loader validation adds path-scoped errors for `history.path` (required), `history.retention` (must be > 0 if set), per-check `retention` (must be > 0 if set) |
| Every new config field has a default or clear required-field validation | I. Operator-First Configuration | ✅ `record` defaults false; `retention` defaults 1000 globally; `path` is required-with-error |
| New log lines use correct slog level and include `check` key | II. Structured Observability | ✅ Recorder logs dropped events (Warn); store logs write errors (Error, with `check` key); API logs requests (Debug) |
| New check/hook type adds metrics to MetricsRecorder + both implementations | II. Structured Observability | ✅ N/A — no new check or hook type in this feature |
| Unit tests cover: success, failure, retry, timeout, assertions (where applicable) | III. Test Discipline | ✅ store_test.go covers write, eviction, concurrent writes, read, not-found; recorder_test.go covers async dispatch, drop-on-full, drain-on-stop; api_test.go covers 200, 404, 400, limit, all-checks |
| External deps replaced with in-process stubs in unit tests | III. Test Discipline | ✅ SQLite in-memory (`file::memory:?cache=shared`) used in store tests; no network in unit tests |
| All new I/O operations accept and respect context.Context | IV. Explicit Concurrency | ✅ `SQLiteStore.Write(ctx)` and `SQLiteStore.Query(ctx)` accept context; async goroutine respects shutdown ctx |
| New goroutines have panic recovery; shutdown cleanup is implemented | IV. Explicit Concurrency | ✅ Async write goroutine has `defer recover()` with error log; `Stop(ctx)` drains channel before close |
| New connections are pooled/reused; response bodies are explicitly closed | V. Resource Efficiency | ✅ One `*sql.DB` opened at startup; `SetMaxOpenConns(4)`; `SetMaxIdleConns(4)` |
| No new third-party deps without PR justification | VI. Minimal Footprint | ⚠️ **One new dep**: `modernc.org/sqlite` — justified: pure-Go SQLite is the only viable option for persistent storage in a single binary without CGO. No stdlib alternative exists. |

**Constitution Violation — Principle VI (Minimal Footprint)**:

> "The daemon MUST remain a single binary with no persistent storage, no UI, and no HTTP API
> server. Adding any of these requires revisiting the product scope, not just a PR."

This feature deliberately adds both persistent storage (SQLite) and a REST API. This is a
**product scope expansion** approved via the spec process (spec.md exists and was ratified).
See Complexity Tracking table.

## Project Structure

### Documentation (this feature)

```
specs/007-execution-history/
├── plan.md              ← this file
├── research.md          ← Phase 0: key decisions (SQLite driver, retention, interfaces)
├── data-model.md        ← Phase 1: schema, config structs, API response shapes
├── contracts/
│   └── history-api.md  ← Phase 1: REST API contract
└── tasks.md             ← Phase 2 (/speckit.tasks — not yet created)
```

### Source Code Changes

```
cmd/watchdawg/
└── main.go                        MODIFY  — wire up history (15-20 lines)

internal/models/
└── config.go                      MODIFY  — add HistoryConfig, record/retention to HealthCheck

internal/config/
└── loader.go                      MODIFY  — add history validation + defaults

internal/metrics/
└── server.go                      MODIFY  — add RegisterRoutes(mux), refactor Start() to call it

internal/healthcheck/
└── history_recorder.go            NEW     — HistoryRecorder interface (3 lines)

internal/history/
├── store.go                       NEW     — ExecutionStore interface + SQLiteStore
├── recorder.go                    NEW     — async channel-backed HistoryRecorder impl
└── api.go                         NEW     — HTTP handler for /history/* endpoints

configs/
└── config.example.json            MODIFY  — add history block + record/retention fields
```

**Existing files not touched**: `healthcheck/scheduler.go` gets `history HistoryRecorder`
field + `SetHistoryRecorder()` + one nil-safe call in `executeHealthCheck` — 6 lines total.
All checker files (`http.go`, `grpc.go`, `kafka.go`, `starlark.go`), `hooks.go`, and
`metrics/server.go` implementation methods are **unchanged**.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| Persistent storage (SQLite) | Feature's core requirement (FR-003: data must survive restarts) | In-memory storage loses data on restart — explicitly against FR-003 |
| HTTP REST API | Operator reporting interface (FR-004, FR-010); HTML UI is a subsequent feature built on this | Prometheus metrics are aggregate-only and have no per-execution error detail (spec clarification, FR-013) |
| New third-party dep (`modernc.org/sqlite`) | SQLite requires a driver; no stdlib SQL driver exists | CGO-based `mattn/go-sqlite3` breaks single-binary cross-compilation; no pure-Go alternative without this dep |

---

## Design Details

### HistoryRecorder Interface (new file in `internal/healthcheck`)

```go
// HistoryRecorder is called once per top-level check execution (after all retries).
// Implementations MUST be non-blocking; Record is called in the check execution hot-path.
type HistoryRecorder interface {
    Record(check *models.HealthCheck, result *models.CheckResult)
}
```

Scheduler integration (in `scheduler.go`):

```go
// New field on Scheduler:
history HistoryRecorder  // nil if history not configured

// New method:
func (s *Scheduler) SetHistoryRecorder(h HistoryRecorder) { s.history = h }

// In executeHealthCheck, after RecordCheckUp:
if s.history != nil {
    s.history.Record(&check, result)
}
```

### Async Recorder (`internal/history/recorder.go`)

```go
type Recorder struct {
    store  ExecutionStore
    ch     chan recordJob
    logger *slog.Logger
}

type recordJob struct {
    check  *models.HealthCheck
    result *models.CheckResult
}

// Record is non-blocking: drops the event if the channel is full and logs a warning.
func (r *Recorder) Record(check *models.HealthCheck, result *models.CheckResult) {
    select {
    case r.ch <- recordJob{check: check, result: result}:
    default:
        r.logger.Warn("History record dropped: channel full", "check", check.Name)
    }
}

// Start runs the background consumer. Caller passes the scheduler's root ctx.
// On ctx cancellation: drains remaining jobs (with timeout) then closes the store.
func (r *Recorder) Start(ctx context.Context)

// Stop drains the channel and closes the store.
func (r *Recorder) Stop()
```

Channel buffer size: 256 (configurable constant). With default schedules (30s–5m), this
gives >100 checks several seconds of burst capacity before dropping.

### SQLite Store (`internal/history/store.go`)

```go
type ExecutionStore interface {
    Write(ctx context.Context, job WriteJob) error
    QueryCheck(ctx context.Context, checkName string, limit int) ([]Record, error)
    QueryAll(ctx context.Context, limit int) (map[string][]Record, error)
    Close() error
}

type SQLiteStore struct {
    db        *sql.DB
    retention int  // global default; per-check limit passed at Write time
}
```

`Write` runs INSERT + eviction DELETE in a single transaction.
`QueryCheck` returns 404-sentinel `ErrNotFound` when no rows exist for the check name.

### Metrics Server Route Registration (`internal/metrics/server.go`)

```go
// New method — registers /metrics on the given mux.
func (s *MetricsServer) RegisterRoutes(mux *http.ServeMux) {
    mux.Handle("/metrics", promhttp.HandlerFor(s.registry, promhttp.HandlerOpts{
        ErrorLog: slog.NewLogLogger(s.logger.Handler(), slog.LevelError),
    }))
}

// Start refactored — creates its own mux internally (unchanged external behaviour).
func (s *MetricsServer) Start(ctx context.Context) error {
    mux := http.NewServeMux()
    s.RegisterRoutes(mux)
    // ... rest unchanged
}
```

### main.go Wiring

```go
// History setup (after metrics server creation, before scheduler.Start()):
if cfg.History != nil {
    store, err := history.NewSQLiteStore(cfg.History, logger)
    if err != nil {
        logger.Error("Failed to open history store", "path", cfg.History.Path, "error", err)
        os.Exit(1)
    }
    recorder := history.NewRecorder(store, logger)
    scheduler.SetHistoryRecorder(recorder)

    if metricsServer != nil {
        historyHandler := history.NewHandler(store, logger)
        sharedMux := http.NewServeMux()
        metricsServer.RegisterRoutes(sharedMux)
        historyHandler.RegisterRoutes(sharedMux)
        // start shared server (replaces metricsServer.Start(metricsCtx) call below)
        go func() { /* http.Server on metricsServer.Address(), sharedMux */ }()
        metricsServerStarted = true
    }

    go recorder.Start(metricsCtx)
    defer recorder.Stop()
}

if metricsServer != nil && !metricsServerStarted {
    go func() {
        if err := metricsServer.Start(metricsCtx); err != nil { ... }
    }()
}
```

To keep main.go clean, the shared-server startup is extracted into a helper function.
`metricsServer.Address()` is a new trivial accessor on `MetricsServer` returning `cfg.Address`.
