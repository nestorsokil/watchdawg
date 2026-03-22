# Research: Starlark HTTP Client

**Phase**: 0 — Resolved before design
**Feature**: 008-starlark-http-client

---

## Decision 1: Builtin Injection Point

**Decision**: Inject the `http_request` builtin inside `internal/starlarkeval` — specifically in `RunCheckScript` and `RunAssertionScript` — by adding it to the `globals` dict before calling `starlark.ExecFile`.

**Rationale**: `starlarkeval` is the shared execution environment for all Starlark contexts (StarlarkChecker and HTTPChecker assertions). Centralising the builtin here means a single implementation is automatically available everywhere without duplication or caller divergence.

**Alternatives considered**:
- Inject in each caller (starlark.go, http.go) — rejected: duplication, risk of divergence, callers should not know about builtin construction details.
- New package `internal/starlarkhttp` — rejected: overkill for a single builtin function; `starlarkeval` is the right home.

---

## Decision 2: HTTP Client Reuse Strategy

**Decision**: A single `*http.Client` is created once per `StarlarkChecker` instance and stored as a field. For HTTP check assertion scripts, the `HTTPChecker`'s existing client is reused. Both callers pass the client to the starlarkeval functions, which construct the builtin closure over it.

**Rationale**: Constitution Principle V requires long-lived I/O resources to be created once and reused; a new client per script execution is a connection leak. Storing the client on the checker follows the same pattern used by `HTTPChecker`.

**Alternatives considered**:
- Package-level global `*http.Client` — rejected: conflates concerns across checker instances, complicates testing.
- Constructing client inside the builtin on every call — rejected: violates Principle V.

---

## Decision 3: Error Surface in Starlark

**Decision**: `http_request` returns a dict with an `error` field (string on failure, `None` on success) rather than raising a Starlark error. The returned dict always has all four fields: `status_code`, `headers`, `body`, `error`.

**Rationale**: Starlark has no try/except construct. If the builtin raises on network failure, the entire script execution fails and the check is marked unhealthy with no ability to recover. Returning a structured error value (per spec FR-005) gives scripts the ability to inspect what went wrong and decide the outcome explicitly.

**Alternatives considered**:
- Raise Starlark error on failure — rejected: spec FR-005 explicitly requires a structured error value; Starlark's lack of exception handling makes raising unrecoverable for the script.

---

## Decision 4: Response Body Size Cap

**Decision**: Default cap of 10 MB enforced via `io.LimitReader` when reading the response body. Configurable per check via an optional `max_body_bytes` integer field on `StarlarkCheckConfig`. The same 10 MB default is applied to assertion scripts (no per-assertion config; the parent check config is not accessible in that context).

**Rationale**: Unbounded body reads violate Constitution Principle V (resource efficiency). 10 MB is generous for health-check responses. Per-check config follows the existing Watchdawg pattern of check-level configuration rather than global settings.

**Alternatives considered**:
- Global daemon-level limit — rejected: the existing pattern is per-check configuration; a global limit would require a new top-level config section.
- No limit — rejected: directly violates Principle V.

---

## Decision 5: No New Third-Party Dependencies

**Decision**: Implement using stdlib `net/http` exclusively. No new third-party packages.

**Rationale**: Constitution Principle VI requires justification for new dependencies. The stdlib HTTP client fully covers the required capabilities (all methods, headers, body, timeout via context, redirect following, connection pooling).

**Alternatives considered**:
- `resty`, `go-resty`, or similar HTTP client libraries — rejected: no capability gap; Principle VI prohibits unjustified additions.

---

## Decision 6: Context Threading in Builtin Closure

**Decision**: The `http_request` builtin is constructed as a closure that captures `ctx context.Context`. `RunCheckScript` and `RunAssertionScript` already receive `ctx`; they pass it to a `NewHTTPRequestBuiltin(ctx, client, maxBodyBytes)` constructor.

**Rationale**: Closures over context are idiomatic Go. The alternative (`thread.SetLocal`) adds Starlark-specific API complexity with no benefit.

**Alternatives considered**:
- `thread.SetLocal(contextKey, ctx)` inside the thread + `thread.Local(contextKey)` inside the builtin — rejected: more complex, non-obvious to future readers, and `SetLocal` requires type assertions on retrieval.

---

## Decision 7: Starlark API Shape

**Decision**: Single function `http_request(url, method="GET", body=None, headers=None)` returning a dict.

Return dict structure:
```
{
  "status_code": int,      # 0 if request could not be completed
  "headers":     dict,     # string → string, response headers
  "body":        string,   # response body (truncated at max_body_bytes)
  "error":       string|None  # None on success; error description on failure
}
```

**Rationale**: Single entry point is simpler to document and learn. `method` keyword arg with `"GET"` default makes common GET calls concise: `http_request("http://...")`. All four HTTP verbs needed by health checks (GET, POST, PUT, DELETE) work with one function.

**Alternatives considered**:
- Separate `http_get` / `http_post` — rejected: operators would need to know which function to use for each method; less discoverable for PUT/DELETE/PATCH.
- `http_request(method, url, ...)` positional method first — rejected: GET is overwhelmingly the most common case; requiring `method` positionally increases noise for 90% of scripts.

---

## Decision 8: Where `max_body_bytes` Lives in Config

**Decision**: Add optional `max_body_bytes` integer to `StarlarkCheckConfig` only. Assertion scripts (used inside HTTPCheck/KafkaCheck) use the hardcoded 10 MB default — they do not have a per-assertion config field because assertions are inline strings, not top-level check configs.

**Rationale**: Assertion scripts are part of `HTTPCheckConfig`/`KafkaCheckConfig`, which have no existing Starlark-specific section. Adding a Starlark-specific field to those configs would be confusing. The 10 MB default is adequate for virtually all assertion use cases.

**Alternatives considered**:
- Add `max_body_bytes` to `HTTPCheckConfig` — rejected: conflates HTTP check config with script execution config.
