# Contract: History REST API

**Branch**: `007-execution-history` | **Phase**: 1
**Base URL**: `http://localhost:{metrics_port}` (localhost only; same port as Prometheus metrics)
**Format**: JSON (`Content-Type: application/json`)
**Auth**: None (localhost-only; see FR-011)

---

## Response Shape

Both endpoints return the same root object. The per-check endpoint is a subset of the
all-checks endpoint.

```json
{
  "checks": {
    "<check_name>": [
      {
        "timestamp": "2026-03-06T10:05:00Z",
        "healthy": true,
        "duration_ms": 42,
        "error": ""
      }
    ]
  }
}
```

`checks` is always present; it is an empty object `{}` when no history exists.
Each check's records are in reverse-chronological order (newest first).

---

## Endpoints

### GET /history/{check_name}

Returns execution records for a specific check.
The `checks` map contains exactly one key: `check_name`.

**Path parameters**:

| Parameter    | Type   | Required | Description                   |
|--------------|--------|----------|-------------------------------|
| `check_name` | string | yes      | Exact check name as in config |

**Query parameters**:

| Parameter | Type | Required | Default | Description                         |
|-----------|------|----------|---------|-------------------------------------|
| `limit`   | int  | no       | 100     | Maximum number of records to return |

**Responses**:

`200 OK` — check has recorded history:
```json
{
  "checks": {
    "api-health": [
      {
        "timestamp": "2026-03-06T10:05:00Z",
        "healthy": true,
        "duration_ms": 42,
        "error": ""
      },
      {
        "timestamp": "2026-03-06T10:04:30Z",
        "healthy": false,
        "duration_ms": 30001,
        "error": "connection refused"
      }
    ]
  }
}
```

`404 Not Found` — no records exist for this check name:
```json
{
  "error": "no history found for check \"api-health\""
}
```

`400 Bad Request` — invalid `limit`:
```json
{
  "error": "invalid limit: must be a positive integer"
}
```

---

### GET /history/*

Returns execution records for all checks that have recorded history.
The `checks` map contains one key per recorded check.

**Query parameters**:

| Parameter | Type | Required | Default | Description                                        |
|-----------|------|----------|---------|---------------------------------------------------|
| `limit`   | int  | no       | 100     | Maximum records per check (applied independently) |

**Responses**:

`200 OK` — always 200, empty map when no history exists:
```json
{
  "checks": {
    "api-health": [
      {
        "timestamp": "2026-03-06T10:05:00Z",
        "healthy": true,
        "duration_ms": 42,
        "error": ""
      }
    ],
    "db-ping": [
      {
        "timestamp": "2026-03-06T10:05:01Z",
        "healthy": false,
        "duration_ms": 5001,
        "error": "dial timeout"
      }
    ]
  }
}
```

`400 Bad Request`:
```json
{
  "error": "invalid limit: must be a positive integer"
}
```

---

## Behaviour Notes

- **Ordering**: always newest-first (reverse-chronological by execution timestamp).
- **`limit` default**: 100. Zero or negative is a 400 error.
- **Removed checks**: checks removed from config retain history and remain queryable.
- **No metrics server**: if `metrics` is absent from config, the history API is not started.
  The store still writes records; the API is simply unavailable.
- **Localhost enforcement**: server binds to `MetricsConfig.Address` (e.g. `127.0.0.1:9090`).

---

## Future Extensions (not in scope)

- Datetime range filtering: `?from=<RFC3339>&to=<RFC3339>` — schema supports it (timestamp index).
- Summary endpoint: `GET /history/{check_name}/summary` — uptime %, p95 latency.
