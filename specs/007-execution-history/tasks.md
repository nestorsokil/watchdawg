# Tasks: Execution History & Reporting

**Input**: Design documents from `/specs/007-execution-history/`
**Prerequisites**: plan.md ✅, spec.md ✅, research.md ✅, data-model.md ✅, contracts/ ✅

**Tests**: Unit tests are MANDATORY per Constitution III. All new store logic, recorder
lifecycle, config validation, and HTTP handler behaviour must have unit test coverage.
Integration tests are in the Polish phase and **require Docker + explicit approval before running**.

**Organization**: Tasks are grouped by user story to enable independent implementation and
testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no in-phase dependency)
- **[Story]**: User story label (US1, US2, US3)

---

## Phase 1: Setup

**Purpose**: Add new dependency; create package skeleton.

- [X] T001 Add `modernc.org/sqlite` dependency: run `go get modernc.org/sqlite` and verify `go.mod` / `go.sum` are updated
- [X] T002 [P] Create `internal/history/` package with empty placeholder files: `store.go`, `recorder.go`, `api.go` (each with `package history` only)

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Config schema and validation changes that all three user stories depend on.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [X] T003 [P] Add `HistoryConfig` struct (`DBPath string`, `Retention int`, `RecordAll bool`) and `Record bool` + `Retention int` fields to `HealthCheck` in `internal/models/config.go`
- [X] T004 Add history validation + defaults to `internal/config/loader.go`: `db_path` required when `history` block present; `retention` defaults to `1000`; per-check `retention` must be `> 0` if set; `record_all_healthchecks` defaults to `false`; path-scoped error messages per Constitution I
- [X] T005 [P] Update `configs/config.example.json` with `history` block (`db_path`, `retention`, `record_all_healthchecks`) and `record` / `retention` fields in an example check entry
- [X] T006 [P] Add unit tests for history config validation in `internal/config/loader_test.go`: missing `db_path`, invalid `retention`, valid block, `record_all_healthchecks` default, per-check retention override, no `history` block (zero impact on existing checks)

**Checkpoint**: Config schema and validation complete — user story phases can begin

---

## Phase 3: User Story 1 — Record Healthcheck Executions (Priority: P1) 🎯 MVP

**Goal**: Each opted-in check execution writes one record to a local SQLite database that survives daemon restarts. A check is opted in if `record: true` or `record_all_healthchecks: true`.

**Independent Test**: Configure one check with `"record": true`, run Watchdawg for a few ticks, stop it, restart it, verify records are still present and accurate.

### Unit Tests for US1

- [X] T007 [P] [US1] Write unit tests for `SQLiteStore.Write` in `internal/history/store_test.go`: write succeeds and all fields are persisted correctly (check name, healthy, duration, error, timestamp); `context.Context` cancellation returns an error; use in-memory DB (`file::memory:?cache=shared`) throughout
- [X] T010 [P] [US1] Write unit tests for `Recorder` in `internal/history/recorder_test.go`: `Record()` dispatches to store; drop-on-full logs Warn and does not block the caller; `Stop()` drains pending jobs before returning; panic in store `Write` is recovered and logged

### Implementation for US1

- [X] T008 [P] [US1] Define `HistoryRecorder` interface in `internal/healthcheck/history_recorder.go`: `Record(check *models.HealthCheck, result *models.CheckResult)`
- [X] T009 [P] [US1] Implement `ExecutionStore` interface, `Record` struct, `SQLiteStore` struct, `NewSQLiteStore(cfg *models.HistoryConfig, logger *slog.Logger)` (opens DB with WAL + busy-timeout DSN, runs `CREATE TABLE IF NOT EXISTS`, sets connection pool limits), and `Write(ctx context.Context, check *models.HealthCheck, result *models.CheckResult, retention int)` (INSERT only; `retention` parameter accepted but eviction deferred to US3), `Close() error` in `internal/history/store.go`
- [X] T011 [US1] Add `history HistoryRecorder` field and `SetHistoryRecorder(h HistoryRecorder)` method to `Scheduler` in `internal/healthcheck/scheduler.go`; add nil-safe `if s.history != nil { s.history.Record(&check, result) }` call in `executeHealthCheck` after `RecordCheckUp`
- [X] T012 [US1] Implement `Recorder` in `internal/history/recorder.go`: buffered channel (const `256`), `NewRecorder(store ExecutionStore, logger *slog.Logger)`, non-blocking `Record()` with `slog.Warn` on drop, `Start(ctx context.Context)` background goroutine with `defer recover()` and error log, `Stop()` that drains remaining jobs (up to 5 s timeout) then calls `store.Close()`
- [X] T013 [US1] Wire history in `cmd/watchdawg/main.go`: after metrics setup, if `cfg.History != nil` open `SQLiteStore` (fail-fast with `logger.Error` + `os.Exit(1)` on error), create `Recorder`, call `scheduler.SetHistoryRecorder(recorder)`, `go recorder.Start(metricsCtx)`, defer `recorder.Stop()`; compute effective opt-in per check: `cfg.History.RecordAll || check.Record`

**Checkpoint**: US1 complete — configure `"record": true` on any check; records write to SQLite and survive restart

---

## Phase 4: User Story 2 — Query Execution History via API (Priority: P2)

**Goal**: REST API at `/history/{check_name}` and `/history/*` returns stored records with uniform `{"checks":{}}` response shape, co-hosted on the Prometheus metrics port.

**Independent Test**: Run a few recorded executions, call both endpoints, verify count, reverse-chronological ordering, and correct field values (`timestamp`, `healthy`, `duration_ms`, `error`).

### Unit Tests for US2

- [X] T015 [P] [US2] Add unit tests for `SQLiteStore.QueryCheck` and `QueryAll` in `internal/history/store_test.go`: found returns records newest-first; not-found returns `ErrNotFound`; `limit` is respected; multiple check names are isolated correctly
- [X] T017 [P] [US2] Write unit tests for `Handler` in `internal/history/api_test.go`: `GET /history/{check_name}` → 200 with records; 404 on not found; 400 on invalid limit; `GET /history/*` → 200 with empty `checks` map; 200 with records from multiple checks; default limit (100) applied when absent

### Implementation for US2

- [X] T014 [US2] Add `QueryCheck(ctx context.Context, checkName string, limit int) ([]Record, error)` and `QueryAll(ctx context.Context, limit int) (map[string][]Record, error)` methods plus `ErrNotFound` sentinel error to `internal/history/store.go`
- [X] T016 [US2] Create `internal/history/api.go`: `Handler` struct, `NewHandler(store ExecutionStore, logger *slog.Logger)`, `RegisterRoutes(mux *http.ServeMux)`; implement `GET /history/{check_name}` (200 / 404 / 400) and `GET /history/*` (200 / 400); both return `{"checks":{...}}`; default limit 100; timestamps serialised as RFC3339
- [X] T018 [P] [US2] Add `RegisterRoutes(mux *http.ServeMux)` method to `MetricsServer` in `internal/metrics/server.go` that registers `/metrics` on the given mux; refactor `Start()` to create its own mux and call `RegisterRoutes` on it (no change to the existing `Start(ctx)` signature or behaviour); add `Address() string` accessor returning `cfg.Address`
- [X] T019 [US2] Update `cmd/watchdawg/main.go`: when both `cfg.History != nil` and `metricsServer != nil`, create a shared `*http.ServeMux`, call `metricsServer.RegisterRoutes(sharedMux)` and `historyHandler.RegisterRoutes(sharedMux)`, start one `http.Server` on `metricsServer.Address()`; fall back to `metricsServer.Start(metricsCtx)` when only metrics is configured

**Checkpoint**: US2 complete — history API live on metrics port; both endpoints return correct, ordered records

---

## Phase 5: User Story 3 — Bounded Storage via Retention Policy (Priority: P3)

**Goal**: The execution store never exceeds the configured record count per check; oldest records are evicted atomically at write time.

**Independent Test**: Set `retention: 5`, run 10 executions, verify only 5 records remain and they are the 5 most recent.

### Unit Tests for US3

- [X] T021 [P] [US3] Add eviction unit tests in `internal/history/store_test.go`: row count stays at retention limit after N writes; oldest records are evicted (not newest); per-check `Retention` override beats global default; unconfigured per-check retention resolves to global default of 1000
- [X] T023 [P] [US3] Add unit tests for per-check retention config in `internal/config/loader_test.go`: per-check `retention: 0` accepted (resolves to global at write time); explicit positive value accepted; negative value rejected with path-scoped error

### Implementation for US3

- [X] T020 [US3] Add eviction `DELETE` to `SQLiteStore.Write` in `internal/history/store.go` inside the same transaction: `DELETE FROM execution_records WHERE check_name = ? AND id NOT IN (SELECT id FROM execution_records WHERE check_name = ? ORDER BY timestamp DESC LIMIT ?)`; compute effective retention in `Recorder.Record`: `check.Retention` if `> 0`, else `cfg.Retention`; pass result to `store.Write`
- [X] T022 [P] [US3] Add per-check `Retention` propagation note to `internal/config/loader.go`: if per-check `Retention == 0`, leave as zero (Recorder resolves to global default at write time); add a code comment documenting the resolution rule

**Checkpoint**: All three user stories functional — store is bounded, API returns correct data, recording survives restarts

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Integration coverage and final validation.

> ⚠️ Integration tests require Docker Compose — **ask before running** (see CLAUDE.md).

- [X] T024 [P] Add integration test for recording in `integration-tests/tests/test_history.py`: configure a check with `record: true`; run several scheduler ticks; stop daemon; restart; assert records persist and all fields are correct
- [X] T025 [P] Add integration test for `record_all_healthchecks` in `integration-tests/tests/test_history.py`: configure `history.record_all_healthchecks: true` with no per-check `record` field; verify all checks produce history records
- [X] T026 [P] Add integration test for history API in `integration-tests/tests/test_history.py`: call `GET /history/{check_name}` and `GET /history/*`; assert HTTP 200, correct record count, newest-first ordering, correct `error` field on failed executions; assert 404 for unknown check name
- [X] T027 Run `go test ./...` and verify all unit tests pass; fix any regressions before marking complete

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1** (Setup): No dependencies — start immediately
- **Phase 2** (Foundational): Depends on Phase 1 — **blocks all user story phases**
- **Phase 3** (US1 P1): Depends on Phase 2 — independent of US2 and US3
- **Phase 4** (US2 P2): Depends on Phase 2 + US1's `SQLiteStore` (read methods extend the same struct; T014 follows T009)
- **Phase 5** (US3 P3): Depends on Phase 3 (US1 Write path established; T020 extends T009)
- **Phase 6** (Polish): Depends on all desired story phases complete

### User Story Dependencies

- **US1 (P1)**: Depends only on Phase 2 — fully independent
- **US2 (P2)**: Depends on Phase 2 + US1's `SQLiteStore` (read path extends the write-path struct)
- **US3 (P3)**: Depends on Phase 2 + US1's `SQLiteStore.Write` (eviction extends the same transaction)

### Within Each Story

- Unit tests before implementation where possible (interface must be defined first)
- Store interface (T009) before store tests (T007, T015, T021)
- Recorder (T012) after interface (T008) and store (T009)
- `main.go` wiring last within each story (T013, T019)

---

## Parallel Opportunities

### Phase 3 (US1)

```
Parallel start:
  T007 (store_test.go — Write tests, once T009 interface is sketched)
  T008 (history_recorder.go — interface definition)
  T009 (store.go — SQLiteStore + Write)

After T008 + T009 complete:
  T010 (recorder_test.go)       T011 (scheduler.go — wiring)
  T012 (recorder.go)

After T011 + T012:
  T013 (main.go — wiring)
```

### Phase 4 (US2)

```
After T009 (store struct exists):
  T014 (store.go — add QueryCheck/QueryAll)

After T014 (parallel):
  T015 (store_test.go — query tests)
  T016 (api.go — handler)
  T018 (metrics/server.go — RegisterRoutes)  ← fully parallel to T015/T016

After T016 + T018:
  T017 (api_test.go)
  T019 (main.go — shared mux)
```

### Phase 5 (US3)

```
Parallel:
  T020 (store.go — add eviction)
  T021 (store_test.go — eviction tests)
  T022 (loader.go — retention comment)
  T023 (loader_test.go — retention config tests)
```

---

## Implementation Strategy

### MVP (Phase 1 + 2 + 3 only)

1. Complete Phase 1: add dependency
2. Complete Phase 2: config schema + validation
3. Complete Phase 3: SQLiteStore write, Recorder, scheduler hook, main.go wiring
4. **STOP and VALIDATE**: configure `"record": true` on one check; verify records written to SQLite file; restart daemon and verify they persist
5. Ship — history is recording even without a query API

### Incremental Delivery

1. Phase 1 + 2 + 3 → Recording works (MVP)
2. Add Phase 4 (US2) → Query API live on the metrics port
3. Add Phase 5 (US3) → Retention enforced; store stays bounded
4. Phase 6 → Integration test coverage

---

## Notes

- `[P]` tasks operate on different files with no in-phase data dependency
- `[US1/2/3]` maps each task to its user story for traceability
- SQLite in-memory DSN for tests: `file::memory:?cache=shared` with `db.SetMaxOpenConns(1)`
- Effective opt-in per check: `cfg.History.RecordAll || check.Record`
- Effective retention per check: `check.Retention` if `> 0`, else `cfg.History.Retention`
- History API is only reachable when `metrics` block is configured (port comes from `MetricsConfig.Address`)
- All store I/O methods must accept and respect `context.Context` (Constitution IV)
- `Recorder.Record()` must never block the scheduler hot-path — non-blocking channel push or immediate drop
- `go test ./...` MUST pass before marking any phase complete
