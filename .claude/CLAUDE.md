# Watchdawg - Claude Context

## Purpose

Watchdawg is a CLI health-checking daemon written in Go. It reads a JSON config file, runs scheduled health checks against external systems, and fires webhook notifications on success or failure. No HTTP server, no UI, no database — just a process that runs until interrupted.

Supported check types: HTTP, gRPC, Kafka, Starlark scripts.
Notification channels: HTTP webhooks, Kafka topics.
Metrics: optional Prometheus exposition.

Feature details and config semantics live in [./specs/](../specs/).

## Architecture

```
main.go
  └─ loads config
  └─ creates Scheduler
       └─ on each tick: run checker → record metrics → log → fire hooks in parallel
```

Config is loaded from a file or stdin. Environment variables are expanded before JSON parsing. Validation is strict: exactly one check type per check, required fields enforced, defaults applied.

Checks run on a cron schedule (duration strings like `30s`/`5m`, or cron expressions). Retries with 1s sleep between attempts. Per-check timeout context passed to all I/O. Hooks fire in parallel after each execution.

## Packages

| Package | Role |
|---|---|
| `cmd/watchdawg` | Entry point |
| `internal/config` | Config loading and validation |
| `internal/models` | Config and result types |
| `internal/healthcheck` | Scheduler, all checkers, hook dispatch, retry, metrics interface |
| `internal/starlarkeval` | Starlark execution utilities shared by checkers |
| `internal/metrics` | Prometheus metrics server |
| `configs/config.example.json` | Full config reference |

## Build & Run

```bash
go build -o bin/watchdawg ./cmd/watchdawg
./bin/watchdawg -config configs/config.json
LOG_FORMAT=json ./bin/watchdawg -config configs/config.json  # JSON logs
cat config.json | ./bin/watchdawg -config -                  # stdin
```

## Testing

```bash
go test ./...     # Unit tests — run first when validating any change
```

Integration tests require Docker — **always ask before running**:
```bash
docker-compose up -d
cd integration-tests && pytest tests/
```

Integration tests live in `integration-tests/tests/`. They use in-process Flask stubs (HTTP target, webhook receiver, gRPC stub, Kafka helpers) and pytest fixtures for setup/teardown.

## Non-Functional Requirements

- **Logging**: `log/slog`, text by default, JSON via `LOG_FORMAT=json`. Log at service boundaries, state transitions, and error paths. Errors logged exactly once at the handling point — never log and re-propagate.
- **Panic recovery**: cron jobs wrapped with recovery (logs and continues). Kafka consumer goroutines recover and restart if context is still active.
- **Graceful shutdown**: SIGINT/SIGTERM cancels root context, waits for in-flight jobs, closes all resources.
- **Concurrency**: shared state is protected; hooks fire in parallel with WaitGroup.

## Guidelines

### Testing
- Unit tests for isolated logic; integration tests for complete end-to-end flows
- Integration tests should be high-level, short, and readable — add helpers/abstractions as needed
- Run unit tests first when validating a change
- If tests fail, diagnose whether it's a direct consequence of the change or a regression. For regressions, explain and stop for input before proceeding
- Always ask before running commands that touch external systems

### Code Quality
- Self-descriptive names for functions and types; comprehensive variable names except for trivial cases
- Comments explain WHY, not HOW; only for non-obvious logic
- Document only major stable components with external dependants
- Never break existing tests; always ask before deleting files or large refactors

### Logging & Error Handling
- Include as much context as possible in log lines
- Errors logged exactly once at the handling point; if only propagating, do not log

### Workflow
- If a task involved a repeatable multi-step workflow (3+ steps, likely to recur), suggest turning it into a Skill
- If a change introduces new patterns not yet documented, suggest updating CLAUDE.md or the relevant spec
- If a change will break existing API or dependants, highlight this in CAPS in the plan and output
