<!--
SYNC IMPACT REPORT
==================
Version Change: (blank template) → 1.0.0
Rationale: Initial ratification — all placeholder tokens replaced with WatchDawg-specific
content derived from exhaustive codebase analysis (March 2026).

Added Principles:
  - I. Operator-First Configuration (new)
  - II. Structured Observability (new)
  - III. Test Discipline (new)
  - IV. Explicit Concurrency and Lifecycle (new)
  - V. Resource Efficiency (new)
  - VI. Minimal Footprint (new)

Added Sections:
  - "Technology Stack" (replaces [SECTION_2_NAME])
  - "Development Workflow" (replaces [SECTION_3_NAME])
  - "Governance" (fully populated)

Templates:
  - ✅ .specify/templates/plan-template.md — Constitution Check updated with WatchDawg gates
  - ✅ .specify/templates/tasks-template.md — Testing note updated to reflect Principle III
  - ✅ .specify/templates/spec-template.md — No structural changes needed
  - ✅ .specify/templates/agent-file-template.md — Auto-generated; no changes needed

Deferred TODOs:
  - None. RATIFICATION_DATE set to 2026-03-06 (first authoring date; treated as adoption date).
-->

# WatchDawg Constitution

## Core Principles

### I. Operator-First Configuration

WatchDawg has no UI. The config file and its error messages are the entire operator interface
at setup time. Configuration quality directly determines how fast a misconfiguration can be
diagnosed and fixed.

- Config MUST be validated fully at startup, before any goroutines or connections are opened.
  A partially-started daemon is worse than one that refuses to start.
- Validation errors MUST be enumerated and path-scoped: include the array index, the check
  name, and the offending field. "invalid check at index 2 (api-health): HTTP URL required"
  is acceptable; "invalid config" is not.
- Defaults MUST be applied silently. Operators MUST NOT be required to specify values that
  have obvious safe defaults (timeout, retries, HTTP method, Kafka group ID).
- Every new config field MUST have a clear validation rule, a default if optional, and an
  entry in `configs/config.example.json`.
- Environment variable expansion (`$VAR`, `${VAR}`) MUST work in all string fields, enabling
  secrets to stay out of config files.

**Rationale**: Operators typically configure WatchDawg during incident setup or infra
bootstrapping, not during leisurely development. Ambiguous errors waste time they don't have.

### II. Structured Observability

Logs and metrics are the only window into a running WatchDawg process. Both MUST be
consistent, complete, and machine-readable from day one — not retrofitted.

**Logging**:

- Use `log/slog` with key-value pairs everywhere. `fmt.Println`, `log.Printf`, and unstructured
  log statements are forbidden in non-test code.
- Log levels MUST be used exactly as follows:
  - **Info**: normal state transitions — daemon start/stop, check scheduled, hook dispatched
  - **Warn**: degraded but continuing — check unhealthy, consumer error, stale message
  - **Error**: failures that require operator attention — hook failed, config invalid, panic caught
  - **Debug**: diagnostic details that would flood normal operation — body previews, script errors
- An error MUST be logged exactly once: at the point where it is handled and not re-propagated.
  Logging an error and also returning it produces duplicate noise in the log stream.
- Every log line at Info level or above MUST include the `check` key when it concerns a specific
  check. Without this field, log correlation is impossible in multi-check deployments.
- `LOG_FORMAT=json` MUST activate `slog.NewJSONHandler`; the default is `slog.NewTextHandler`.

**Metrics**:

- New check types MUST instrument: execution count, execution duration, and current up/down state.
- New hook types MUST instrument: dispatch count and dispatch duration.
- `NoopMetricsRecorder` MUST be used when no metrics config is present. Metrics recording
  MUST have zero overhead in the noop path.
- Adding a metric to `MetricsRecorder` interface MUST be accompanied by implementations in
  both `MetricsServer` and `NoopMetricsRecorder`. An unimplemented interface method is a
  compile error, not a silent omission.

**Rationale**: Operators rely on logs for incident response and on metrics for alerting.
Inconsistent log fields break log queries. Missing metrics make alerting unreliable.

### III. Test Discipline

Every behaviour change or new check/notifier type MUST be covered by tests before the change
is considered complete. Tests are written to document behaviour and catch regressions, not as
an afterthought to satisfy a checklist.

**Unit Tests**:

- MUST cover: the happy path, failure conditions, retry behaviour, context cancellation, and
  assertion evaluation (for check types that support it).
- MUST NOT use third-party assertion libraries. Use standard Go `t.Fatalf`/`t.Errorf` with
  messages that make the failure self-explanatory without looking at the test code.
- All external dependencies MUST be replaced with in-process stubs: `httptest.NewServer` for
  HTTP, `bufconn` for gRPC, injected mock readers for Kafka. Tests that require a running
  external service are integration tests, not unit tests.
- Test loggers MUST discard output (`io.Discard`) to keep test runs clean.
- Test function names MUST follow `Test<Type>_<Scenario>` so failures identify both the
  component and the condition without reading the test body.
- Test helpers and fixture builders MUST appear at the top of the file, before any `Test*`
  function.

**Integration Tests**:

- MUST cover full end-to-end flows through real scheduler ticks, not just individual functions.
- MUST use in-process Flask, gRPC, and Kafka stubs within Docker Compose. Mocking at the
  transport layer is not acceptable in integration tests.
- Require Docker and MUST be explicitly approved before running. They affect external ports
  and system resources.
- When an integration test regresses unexpectedly, stop and diagnose before fixing. Patching
  a symptom without understanding the cause risks hiding a deeper bug.

**Rationale**: Fast unit tests catch logical errors without external dependencies. Integration
tests catch wiring bugs and configuration assumptions that unit tests cannot observe. Both
layers are necessary — neither substitutes for the other.

### IV. Explicit Concurrency and Lifecycle

WatchDawg runs long-lived background goroutines and must shut down cleanly under signal.
These properties require deliberate design, not ad-hoc goroutine spawning.

- All I/O and blocking operations MUST accept and honour `context.Context`. Cancellation must
  propagate to in-flight requests, consumer loops, and gRPC calls alike.
- Background goroutines (e.g., Kafka consumers) MUST receive the scheduler's root context so
  that a single cancellation tears everything down.
- Panic recovery MUST be implemented wherever a goroutine must survive a panic: Kafka consumer
  goroutines restart if context is still active; cron jobs continue via `cron.Recover`.
  Panics MUST be logged at Error level with full context.
- **Graceful shutdown sequence is fixed**: cancel root context → drain cron jobs → call
  `Cleanup()` on all checkers → close notifier. Reordering this risks resource leaks or
  premature teardown while in-flight checks are still running.
- Shared mutable state MUST be protected. New code introducing shared state MUST document the
  protection mechanism (mutex, atomic, channel) at the declaration site.

**Rationale**: Goroutine leaks silently accumulate. Unclear shutdown order causes panics or
stuck processes under signal. Explicit contracts prevent both.

### V. Resource Efficiency

WatchDawg may run for weeks or months. Resources allocated at startup MUST not grow
unboundedly, and connections MUST be reused.

- Long-lived I/O resources (HTTP clients, Kafka readers, Kafka writers) MUST be created once
  and reused. Creating a new connection per check execution is a defect, not a style choice.
- Kafka writers MUST be pooled by `(brokers, topic)` key. A new writer for the same target
  is a connection leak.
- HTTP response bodies MUST be explicitly closed with `defer resp.Body.Close()`. Failing to
  close leaks file descriptors.
- The per-check timeout context (default 30s) covers all retry attempts for one execution.
  New check types MUST not create their own unbounded blocking operations outside this timeout.
- Retry sleep between failed attempts is fixed at 1s. This is a conscious simplicity choice.
  Do not introduce configurable backoff without explicit product justification.

**Rationale**: Resource leaks in a long-running daemon compound over time and cause
incidents. Explicit resource management policies prevent gradual degradation.

### VI. Minimal Footprint

WatchDawg is a daemon, not a platform. Every new capability must justify why it belongs here
rather than in a general-purpose monitoring tool.

- The daemon MUST remain a single binary with no persistent storage, no UI, and no HTTP API
  server. Adding any of these requires revisiting the product scope, not just a PR.
- Standard library MUST be preferred for utility code. New third-party dependencies require
  explicit justification: what capability gap exists that the stdlib or existing deps cannot
  fill?
- New check types MUST correspond to a protocol-level health check (HTTP, gRPC, Kafka,
  Starlark). General-purpose scripting or business logic belongs in Starlark assertions, not
  in a new checker type.
- Config schema additions MUST be additive and backward-compatible. Removing or renaming a
  field is a breaking change requiring explicit migration guidance.

**Rationale**: Complexity grows faster than value in a monitoring daemon. A narrow scope
keeps the binary auditable, deployable anywhere, and easy to reason about.

## Technology Stack

- **Language**: Go 1.24+ (`internal/` packages only; no public API surface)
- **Scheduling**: `github.com/robfig/cron/v3` — 6-field cron with seconds precision
- **Kafka**: `github.com/segmentio/kafka-go` — consumer and producer
- **Starlark**: `go.starlark.net` — embedded scripting for health check assertions
- **gRPC**: `google.golang.org/grpc` + `google.golang.org/grpc/health/grpc_health_v1`
- **Metrics**: `github.com/prometheus/client_golang` — Prometheus exposition only
- **Logging**: `log/slog` (stdlib)

Package structure: `cmd/watchdawg` (entry point), `internal/config`, `internal/models`,
`internal/healthcheck`, `internal/starlarkeval`, `internal/metrics`. New packages require
a clear domain reason and MUST live under `internal/`.

## Development Workflow

**Build and run**:

```bash
go build -o bin/watchdawg ./cmd/watchdawg
./bin/watchdawg -config configs/config.json
LOG_FORMAT=json ./bin/watchdawg -config configs/config.json
cat config.json | ./bin/watchdawg -config -
```

**Validation order** (MUST follow this sequence):

1. `go test ./...` — always first, before any integration work
2. If adding a new check type: confirm Principle II metrics and Principle III unit tests complete
3. If changing config schema: confirm `config/loader.go` validation and test coverage updated
4. Integration tests only after unit tests pass, and only with explicit approval

**Adding a new check type checklist**:

1. Config struct in `internal/models/config.go`
2. Validation in `internal/config/loader.go` (exactly-one enforcement, required fields, defaults)
3. Checker implementation in `internal/healthcheck/<type>.go`
4. Registered in checkers slice in `internal/healthcheck/scheduler.go`
5. Unit tests in `internal/healthcheck/<type>_test.go`
6. Example in `configs/config.example.json`
7. Spec in `specs/feature-<type>-check.md`

**Adding a new hook type checklist**:

1. Config struct in `internal/models/config.go`
2. Validation in `internal/config/loader.go`
3. Dispatch case in `internal/healthcheck/hooks.go`
4. Unit tests in `internal/healthcheck/hooks_test.go`

## Governance

This constitution supersedes all other informal conventions. It is the binding technical
contract for WatchDawg contributions.

**Amendment procedure**:

1. Identify the principle(s) requiring change.
2. Determine version bump: PATCH (wording/clarification), MINOR (new principle or section),
   MAJOR (principle removed or incompatibly redefined).
3. Update this file, increment `CONSTITUTION_VERSION`, update `LAST_AMENDED_DATE`.
4. Update the Sync Impact Report comment at the top of this file.
5. Propagate to dependent templates (plan, spec, tasks) if governance structure changes.
6. Update `.claude/CLAUDE.md` if any workflow guidance changes.

**Compliance on every PR**:

- Config changes: enumerated, path-scoped error messages present (Principle I)
- New log lines: correct level, `check` key present (Principle II)
- New metrics: both `MetricsServer` and `NoopMetricsRecorder` updated (Principle II)
- Tests: `go test ./...` passes; new behaviour covered (Principle III)
- New goroutines: context respected, panic recovery in place (Principle IV)
- New connections: reuse and close strategy documented (Principle V)
- New deps: justification present in PR description (Principle VI)

**Versioning policy**:

- MAJOR: removing or redefining an existing principle; breaking config schema change
- MINOR: adding a new principle, new section, materially expanded guidance
- PATCH: clarifying wording, typo fixes, non-semantic additions

Runtime development guidance: `.claude/CLAUDE.md`

**Version**: 1.0.0 | **Ratified**: 2026-03-06 | **Last Amended**: 2026-03-06
