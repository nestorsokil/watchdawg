# Feature Specification: HTTP Health Check

**Feature Branch**: `001-http-check`
**Created**: 2026-03-06
**Status**: Implemented

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Basic Endpoint Monitoring (Priority: P1)

An operator wants to verify that an HTTP endpoint is reachable and returning an expected status code on a schedule. This is the core use case: know when a service is down.

**Why this priority**: Without basic reachability checking, the feature has no value. All other stories build on this.

**Independent Test**: Configure a check with a URL and schedule. Verify it reports healthy when the endpoint returns 2xx, and unhealthy when it doesn't respond or returns an error.

**Acceptance Scenarios**:

1. **Given** an HTTP check configured with a URL and schedule, **When** the endpoint returns a 2xx status, **Then** the check result is healthy
2. **Given** an HTTP check configured with a URL and schedule, **When** the endpoint is unreachable or times out, **Then** the check result is unhealthy and the error is surfaced in the result
3. **Given** an HTTP check with `retries: 2`, **When** the first two attempts fail but the third succeeds, **Then** the result is healthy and `attempt` reflects the successful attempt number
4. **Given** an HTTP check with `retries: 2`, **When** all three attempts fail, **Then** the result is unhealthy and `attempt` is 3
5. **Given** a check with `expected.status_code: [200, 201]`, **When** the endpoint returns 201, **Then** the check is healthy
6. **Given** a check with `expected.status_code: 200`, **When** the endpoint returns 201, **Then** the check is unhealthy

---

### User Story 2 - Response Header Validation (Priority: P2)

An operator wants to verify that the response includes specific headers with exact values, not just that the endpoint is reachable.

**Why this priority**: Header validation adds a common but secondary layer of correctness checking. Useful for content negotiation and security headers.

**Independent Test**: Configure `expected.headers` with a known header. Verify the check fails when the header is absent or has a different value.

**Acceptance Scenarios**:

1. **Given** a check with `expected.headers: {"Content-Type": "application/json"}`, **When** the response includes that header with that exact value, **Then** the check is healthy
2. **Given** a check with `expected.headers: {"Content-Type": "application/json"}`, **When** the response omits the header, **Then** the check is unhealthy
3. **Given** a response header with multiple values, **When** the check validates it, **Then** only the first value is compared

---

### User Story 3 - Response Body Assertion (Priority: P3)

An operator wants to validate response body content beyond status codes, using a Starlark expression or script for custom logic.

**Why this priority**: Body assertions are the most expressive validation mechanism but require the simpler checks (status, headers) to already work.

**Independent Test**: Configure an assertion against a known JSON response. Verify the check passes when the assertion returns true and fails when it returns false.

**Acceptance Scenarios**:

1. **Given** `assertion: "status_code == 200 and 'ok' in body"`, **When** the response matches, **Then** the check is healthy
2. **Given** `format: "json"` and an assertion referencing `result`, **When** the body parses and the assertion passes, **Then** the check is healthy
3. **Given** `format: "json"` and a response that is not valid JSON, **When** the check runs, **Then** the check is unhealthy before the assertion runs
4. **Given** a failing assertion, **When** the check runs, **Then** the result is unhealthy; status code and header checks are not re-run
5. **Given** an assertion that crashes (script error), **When** the check runs, **Then** the check is unhealthy and the error is surfaced separately

---

### Edge Cases

- What happens when the network connection is refused vs. times out? Both are unhealthy; the error field distinguishes them.
- What if `timeout` expires mid-retry? The check returns unhealthy immediately; remaining retries are not attempted.
- What if `expected.status_code` is an empty array? Treated the same as not configured — any 2xx is acceptable.
- What if a response header appears multiple times? Only the first value is used for comparison.
- What if `format` is set but the body is empty? Parsing fails; the check is unhealthy.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST execute an HTTP request to the configured URL on the configured schedule
- **FR-002**: System MUST support configurable HTTP method and request headers
- **FR-003**: System MUST retry failed attempts up to `retries` additional times with a fixed pause between attempts
- **FR-004**: System MUST enforce a `timeout` deadline spanning all attempts
- **FR-005**: System MUST validate the response status code against configured acceptable codes (defaulting to any 2xx)
- **FR-006**: System MUST validate configured response headers with exact value matching (first value only for multi-value headers)
- **FR-007**: System MUST run validation in order: status code → response headers → assertion; stopping at first failure
- **FR-008**: System MUST parse the response body as JSON or XML when `format` is configured, making it available as `result` in assertions
- **FR-009**: System MUST accept self-signed/invalid TLS certificates when `verify_tls: false` is set
- **FR-010**: System MUST include HTTP-specific details (status, headers, body, body size) in the result whenever a response was received, even on validation failure
- **FR-011**: System MUST surface infrastructure errors (network, parse) separately from validation failures in the result

### Key Entities

- **Check Result**: Outcome of one check execution — `healthy`, `message`, `duration`, `attempt`, `error`, `http` (status, headers, body, body_size)
- **Attempt**: A single HTTP request within a retry cycle; identified by 1-based attempt number

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An operator can detect a down endpoint within one schedule interval
- **SC-002**: An operator can distinguish network failures from validation failures by inspecting the result
- **SC-003**: Retry behaviour allows transient failures to resolve without triggering false alarms
- **SC-004**: An operator can write a custom assertion that validates any aspect of the response body without modifying Watchdawg
