# Contract: Starlark HTTP API

**Feature**: 008-starlark-http-client
**Audience**: Operators writing Starlark health check scripts and assertions

This document is the stable interface contract for the `http_request` builtin function available in all Starlark execution contexts within Watchdawg.

---

## Function Reference

### `http_request`

Makes an outbound HTTP request and returns the response.

**Signature**:
```python
response = http_request(url, method="GET", body=None, headers=None)
```

**Parameters**:

| Name | Type | Default | Description |
|------|------|---------|-------------|
| `url` | `str` | required | Fully-qualified URL (`http://` or `https://`) |
| `method` | `str` | `"GET"` | HTTP method (GET, POST, PUT, DELETE, PATCH, HEAD) |
| `body` | `str \| None` | `None` | Request body string |
| `headers` | `dict[str,str] \| None` | `None` | Additional request headers |

**Return value** — always a `dict` with these fields:

| Field | Type | Description |
|-------|------|-------------|
| `status_code` | `int` | HTTP status code (e.g. 200, 404). `0` on network failure. |
| `headers` | `dict[str,str]` | Response headers. Multi-value headers joined with `, `. |
| `body` | `str` | Response body. Truncated if larger than `max_body_bytes`. |
| `error` | `str \| None` | `None` on success; error description on any failure. |

**Timeout**: The request is cancelled when the enclosing check's configured timeout elapses. The `error` field will indicate timeout.

---

## Usage Examples

### Basic GET

```python
def check():
    resp = http_request("https://example.com/health")
    if resp.error != None:
        return {"healthy": False, "message": "request failed: " + resp.error}
    return {"healthy": resp.status_code == 200}
```

### POST with JSON body and headers

```python
def check():
    resp = http_request(
        "https://api.example.com/verify",
        method = "POST",
        body    = '{"key": "value"}',
        headers = {"Content-Type": "application/json", "X-Token": token},
    )
    if resp.error != None:
        return {"healthy": False, "message": resp.error}
    return {"healthy": resp.status_code == 200, "message": resp.body}
```

### In an HTTP assertion (checking a side effect)

```python
# Assertion script — primary request already completed; status_code, body etc. are in scope
side = http_request("https://audit.example.com/latest")
if side.error != None:
    fail("audit check failed: " + side.error)
valid = status_code == 200 and side.status_code == 200
```

### Inspecting response headers

```python
def check():
    resp = http_request("https://cdn.example.com/asset")
    if resp.error != None:
        return {"healthy": False, "message": resp.error}
    ct = resp.headers.get("Content-Type", "")
    return {"healthy": ct.startswith("application/json"), "message": "Content-Type: " + ct}
```

---

## Availability

`http_request` is available in:
- **Starlark check scripts** (`type: starlark`) — via the `script` field
- **HTTP check assertion scripts** — via the `assertion` field on an HTTP check
- **Kafka check assertion scripts** — via the `assertion` field on a Kafka check

---

## Constraints

- The request URL must use `http://` or `https://` scheme. Other schemes (e.g. `file://`, `ftp://`) are not supported.
- Request execution is bounded by the enclosing check's `timeout`. There is no separate per-request timeout.
- Response bodies larger than `max_body_bytes` (default 10 MB; configurable on Starlark checks) are truncated and `error` is set.
- Redirects are followed automatically (up to 10 redirects).
- There is no URL allowlist or denylist. Operators are responsible for the URLs their scripts call.

---

## Backward Compatibility

`http_request` is a new addition. Existing scripts that do not reference it are unaffected. The name `http_request` is now reserved as a global in all Starlark execution contexts — scripts that define a variable or function named `http_request` will shadow the builtin.
