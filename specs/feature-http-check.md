# HTTP Check

An HTTP health check executes an HTTP request on a schedule, validates the
response against configurable expectations, and returns a structured result
indicating whether the endpoint is healthy.

---

## Configuration

```json
{
  "name": "api-health",
  "schedule": "30s",
  "retries": 2,
  "timeout": 5000000000,
  "http": {
    "url": "https://api.example.com/health",
    "method": "GET",
    "headers": { "Authorization": "Bearer token" },
    "body": "",
    "expected": {
      "status_code": 200,
      "format": "json",
      "headers": { "Content-Type": "application/json" },
      "verify_tls": true
    },
    "assertion": "result['status'] == 'ok'"
  }
}
```

| Field              | Required | Description |
|--------------------|----------|-------------|
| `name`             | yes      | Unique check identifier. |
| `schedule`         | yes      | Interval (`"30s"`, `"5m"`, `"1h"`) or cron expression. |
| `retries`          | no       | Additional attempts after the first (default: 0). |
| `timeout`          | no       | Deadline for the entire execution including retries. |
| `http.url`         | yes      | Target URL. |
| `http.method`      | yes      | HTTP method. |
| `http.headers`     | no       | Request headers. |
| `http.body`        | no       | Request body. Omitted when empty. |
| `expected.status_code` | no   | Acceptable status code(s): a single integer or an array. Defaults to any 2xx. |
| `expected.format`  | no       | Parse the response body as `"json"` or `"xml"` to make it available in assertions. |
| `expected.headers` | no       | Response headers that must be present with exact values. |
| `expected.verify_tls` | no    | Set to `false` to accept self-signed/invalid TLS certificates. Default: `true`. |
| `assertion`        | no       | Starlark expression or script for custom validation (see below). |

---

## Execution

On each scheduled tick, the check runs with a total attempt count of
`retries + 1`. The execution deadline (from `timeout`) spans all attempts.

The check returns as soon as an attempt succeeds. Between failed attempts there
is a fixed pause. The result's `duration` reflects total elapsed time across all
attempts and pauses.

Each attempt produces an independent result capturing its 1-based attempt
number and a timestamp. The attempt number is included in the final result so
callers can tell whether success or failure came from a retry.

Any failure at the network or protocol level (failed to connect, failed to read
body, etc.) is treated as unhealthy and the infrastructure error is surfaced in
the result separately from validation failures.

---

## Validation

Validation runs in order. The first failure short-circuits: remaining checks are
skipped and the result is returned immediately as unhealthy.

### 1. Status code

If one or more acceptable status codes are configured, the response status must
match at least one of them. If none are configured, any 2xx status is
acceptable.

### 2. Response headers

If expected headers are configured, every listed header must be present in the
response with an exact value match.

When a response header has multiple values, only the first value is considered.

### 3. Assertion

If an assertion is configured, it runs after the above checks pass. See
[feature-assertions.md](feature-assertions.md) for expression syntax and result
extraction rules.

Variables available in HTTP assertions:

| Variable      | Type   | Value |
|---------------|--------|-------|
| `status_code` | int    | Response status code |
| `body`        | string | Raw response body |
| `body_size`   | int    | Length of body in bytes |
| `headers`     | dict   | Response headers (first value per key) |
| `result`      | dict / list / scalar | Parsed body — only present when `expected.format` is set |

---

## Result

| Field      | Description |
|------------|-------------|
| `healthy`  | Whether the check passed. |
| `message`  | Human-readable explanation of the outcome. |
| `duration` | Total elapsed milliseconds across all attempts. |
| `attempt`  | 1-based number of the attempt that produced this result. |
| `error`    | Set on infrastructure failures (network, parse errors). Empty for clean validation failures. |
| `http`     | HTTP-specific details: status code, headers, body, body size. Present whenever the response was successfully received, even if validation failed. |

---

## Examples

### Minimal — any 2xx is healthy

```json
{ "url": "https://api.example.com/health", "method": "GET", "expected": {} }
```

### Accept multiple status codes

```json
"expected": { "status_code": [200, 201, 202] }
```

### Assertion examples

See [feature-assertions.md](feature-assertions.md). HTTP-specific variables
(`status_code`, `body`, `headers`, `result`) are available.

### Skip TLS verification

```json
"expected": { "verify_tls": false }
```
