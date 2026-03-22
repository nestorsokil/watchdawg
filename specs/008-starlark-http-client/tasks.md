# Tasks: Starlark HTTP Client

**Input**: Design documents from `/specs/008-starlark-http-client/`
**Prerequisites**: plan.md ✅, spec.md ✅, research.md ✅, data-model.md ✅, contracts/ ✅

**Tests**: Unit tests are MANDATORY per the project constitution (Principle III — Test Discipline).
Integration test task included for end-to-end validation of the full scheduler flow.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1, US2, US3)
- Exact file paths included in all descriptions

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Stub the new file so all user story phases have a clear, conflict-free target to write to.

- [X] T001 Create stub `internal/starlarkeval/http_client.go` with package declaration and exported function signature `NewHTTPRequestBuiltin` (empty body, returns nil) to establish the compilation target

**Checkpoint**: Project compiles after T001.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Config changes and core builtin implementation that MUST be complete before any user story wiring can be done.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [X] T002 Add optional `MaxBodyBytes int` field (JSON: `max_body_bytes`) to `StarlarkCheckConfig` in `internal/models/config.go`
- [X] T003 Add validation and silent default (10 MB) for `max_body_bytes` in `internal/config/loader.go`; reject values ≤ 0 with a path-scoped error message matching the pattern `"invalid check at index %d (%s): max_body_bytes must be positive"`
- [X] T004 Implement `NewHTTPRequestBuiltin(ctx context.Context, client *http.Client, maxBodyBytes int) *starlark.Builtin` in `internal/starlarkeval/http_client.go`: accepts `url`, `method="GET"`, `body=None`, `headers=None`; returns `{status_code, headers, body, error}` dict; uses `http.NewRequestWithContext`; reads body via `io.LimitReader`; sets `error` field on any failure rather than raising
- [X] T005 [P] Update `configs/config.example.json` to add `"max_body_bytes": 1048576` on the starlark check example entry

**Checkpoint**: `go test ./...` passes. Foundation ready — user story wiring can now begin.

---

## Phase 3: User Story 1 — Outbound HTTP in StarlarkChecker (Priority: P1) 🎯 MVP

**Goal**: A pure Starlark check script can call `http_request(...)` to make outbound HTTP calls and act on the response.

**Independent Test**: Configure a Starlark check whose script uses `http_request` against a local `httptest.Server`; assert the check result reflects the HTTP response content.

> **Write tests FIRST — they must FAIL before T010/T011 are implemented.**

### Unit Tests for User Story 1

- [X] T006 Write unit tests in `internal/starlarkeval/http_client_test.go` covering: successful GET (200, headers forwarded, body returned, error=None), successful POST with body and custom headers, non-2xx status code (response returned, error=None), malformed URL (error field set, no panic), and unsupported scheme (error field set)

- [X] T007 Write unit test in `internal/starlarkeval/http_client_test.go` covering response body truncation: server returns body larger than maxBodyBytes; assert body is truncated and error field describes truncation

- [X] T008 Write unit test in `internal/healthcheck/starlark_test.go` covering `check()` function that calls `http_request` and returns healthy/unhealthy based on status_code; use `httptest.NewServer`

### Implementation for User Story 1

- [X] T009 Add `client *http.Client` field to `StarlarkChecker` struct and initialise it (single shared instance, standard transport) in `NewStarlarkChecker` in `internal/healthcheck/starlark.go`
- [X] T010 [US1] Update `starlarkeval.RunCheckScript` signature to accept `client *http.Client` and `maxBodyBytes int`; inject the builtin from `NewHTTPRequestBuiltin` into the globals dict before calling `starlark.ExecFile` in `internal/starlarkeval/eval.go`
- [X] T011 [US1] Update `StarlarkChecker.executeOnce` to pass `check.Starlark.MaxBodyBytes` (falling back to 10 MB default) and `s.client` when calling `starlarkeval.RunCheckScript` in `internal/healthcheck/starlark.go`

**Checkpoint**: `go test ./...` passes. A Starlark check using `http_request` is fully functional.

---

## Phase 4: User Story 2 — HTTP Client in Assertion Scripts (Priority: P2)

**Goal**: Starlark assertion scripts attached to HTTP checks (and Kafka checks) can call `http_request(...)` for follow-up requests.

**Independent Test**: Configure an HTTP check with an assertion script that makes a secondary HTTP call via `http_request`; assert the overall check result reflects both the primary response and the secondary call outcome.

> **Write tests FIRST — they must FAIL before T013/T014 are implemented.**

### Unit Tests for User Story 2

- [X] T012 [P] [US2] Write unit tests in `internal/healthcheck/http_test.go` (or `internal/healthcheck/http_assertion_test.go` if file does not exist) covering: assertion script calls `http_request` successfully → check passes; assertion script's `http_request` returns error → check fails with descriptive message; use `httptest.NewServer` for both primary and secondary endpoints

### Implementation for User Story 2

- [X] T013 [US2] Update `starlarkeval.RunAssertionScript` signature to accept `client *http.Client` and `maxBodyBytes int`; inject the `http_request` builtin into globals before calling `starlark.ExecFile` in `internal/starlarkeval/eval.go`
- [X] T014 [US2] Update `HTTPChecker.validateWithStarlark` to pass `h.client` and the 10 MB default (no per-assertion config) when calling `starlarkeval.RunAssertionScript` in `internal/healthcheck/http.go`
- [X] T015 [P] [US2] Update `KafkaChecker` assertion call site (if it calls `RunAssertionScript`) to pass client and maxBodyBytes in `internal/healthcheck/kafka.go`; use a newly-constructed default `*http.Client` stored on the checker struct

**Checkpoint**: `go test ./...` passes. Assertion scripts in HTTP and Kafka checks can use `http_request`.

---

## Phase 5: User Story 3 — Timeout Propagation (Priority: P3)

**Goal**: HTTP calls made from Starlark scripts are cancelled when the enclosing check's timeout elapses; no goroutine leaks.

**Independent Test**: Point a Starlark script's `http_request` at a deliberately slow `httptest.Server`; set a tight check timeout; assert the check fails within the timeout window.

> **Write tests FIRST — they must FAIL before implementation confirms correct timeout handling.**

### Unit Tests for User Story 3

- [X] T016 [US3] Add timeout unit test to `internal/starlarkeval/http_client_test.go`: construct a context with a 50 ms deadline; point `http_request` at a server that sleeps 500 ms; assert the returned dict has `status_code=0` and `error` describes the timeout; assert the test itself completes in under 200 ms

- [X] T017 [US3] Add context-already-cancelled test to `internal/starlarkeval/http_client_test.go`: call `NewHTTPRequestBuiltin` with an already-cancelled context; assert the request is not sent and `error` is set immediately

### Implementation for User Story 3

- [X] T018 [US3] Verify (and fix if needed) that `NewHTTPRequestBuiltin` uses `http.NewRequestWithContext(ctx, ...)` throughout — no fallback to `http.NewRequest` — in `internal/starlarkeval/http_client.go`; ensure `defer resp.Body.Close()` is present unconditionally after a successful `client.Do`

**Checkpoint**: `go test ./...` passes including timeout tests. Checks always terminate within their configured timeout even when scripts make slow HTTP calls.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Full-suite validation, documentation, and optional integration test.

- [X] T019 Run `go test ./...` and confirm zero failures and zero regressions across all packages
- [X] T020 [P] Verify `configs/config.example.json` has a starlark check entry demonstrating `max_body_bytes` and a multi-line script using `http_request` (update if T005 example is insufficient)
- [X] T021 [P] Add `http_request` to `specs/008-starlark-http-client/contracts/starlark-http-api.md` backward-compatibility note if any shadowing behaviour was discovered during implementation (no code change if already accurate)
- [X] T022 Write integration test `integration-tests/tests/test_starlark_http.py`: start a Flask stub, configure a Starlark check whose script calls `http_request` against the stub, run a scheduler tick, assert the check result is healthy; mark test with `@pytest.mark.integration` *(requires Docker — ask before running)*

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately
- **Foundational (Phase 2)**: Depends on Phase 1 — BLOCKS all user story phases
- **User Story Phases (3, 4, 5)**: All depend on Phase 2 completion; US1 (Phase 3) must complete before US2 (Phase 4) because `RunCheckScript` changes in eval.go are a model for the `RunAssertionScript` changes
- **Polish (Phase 6)**: Depends on all user story phases complete

### User Story Dependencies

- **US1 (P1)**: Depends on Foundational only — independent of US2/US3
- **US2 (P2)**: Depends on Foundational + US1 (same eval.go injection pattern established by US1)
- **US3 (P3)**: Depends on Foundational + US1 (tests call same builtin) — can be developed alongside US2

### Within Each User Story

- Unit tests written first (red phase) before implementation (green phase)
- T009 (add client to StarlarkChecker) before T011 (use client in execute)
- T010 (update RunCheckScript signature) before T011 (call with new args)
- T013 (update RunAssertionScript signature) before T014/T015 (call with new args)

### Parallel Opportunities

- T005 (config.example.json) can run in parallel with T002–T004
- T006, T007, T008 can run in parallel (different test functions in same file — sequential within file, parallelisable if split across files)
- T012 (US2 tests) and T016/T017 (US3 tests) can run in parallel with US1 implementation tasks in different files
- T020 and T021 (polish tasks) can run in parallel

---

## Parallel Example: User Story 1

```bash
# After Phase 2 is complete, launch US1 test-writing in parallel with config prep:
Task T006: "Write http_client unit tests (success/error/URL cases)"
Task T007: "Write body truncation unit test"
Task T008: "Write StarlarkChecker integration unit test with http_request"
# Then sequentially:
Task T009 → T010 → T011 (client field → signature update → wire up)
```

---

## Implementation Strategy

### MVP (User Story 1 Only)

1. Complete Phase 1: Setup (T001)
2. Complete Phase 2: Foundational (T002–T005)
3. Complete Phase 3: User Story 1 (T006–T011)
4. **STOP and VALIDATE**: `go test ./...` — pure Starlark checks can use `http_request`
5. Demo: a `type: starlark` check calling an external API

### Incremental Delivery

1. Setup + Foundational → compilation target ready
2. US1 complete → `type: starlark` checks have HTTP client (MVP!)
3. US2 complete → HTTP/Kafka assertion scripts also have `http_request`
4. US3 complete → timeout behaviour verified with dedicated tests
5. Polish → full suite green, integration test in place

---

## Notes

- [P] tasks touch different files and have no blocking dependency on each other
- [Story] labels map each task to the user story it delivers
- Constitution Principle III: tests are written FIRST (red) before implementation (green)
- Constitution Principle V: single `*http.Client` per checker — never construct inside the builtin
- Constitution Principle IV: `http.NewRequestWithContext` — no bare `http.NewRequest` calls
- Commit after each checkpoint (end of Phase 2, end of each user story phase)
- Do NOT run integration tests (T022) without explicit approval
