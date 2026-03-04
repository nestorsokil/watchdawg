# gRPC Check

A gRPC health check verifies that a gRPC server (or a named service on that
server) is responding as healthy using the standard
[gRPC Health Checking Protocol](https://github.com/grpc/grpc/blob/master/doc/health-checking.md).

---

## Configuration

```json
{
  "name": "my-service-health",
  "schedule": "30s",
  "retries": 2,
  "timeout": 5000000000,
  "grpc": {
    "target": "myservice:50051",
    "plaintext": true,
    "service": "my.package.MyService"
  }
}
```

| Field             | Required | Description |
|-------------------|----------|-------------|
| `name`            | yes      | Unique check identifier. |
| `schedule`        | yes      | Interval (`"30s"`, `"5m"`, `"1h"`) or cron expression. |
| `retries`         | no       | Additional attempts after the first (default: 0). |
| `timeout`         | no       | Deadline for the entire execution including retries. |
| `grpc.target`     | yes      | Server address as `host:port`. |
| `grpc.plaintext`  | no       | When `true`, connect without TLS. Takes precedence over `verify_tls`. Default: `false`. |
| `grpc.verify_tls` | no       | Set to `false` to accept self-signed or invalid certificates. Only applies when `plaintext` is false. Default: `true`. |
| `grpc.service`    | no       | Fully-qualified service name to check. When omitted, a server-level check is performed. |

---

## Execution

The retry model is the same as the HTTP check: `retries + 1` total attempts,
with a fixed pause between failed attempts, and an early return on the first
success. Duration covers all attempts.

Each attempt:
1. Opens a connection to `target`.
2. Calls `grpc.health.v1.Health/Check` with the configured `service` name (empty string for server-level).
3. Evaluates the response status.
4. Closes the connection.

A connection failure or an RPC-level error is treated as unhealthy.

---

## Health determination

The check passes if and only if the server responds with status `SERVING`.

Any other status (`NOT_SERVING`, `SERVICE_UNKNOWN`, `UNKNOWN`) is a failure.
The raw status string is included in the result so callers can distinguish these
cases.

---

## Result

| Field           | Description |
|-----------------|-------------|
| `healthy`       | `true` only when the server responded `SERVING`. |
| `message`       | Human-readable outcome. |
| `duration`      | Total elapsed milliseconds across all attempts. |
| `attempt`       | 1-based number of the attempt that produced this result. |
| `error`         | Set on connection or RPC failures. Empty when the server responded but was not `SERVING`. |
| `grpc.status`   | The raw health status string from the server (`"SERVING"`, `"NOT_SERVING"`, etc.). Present whenever the server responded, even on failure. |

---

## Examples

### Server-level check over plaintext

```json
{
  "grpc": {
    "target": "myservice:50051",
    "plaintext": true
  }
}
```

Checks the overall server health. Suitable for internal services that don't use TLS.

### Named service check over TLS

```json
{
  "grpc": {
    "target": "myservice:443",
    "service": "my.package.MyService"
  }
}
```

### Self-signed certificate

```json
{
  "grpc": {
    "target": "dev-service:50051",
    "verify_tls": false
  }
}
```
