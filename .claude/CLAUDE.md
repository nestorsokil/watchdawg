# WatchDawg - Claude Context

## What This Is

WatchDawg is a dynamic health-checking service written in Go. It runs scheduled health checks against HTTP endpoints or via Starlark scripts, then fires webhook notifications on success or failure. No HTTP server — it's a CLI daemon that reads a JSON config and runs until interrupted.

## Project Structure

```
cmd/watchdawg/main.go              # Entry point: parse -config flag, load config, run scheduler
internal/
  config/loader.go                 # JSON config loading + field validation
  models/config.go                 # All config structs (HealthCheck, HTTPCheckConfig, etc.)
  models/result.go                 # CheckResult struct
  healthcheck/
    scheduler.go                   # Cron-based scheduler, routes checks, fires webhooks
    http.go                        # HTTPChecker: executes HTTP checks + Starlark assertions
    starlark.go                    # StarlarkChecker: runs pure Starlark scripts
    webhook.go                     # WebhookNotifier: sends on_success / on_failure webhooks
    starlark_test.go               # 35+ unit tests for Starlark checker
  starlark/starlark_runner.go      # Unused helper utilities (commented out)
configs/
  config.json                      # Default runtime config
  config.example.json              # Full reference config showing all features
  config.integration.json          # Integration test config (nginx check)
integration-tests/                 # Python/pytest integration tests (Docker Compose based)
```

## Build & Run

```bash
go build -o bin/watchdawg ./cmd/watchdawg
./bin/watchdawg -config configs/config.json
```

## Testing

```bash
go test ./...                      # Unit tests (starlark_test.go)
# Integration tests require Docker:
docker-compose up -d
cd integration-tests && pytest tests/
```

## Key Dependencies

- `github.com/robfig/cron/v3` — cron scheduler with seconds precision
- `go.starlark.net` — Starlark interpreter for validation scripts
- Go 1.25

## Architecture

1. `main.go` loads config → creates `Scheduler` → adds all checks → blocks on SIGTERM/SIGINT
2. `Scheduler` uses `robfig/cron` with seconds precision. Converts user schedules (`30s`, `5m`, `1h`, or cron syntax) to 6-field cron expressions
3. On each tick, `Scheduler.executeHealthCheck` calls `HTTPChecker` or `StarlarkChecker`, logs result, and fires webhooks via `WebhookNotifier`
4. `HTTPChecker` optionally runs a Starlark assertion against the response (variables: `status_code`, `body`, `headers`, `result` if format is json/xml)
5. `StarlarkChecker` executes scripts directly — calls `check()` function if defined, otherwise reads global `healthy`/`result`/`message` variables


## Check Types: Planned but Unimplemented

`grpc` and `kafka` are defined as `CheckType` constants but have no checker implementations. Adding them requires a new checker struct and a case in `scheduler.go:executeHealthCheck`.

## Guidelines

### Testing
- Write unit tests for new isolated functionality
- Write integration tests for complete features
- Integration tests should be high-level, short, easy to write and read. Add helper functions, new abstractions if needed to make current and future tests better
- Run unit tests first when validating a change
- If tests fail after a change, diagnose whether it's a direct consequence of the structural/behavioral change or an unintended regression. For regressions, explain the connection to the logic and stop for my input before proceeding.
- Always ask me before running any command that touches external systems (integration tests, docker-compose, migrations, etc.)

### Code Quality
- Name functions and data structures self-descriptively
- Use comprehensive variable names except for absolutely obvious cases (e.g. i == index)
- Reserve comments for non-obvious logic; explain WHY not HOW
- Document only major stable components with external dependants
- Never break existing tests when making changes
- Always ask me before deleting files or doing large refactors

### Logging & Error Handling
- Add logging at service boundaries, significant state transitions, and error paths; don't overuse DEBUG
- Log lines should include as much context as possible
- Errors should be logged exactly once, at the point where they are handled and not propagated further; if you only propagate an error, do not log it

### Workflow
- If a task involved a repeatable multi-step workflow (3+ steps, likely to recur), suggest turning it into a Skill
- If you introduced new patterns or conventions not yet in CLAUDE.md, suggest updating it
- If your change will break existing API, dependents etc, highlight this with CAPS in plan and/or output