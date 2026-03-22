# Data Model: Starlark HTTP Client

**Feature**: 008-starlark-http-client

---

## Config Changes

### `StarlarkCheckConfig` (existing, extended)

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `script` | string | yes | — | Starlark script body |
| `globals` | map[string]any | no | `{}` | Key-value pairs injected as Starlark globals |
| `max_body_bytes` | int | no | `10485760` (10 MB) | Maximum response body size for HTTP calls made from the script. Bodies exceeding this limit are truncated. |

**Validation rules**:
- `max_body_bytes` MUST be ≥ 1 if specified; values of 0 or negative are rejected with a path-scoped error.
- Default applied silently per Constitution Principle I.

---

## Runtime Values (Starlark)

### `http_request` Return Dict

The `http_request` builtin always returns a Starlark dict with the following fields:

| Field | Starlark Type | Description |
|-------|---------------|-------------|
| `status_code` | `int` | HTTP response status code. `0` if the request could not be completed. |
| `headers` | `dict[str, str]` | Response headers. Empty dict on error. Multi-value headers are joined with `, `. |
| `body` | `str` | Response body as a UTF-8 string. Truncated at `max_body_bytes` if larger. Empty string on error. |
| `error` | `str \| None` | `None` on success. Human-readable error description on network failure, timeout, or oversized body. |

### `http_request` Call Signature

```python
http_request(url, method="GET", body=None, headers=None)
```

| Parameter | Starlark Type | Required | Default | Description |
|-----------|---------------|----------|---------|-------------|
| `url` | `str` | yes | — | Full URL including scheme. Must be a valid HTTP or HTTPS URL. |
| `method` | `str` | no | `"GET"` | HTTP method. Case-insensitive. Supported: GET, POST, PUT, DELETE, PATCH, HEAD. |
| `body` | `str \| None` | no | `None` | Request body. Sent as-is. Ignored for HEAD and GET if not provided. |
| `headers` | `dict[str, str] \| None` | no | `None` | Additional request headers. Keys and values must be strings. |

**Error conditions** (set `error` field, do not raise):
- URL is not a valid HTTP/HTTPS URL
- Network connection failure
- DNS resolution failure
- Check timeout elapsed before response received
- Response body exceeds `max_body_bytes` (body is truncated; `error` describes the truncation)

**Raises** (hard script failure — not catchable in Starlark):
- `method` is not a string
- `headers` is not a dict or None
- `url` is not a string

---

## Unchanged Entities

All existing Starlark globals (`status_code`, `body`, `headers`, `result`, `value`, `key`, `struct`) remain unchanged. `http_request` is additive.
