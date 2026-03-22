# Quickstart: Starlark HTTP Client

**Feature**: 008-starlark-http-client
**Audience**: Operators configuring Watchdawg health checks

---

## What Changed

Starlark scripts (in `type: starlark` checks and in `assertion` fields) can now call `http_request(url, ...)` to make outbound HTTP requests. The response is returned as a dict with `status_code`, `headers`, `body`, and `error` fields.

---

## Minimal Example: Starlark Check Calling an HTTP Endpoint

```json
{
  "name": "downstream-health",
  "schedule": "30s",
  "type": "starlark",
  "starlark": {
    "script": "def check():\n  resp = http_request('http://downstream/health')\n  if resp.error != None:\n    return {'healthy': False, 'message': resp.error}\n  return {'healthy': resp.status_code == 200}"
  }
}
```

---

## Limiting Response Body Size

The default cap is 10 MB. To lower it for a check that expects small responses:

```json
{
  "name": "compact-status",
  "schedule": "1m",
  "type": "starlark",
  "starlark": {
    "max_body_bytes": 4096,
    "script": "def check():\n  resp = http_request('http://service/status')\n  return {'healthy': resp.error == None and resp.status_code == 200}"
  }
}
```

---

## In an HTTP Check Assertion

`http_request` is also available in assertion scripts attached to HTTP checks:

```json
{
  "name": "order-service",
  "schedule": "1m",
  "type": "http",
  "http": {
    "url": "http://orders/create",
    "method": "POST",
    "assertion": "side = http_request('http://audit/latest')\nvalid = status_code == 201 and side.status_code == 200"
  }
}
```

---

## Error Handling Pattern

Always check `resp.error` before using `resp.status_code`:

```python
def check():
    resp = http_request("http://api/ping", method="GET")
    if resp.error != None:
        return {"healthy": False, "message": "http_request failed: " + resp.error}
    if resp.status_code != 200:
        return {"healthy": False, "message": "unexpected status: " + str(resp.status_code)}
    return {"healthy": True}
```

---

## Constraints

- URL must start with `http://` or `https://`
- The request is cancelled if the check's `timeout` elapses
- Response bodies larger than `max_body_bytes` are truncated (`error` field describes this)
- No URL filtering — operators control what URLs scripts may call
