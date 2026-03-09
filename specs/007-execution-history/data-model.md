# Data Model: Execution History & Reporting

**Branch**: `007-execution-history` | **Phase**: 1

---

## Entities

### ExecutionRecord

Represents a single top-level check execution (after all retries).

| Field        | Type     | Constraints              | Notes                                      |
|--------------|----------|--------------------------|--------------------------------------------|
| `id`         | INTEGER  | PK, AUTOINCREMENT        | Internal row identifier; used for eviction |
| `check_name` | TEXT     | NOT NULL                 | Matches `HealthCheck.Name`                 |
| `timestamp`  | INTEGER  | NOT NULL                 | Unix nanoseconds; when execution started   |
| `healthy`    | INTEGER  | NOT NULL, 0 or 1         | `1` = pass, `0` = fail                     |
| `duration_ms`| INTEGER  | NOT NULL, >= 0           | Total wall-clock duration including retries|
| `error`      | TEXT     | NOT NULL, DEFAULT ''     | Empty string if healthy                    |

**Source fields from `models.CheckResult`**:
- `check_name` ← `CheckResult.CheckName`
- `timestamp` ← `CheckResult.Timestamp` (nanoseconds)
- `healthy` ← `CheckResult.Healthy`
- `duration_ms` ← `CheckResult.Duration`
- `error` ← `CheckResult.Error`

`Attempt` and `HTTPResult`/`GRPCResult` from `CheckResult` are **not** stored — this feature
records execution-level outcomes, not attempt-level or protocol-level details.

---

### RetentionPolicy

Not a stored entity — a config-time constraint enforced on write.

| Source           | Field              | Default | Notes                               |
|------------------|--------------------|---------|-------------------------------------|
| `HistoryConfig`  | `Retention`        | 1000    | Global default; applies to all checks unless overridden |
| `HealthCheck`    | `Retention`        | 0       | 0 = use global default              |

Effective retention for a check = `check.Retention` if > 0, else `config.History.Retention`.

---

## SQLite Schema

```sql
CREATE TABLE IF NOT EXISTS execution_records (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    check_name  TEXT    NOT NULL,
    timestamp   INTEGER NOT NULL,
    healthy     INTEGER NOT NULL,
    duration_ms INTEGER NOT NULL,
    error       TEXT    NOT NULL DEFAULT ''
);

-- Supports: per-check reverse-chronological queries, retention eviction by check
CREATE INDEX IF NOT EXISTS idx_exec_check_ts
    ON execution_records (check_name, timestamp DESC);
```

**PRAGMAs** (applied at open via DSN):
- `journal_mode=WAL` — concurrent read throughput; safe for multiple goroutines
- `busy_timeout=5000` — 5s retry on `SQLITE_BUSY` instead of immediate error
- Foreign keys not needed (no relational structure)

**Connection pool** (`sql.DB`):
- `SetMaxOpenConns(4)` — small pool; writes serialise at SQLite level under WAL
- `SetMaxIdleConns(4)`

---

## Write Path (per execution)

```
BEGIN TRANSACTION
  INSERT INTO execution_records (check_name, timestamp, healthy, duration_ms, error)
  VALUES (?, ?, ?, ?, ?);

  DELETE FROM execution_records
  WHERE check_name = ?
    AND id NOT IN (
      SELECT id FROM execution_records
      WHERE check_name = ?
      ORDER BY timestamp DESC
      LIMIT ?   -- effective retention limit
    );
COMMIT
```

The delete is a no-op when the count is within the limit, so it adds negligible overhead for
the common case.

---

## Read Path

**Per-check query** (`GET /history/{check_name}?limit=N`):

```sql
SELECT timestamp, healthy, duration_ms, error
FROM execution_records
WHERE check_name = ?
ORDER BY timestamp DESC
LIMIT ?;
```

**All-checks query** (`GET /history/*?limit=N`):

```sql
-- Enumerate distinct recorded check names
SELECT DISTINCT check_name FROM execution_records ORDER BY check_name;

-- Then for each check_name (or as a single query with ROW_NUMBER in future):
SELECT check_name, timestamp, healthy, duration_ms, error
FROM execution_records
WHERE check_name = ?
ORDER BY timestamp DESC
LIMIT ?;
```

Future datetime range filtering adds a `WHERE timestamp BETWEEN ? AND ?` clause to the
per-check query; the index on `(check_name, timestamp DESC)` covers this efficiently.

---

## Config Structs (Go)

```go
// Added to models.Config (root)
type Config struct {
    Metrics      *MetricsConfig `json:"metrics,omitempty"`
    History      *HistoryConfig `json:"history,omitempty"`   // NEW
    HealthChecks []HealthCheck  `json:"healthchecks"`
}

// New top-level config block
type HistoryConfig struct {
    DBPath    string `json:"db_path"`                       // required; SQLite file path
    Retention int    `json:"retention,omitempty"`            // default: 1000
    RecordAll bool   `json:"record_all_healthchecks,omitempty"` // default: false; opts every check in
}

// New fields on HealthCheck
type HealthCheck struct {
    // ... existing fields ...
    Record    bool `json:"record,omitempty"`    // default: false; overridden by RecordAll
    Retention int  `json:"retention,omitempty"` // 0 = use global default
}
```

---

## API Response Shapes (Go)

```go
// GET /history/{check_name}
type CheckHistoryResponse struct {
    CheckName string            `json:"check_name"`
    Records   []ExecutionRecord `json:"records"`
}

// GET /history/*
type AllHistoryResponse struct {
    Checks map[string][]ExecutionRecord `json:"checks"`
}

// Shared record shape in API responses
type ExecutionRecord struct {
    Timestamp   time.Time `json:"timestamp"`    // RFC3339
    Healthy     bool      `json:"healthy"`
    DurationMs  int64     `json:"duration_ms"`
    Error       string    `json:"error"`
}
```

`ExecutionRecord` in API responses uses `time.Time` (serialises as RFC3339) converted from
the stored nanosecond integer. The `id` column is internal and never exposed in responses.
