# Implementation Plan: Starlark HTTP Client

**Branch**: `008-starlark-http-client` | **Date**: 2026-03-14 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/008-starlark-http-client/spec.md`

## Summary

Add an `http_request` builtin function to the Starlark execution environment so that operators can make outbound HTTP calls from within health-check scripts and assertion scripts. The builtin is injected in `internal/starlarkeval` — the shared execution layer — making it available in all Starlark contexts (pure Starlark checks, HTTP assertions, Kafka assertions) without changes to individual checker callers beyond passing the reusable `*http.Client`. A single `StarlarkChecker` instance holds one `*http.Client`; `HTTPChecker` reuses its existing client. Errors are returned as a structured response dict rather than raising, giving scripts visibility into failures without crashing. Response bodies are capped at a configurable limit (default 10 MB) to prevent unbounded memory use. No new third-party dependencies.

## Technical Context

**Language/Version**: Go 1.24+
**Primary Dependencies**: `go.starlark.net` (existing), stdlib `net/http` (existing)
**Storage**: N/A
**Testing**: `go test ./...`; `httptest.NewServer` for HTTP stubs
**Target Platform**: Linux/macOS server daemon (single binary)
**Project Type**: CLI daemon
**Performance Goals**: No measurable overhead beyond network round-trip for script-initiated HTTP calls
**Constraints**: All HTTP calls respect per-check timeout context; response bodies capped at 10 MB by default; no new third-party dependencies
**Scale/Scope**: Affects all Starlark execution contexts (StarlarkChecker + assertion scripts); single binary

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Gate | Principle | Status |
|------|-----------|--------|
| Config errors are enumerated and path-scoped (index + check name + field) | I. Operator-First Configuration | ✅ `max_body_bytes` validation emits path-scoped error matching existing patterns |
| Every new config field has a default or clear required-field validation | I. Operator-First Configuration | ✅ `max_body_bytes` defaults to 10 MB silently; ≤0 rejected with clear message |
| New log lines use correct slog level and include `check` key | II. Structured Observability | ✅ HTTP errors inside builtin logged at Debug with `check` key from thread name; no new Info/Warn lines needed |
| New check/hook type adds metrics to MetricsRecorder + both implementations | II. Structured Observability | ✅ Not a new check type; existing StarlarkChecker metrics cover it |
| Unit tests cover: success, failure, retry, timeout, assertions (where applicable) | III. Test Discipline | ✅ Planned: success GET/POST, network error, timeout, truncation, bad URL, header pass-through |
| External deps replaced with in-process stubs in unit tests | III. Test Discipline | ✅ `httptest.NewServer` for all HTTP targets |
| All new I/O operations accept and respect context.Context | IV. Explicit Concurrency | ✅ `http.NewRequestWithContext(ctx, ...)` used in builtin; request cancelled on timeout |
| New goroutines have panic recovery; shutdown cleanup is implemented | IV. Explicit Concurrency | ✅ No new goroutines; HTTP call is synchronous within the Starlark thread |
| New connections are pooled/reused; response bodies are explicitly closed | V. Resource Efficiency | ✅ Single `*http.Client` per StarlarkChecker; `defer resp.Body.Close()` in builtin |
| No new third-party deps without PR justification | VI. Minimal Footprint | ✅ Stdlib `net/http` only; no new deps |

## Project Structure

### Documentation (this feature)

```text
specs/008-starlark-http-client/
├── plan.md              # This file
├── research.md          # Phase 0 — resolved design decisions
├── data-model.md        # Phase 1 — config changes + Starlark runtime values
├── quickstart.md        # Phase 1 — operator usage guide
├── contracts/
│   └── starlark-http-api.md   # Phase 1 — Starlark function contract
└── tasks.md             # Phase 2 output (/speckit.tasks — not created by /speckit.plan)
```

### Source Code (repository root)

```text
internal/
├── models/
│   └── config.go              # Add max_body_bytes to StarlarkCheckConfig
├── config/
│   └── loader.go              # Validate + default max_body_bytes
├── starlarkeval/
│   ├── eval.go                # Inject http_request builtin into globals in Run*Script functions
│   └── http_client.go         # NEW — NewHTTPRequestBuiltin(ctx, client, maxBodyBytes)
├── healthcheck/
│   ├── starlark.go            # Add *http.Client field; pass to starlarkeval
│   ├── starlark_test.go       # Expand: HTTP success, failure, timeout, truncation tests
│   └── http.go                # Pass existing *http.Client to assertion eval calls

configs/
└── config.example.json        # Add max_body_bytes example on starlark check

integration-tests/
└── tests/
    └── test_starlark_http.py  # NEW — end-to-end: script calls Flask stub via http_request
```

**Structure Decision**: Single-project layout (existing). Changes are isolated to the `starlarkeval` package (new builtin) and minimal wiring in `starlark.go` and `http.go`. No new packages required.

## Complexity Tracking

> No Constitution violations. Table not needed.
