# Feature Specification: Starlark HTTP Client

**Feature Branch**: `008-starlark-http-client`
**Created**: 2026-03-10
**Status**: Draft
**Input**: User description: "let's allow using http client in starlark"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Outbound HTTP Call in a Starlark Health Check (Priority: P1)

A Watchdawg operator writes a pure Starlark health check that calls an external HTTP endpoint and evaluates the response. For example, a script that queries a status API, checks the JSON body for a specific field value, and marks the check as healthy or unhealthy based on the result.

**Why this priority**: This is the core use case. It enables composite and conditional health checks that are not possible with the built-in HTTP checker alone, and directly delivers the requested capability.

**Independent Test**: Can be fully tested by configuring a Starlark check whose script uses the HTTP client to call a local test server, then asserting that the check result reflects the HTTP response content.

**Acceptance Scenarios**:

1. **Given** a Starlark check script that calls an HTTP GET endpoint, **When** the endpoint returns a 200 response, **Then** the script receives the response status code, headers, and body and can use them in its logic.
2. **Given** a Starlark check script that calls an HTTP GET endpoint, **When** the endpoint is unreachable or times out, **Then** the script receives an error value it can inspect or propagate, and the check is marked as failed.
3. **Given** a Starlark check script that calls an HTTP POST endpoint with a request body and custom headers, **When** the check runs, **Then** the server receives the correct method, body, and headers.

---

### User Story 2 - HTTP Client in an HTTP Check Assertion Script (Priority: P2)

A Watchdawg operator writes a Starlark assertion script attached to an existing HTTP check. After the primary HTTP request completes, the assertion script uses the HTTP client to call a secondary endpoint to verify a side effect — for example, that a cache was updated or an audit log was written.

**Why this priority**: Extends the HTTP client capability to the assertion context, enabling end-to-end validation flows that span multiple services without requiring a separate Starlark check.

**Independent Test**: Can be fully tested by configuring an HTTP check with an assertion script that makes a follow-up HTTP call and fails the check if the secondary response does not meet expectations.

**Acceptance Scenarios**:

1. **Given** an HTTP check with a Starlark assertion script, **When** the primary request succeeds and the assertion script makes a secondary HTTP call that returns the expected data, **Then** the overall check is marked as passed.
2. **Given** an HTTP check with a Starlark assertion script, **When** the secondary HTTP call in the assertion fails, **Then** the overall check is marked as failed with a descriptive error.

---

### User Story 3 - Timeout Propagation for Script-Initiated Requests (Priority: P3)

A Watchdawg operator configures a check with a short timeout. The Starlark script makes an HTTP call to a slow endpoint. The system cancels the HTTP call when the check's timeout elapses rather than allowing the script to hang indefinitely.

**Why this priority**: Correct timeout behavior is critical for system stability. A script that can block indefinitely would break the scheduler's timing guarantees and exhaust resources.

**Independent Test**: Can be fully tested by pointing a Starlark script's HTTP call at a deliberately slow endpoint with a tight check timeout and asserting that the check fails within the configured timeout window.

**Acceptance Scenarios**:

1. **Given** a check with a 2-second timeout and a Starlark script that calls an endpoint with a 10-second response delay, **When** the check runs, **Then** the HTTP call is cancelled and the check fails within 2 seconds.
2. **Given** the check timeout elapses while an HTTP call is in-flight, **Then** the script receives a timeout error value and no goroutines are leaked.

---

### Edge Cases

- What happens when the script makes an HTTP call with a malformed URL? The script receives an error value; no panic occurs.
- What happens when the script makes an HTTP call that results in a redirect? Redirects are followed automatically up to a reasonable limit; the final response is returned.
- What happens when the script issues multiple sequential HTTP calls? Each call runs normally; all respect the remaining check timeout.
- What happens when the response body is very large? The body is read fully but a reasonable size limit is enforced to prevent memory exhaustion.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Starlark scripts MUST be able to initiate outbound HTTP requests using at least GET and POST methods.
- **FR-002**: Starlark scripts MUST be able to specify the request URL, HTTP method, request headers, and request body for each HTTP call.
- **FR-003**: Starlark scripts MUST receive the HTTP response status code, response headers, and response body as accessible values.
- **FR-004**: All HTTP requests initiated from a Starlark script MUST be bounded by the enclosing check's configured timeout; the request MUST be cancelled when the timeout elapses.
- **FR-005**: When an HTTP request fails (network error, timeout, DNS failure), the script MUST receive a structured error value rather than crashing the check.
- **FR-006**: The HTTP client MUST be available in both pure Starlark checks and Starlark assertion scripts attached to HTTP checks.
- **FR-007**: HTTP redirects MUST be followed automatically, subject to the check timeout.
- **FR-008**: Response bodies MUST be capped at a configurable maximum size to prevent unbounded memory use; content beyond the limit MUST be truncated or result in an error.

### Assumptions

- All standard HTTP methods (GET, POST, PUT, DELETE, PATCH, HEAD) are supported, not just GET and POST; GET and POST are the minimum.
- No URL allowlist or denylist is applied at the daemon level; operators are responsible for what URLs their scripts call.
- The response body is returned as a string value in the script.
- Request and response headers are represented as key-value maps in the script.
- The default maximum response body size is 10 MB; this can be adjusted via configuration.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of existing Starlark check and assertion tests continue to pass after the feature is introduced (no regressions).
- **SC-002**: A Starlark script performing a single outbound HTTP call adds no more than the network round-trip time to the check's execution time (no measurable overhead from the scripting layer itself).
- **SC-003**: A check with a configured timeout of T seconds always terminates within T + 0.5 seconds even when the script's HTTP call targets an unresponsive host.
- **SC-004**: Operators can implement a composite health check spanning two HTTP endpoints using a single Starlark script with no additional tooling or configuration outside the script body.
