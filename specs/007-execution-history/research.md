# Research: Execution History & Reporting

**Branch**: `007-execution-history` | **Phase**: 0

---

## Decision 1: SQLite Driver

**Decision**: `modernc.org/sqlite` (pure-Go, CGO-free)

**Rationale**:
- Watchdawg is built as a single static binary. `mattn/go-sqlite3` requires CGO, which
  complicates cross-compilation and CI builds. `modernc.org/sqlite` is a transpilation of
  the SQLite C source to pure Go — no CGO, no C compiler, standard `go build` works everywhere.
- Implements the `database/sql` interface (`sql.DB`), so we use standard Go database patterns.
- Supports all standard SQLite PRAGMAs including WAL mode.
- Backed by Tailscale; widely deployed in production Go services.
- Minor performance overhead vs CGO (~5–15% on write-heavy workloads) is irrelevant at
  Watchdawg's write rate (one record per check per schedule interval).

**Alternatives considered**:
- `mattn/go-sqlite3`: CGO required → rejected (breaks single-binary cross-compilation).
- `zombiezen.com/go/sqlite`: pure Go but skips `database/sql` in favour of a custom `Conn`/
  `sqlitex.Pool` API. Adds a second dependency layer with no benefit over modernc for this use case.

**Dependency**:
```
go get modernc.org/sqlite
```

Driver name is `"sqlite"` (not `"sqlite3"`):
```go
import _ "modernc.org/sqlite"
db, err := sql.Open("sqlite", "file:watchdawg.db?_journal_mode=WAL&_busy_timeout=5000")
db.SetMaxOpenConns(4)
db.SetMaxIdleConns(4)
```

**Gotchas**:
- Enable WAL in the DSN, not as a post-open PRAGMA, to avoid a race between the connection
  pool and the PRAGMA execution.
- `_busy_timeout=5000` makes concurrent writers back off gracefully without surfacing errors.
- In-memory test databases: use `file::memory:?cache=shared` so multiple connections share
  the same instance. Without `cache=shared` each connection gets its own empty database.

---

## Decision 2: Retention Strategy — On-Write vs Background Goroutine

**Decision**: On-write eviction (delete oldest records in the same transaction as the insert)

**Rationale**:
- Watchdawg is the only writer to this SQLite file. The store can only grow when Watchdawg
  inserts a record, so evicting on insert is sufficient and deterministic.
- Eviction is atomic: a single `DELETE … WHERE id NOT IN (SELECT id … ORDER BY timestamp DESC
  LIMIT N)` runs in the same transaction as the INSERT. The table can never exceed the limit
  between commits.
- No additional goroutines → no lifecycle management, no panic recovery, no context plumbing.
  One goroutine (the async write consumer) already handles all writes sequentially.
- Background goroutines per-check would add N goroutines (one per recorded check) sweeping a
  table that is already bounded. The overhead is unnecessary complexity with no correctness
  advantage (Constitution IV).
- Eviction latency: negligible. SQLite DELETE with an index on `(check_name, id)` completes in
  microseconds for the small row counts involved.

---

## Decision 3: Scheduler Integration — `HistoryRecorder` Interface

**Decision**: New `HistoryRecorder` interface in `internal/healthcheck`; scheduler holds an
optional field, called nil-safely in `executeHealthCheck`.

**Why not merge with `MetricsRecorder`**:
- `MetricsRecorder` methods (`RecordCheckAttempt`, `RecordMessageAge`, etc.) are called
  **per retry attempt**, from inside individual checker `Execute()` methods.
- `HistoryRecorder.Record()` must fire **once per top-level execution** (after all retries),
  from the scheduler — per FR-002 (per-execution granularity, not per-attempt).
- These are different event granularities. Merging the interfaces would either bloat them or
  require callers to pass data they don't have at the wrong time.
- `MetricsRecorder` is already non-blocking (Prometheus ops are lock-free in-memory). No
  benefit from adding async dispatch there.

**Interface**:
```go
// HistoryRecorder is called once per top-level check execution (after all retries).
// Implementations must be non-blocking; a slow Record will delay hook dispatch.
type HistoryRecorder interface {
    Record(check *models.HealthCheck, result *models.CheckResult)
}
```

Scheduler change: add `history HistoryRecorder` field (nil by default); add
`SetHistoryRecorder(HistoryRecorder)` method; call `if s.history != nil {
s.history.Record(&check, result) }` in `executeHealthCheck` — 3 lines of new code.

**Async write path**:
The `internal/history` implementation uses an internal buffered channel. `Record()` is a
non-blocking push; a single background goroutine drains the channel and writes to SQLite.
This decouples check execution latency from DB write latency entirely.

```
Scheduler → history.Record(check, result)  // non-blocking, pushes to channel
                  ↓
          internal buffered chan
                  ↓
          background goroutine → SQLiteStore.Write() + eviction
```

On shutdown: drain the channel (with a timeout) before closing the store.

---

## Decision 4: Sharing the HTTP Port with the Metrics Server

**Decision**: Extract route registration from `MetricsServer.Start()` into a new
`RegisterRoutes(mux *http.ServeMux)` method. `Start()` creates its own mux internally
(unchanged behaviour for metrics-only case). In main.go, when both are configured,
create a shared mux, register both, and start one `net/http.Server`.

**Rationale**:
- Minimal change to `MetricsServer`: one new method, `Start()` signature unchanged.
- If only metrics is configured: `metricsServer.Start(ctx)` works exactly as before.
- If both are configured: main.go creates a shared mux, registers both route sets, starts
  one server on the metrics address. The history package never needs to know the address.
- History API is only available when metrics is configured (the port it would bind to comes
  from `MetricsConfig.Address`). If metrics is absent, the history store still writes records
  but the REST API is not started.

**main.go change (when history + metrics both present)**:
```go
sharedMux := http.NewServeMux()
metricsServer.RegisterRoutes(sharedMux)
historyServer.RegisterRoutes(sharedMux)
// start one http.Server on metricsServer.Address() — metrics.Start() not called separately
```

---

## Decision 5: Package Layout

**Decision**: `internal/history` package; `HistoryRecorder` interface defined in
`internal/healthcheck`.

```
internal/healthcheck/
└── history_recorder.go   # HistoryRecorder interface (new file, no changes to existing files)

internal/history/
├── store.go              # ExecutionStore interface + SQLiteStore
├── recorder.go           # Async channel-backed HistoryRecorder implementation
└── api.go                # HTTP handler: GET /history/{check_name}, GET /history/*
```

No circular dependencies: `internal/history` imports `internal/healthcheck` (for the
interface) and `internal/models` (for types). Existing healthcheck files are unchanged.

---

## Decision 6: Config Schema

**New fields** (additive, backward-compatible):

```json
{
  "history": {
    "db_path": "./watchdawg.db",
    "retention": 1000,
    "record_all_healthchecks": false
  },
  "healthchecks": [
    {
      "name": "my-api",
      "record": true,
      "retention": 500,
      "..."
    }
  ]
}
```

- `history` block absent → no recording, no API. Zero impact on existing deployments.
- `record_all_healthchecks` defaults to `false`. When `true`, all checks are opted in without requiring `record: true` per check.
- Per-check `record: true` is honoured when `record_all_healthchecks` is `false`; redundant but harmless when `true`.
- Per-check `retention` overrides the global default when set.
- Global `retention` defaults to `1000` if `history` is configured but `retention` omitted.
- `db_path` required when `history` block is present; supports `$VAR` expansion (Constitution I).
