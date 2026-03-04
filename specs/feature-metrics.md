# Metrics

WatchDawg exposes a metrics endpoint. When enabled, an HTTP server starts on
the configured address and serves metrics in the format appropriate for the
configured type.

---

## Configuration

Metrics are opt-in. The endpoint is not started unless a `metrics` block is
present in the config.

```json
{
  "metrics": {
    "type": "prometheus",
    "address": ":9090"
  },
  "healthchecks": [...]
}
```

| Field     | Required | Default        | Description |
|-----------|----------|----------------|-------------|
| `type`    | no       | `"prometheus"` | Metrics exposition format. Currently only `"prometheus"` is supported. |
| `address` | yes      | —              | Host and port to bind the metrics server (e.g. `":9090"`, `"127.0.0.1:9090"`). |

The metrics server starts when the daemon starts and shuts down cleanly when the
daemon receives a termination signal. An unrecognised `type` value is a
configuration error and prevents startup.

---

## Metrics

All metrics use the namespace `watchdawg`. Labels are applied where noted.

### `watchdawg_check_up`

**Type:** Gauge

**Description:** Whether the check is currently healthy. `1` means healthy, `0`
means unhealthy.

**Labels:** `check` (check name)

The value reflects the outcome of the most recent execution. Before a check has
run for the first time, this metric is not present.

---

### `watchdawg_check_executions_total`

**Type:** Counter

**Description:** Total number of times a check has been executed, including
retried attempts.

**Labels:** `check` (check name), `result` (`success` or `failure`)

A single scheduled tick that retries twice and then fails increments the counter
three times: two with `result="failure"` and no `result="success"`.

---

### `watchdawg_check_duration_seconds`

**Type:** Histogram

**Description:** Duration of each check execution attempt in seconds.

**Labels:** `check` (check name)

Duration is measured from the start of the attempt to the moment a result is
produced, including any Starlark assertion time but excluding retry delay.

---

### `watchdawg_check_message_age_seconds`

**Type:** Gauge

**Description:** Age of the most recently received message in seconds, measured at each scheduled check execution.

**Labels:** `check` (check name)

Only emitted by checks that consume messages (e.g. Kafka). Not present until at least one message has been received on the topic.

---

### `watchdawg_hook_duration_seconds`

**Type:** Histogram

**Description:** Duration of each hook execution in seconds, from invocation to completion (or failure).

**Labels:** `check` (check name), `type` (`http` or `kafka`), `target` (URL or topic name), `trigger` (`on_success` or `on_failure`)

Useful for detecting slow HTTP hook endpoints or a lagging Kafka producer.

---

### `watchdawg_hook_executions_total`

**Type:** Counter

**Description:** Total number of hook executions, labeled by hook type, destination, and what triggered them.

**Labels:**

| Label     | Values                | Description |
|-----------|-----------------------|-------------|
| `check`   | check name            | The check that produced the result. |
| `type`    | `http`, `kafka`       | Hook type. |
| `target`  | URL or topic name     | Destination of the hook: the URL for HTTP hooks, the topic name for Kafka hooks. |
| `trigger` | `on_success`, `on_failure` | Which list the hook belongs to. |
| `result`  | `success`, `failure`  | Whether the hook execution succeeded. |

Each hook in an `on_success` or `on_failure` list is counted individually.
Multiple HTTP hooks on the same check are distinguished by `target`.

---

## Examples

Minimal config enabling metrics on all interfaces at port 9090:

```json
{
  "metrics": {
    "address": ":9090"
  },
  "healthchecks": [
    {
      "name": "api",
      "schedule": "30s",
      "http": { "url": "https://api.example.com/health", "method": "GET", "expected": { "status_code": 200 } },
      "on_failure": [
        { "http": { "url": "https://hooks.example.com/alert" } },
        { "http": { "url": "https://pagerduty.example.com/notify" } }
      ]
    }
  ]
}
```

Sample scrape output after the first execution (healthy, one failure hook run each):

```
# HELP watchdawg_check_up Whether the check is currently healthy (1=up, 0=down)
# TYPE watchdawg_check_up gauge
watchdawg_check_up{check="api"} 1

# HELP watchdawg_check_executions_total Total number of check execution attempts
# TYPE watchdawg_check_executions_total counter
watchdawg_check_executions_total{check="api",result="success"} 1

# HELP watchdawg_check_duration_seconds Duration of each check execution attempt in seconds
# TYPE watchdawg_check_duration_seconds histogram
watchdawg_check_duration_seconds_bucket{check="api",le="0.005"} 0
watchdawg_check_duration_seconds_bucket{check="api",le="0.01"} 0
watchdawg_check_duration_seconds_bucket{check="api",le="0.025"} 1
watchdawg_check_duration_seconds_bucket{check="api",le="0.05"} 1
watchdawg_check_duration_seconds_bucket{check="api",le="0.1"} 1
watchdawg_check_duration_seconds_bucket{check="api",le="0.25"} 1
watchdawg_check_duration_seconds_bucket{check="api",le="0.5"} 1
watchdawg_check_duration_seconds_bucket{check="api",le="1"} 1
watchdawg_check_duration_seconds_bucket{check="api",le="2.5"} 1
watchdawg_check_duration_seconds_bucket{check="api",le="5"} 1
watchdawg_check_duration_seconds_bucket{check="api",le="10"} 1
watchdawg_check_duration_seconds_bucket{check="api",le="+Inf"} 1
watchdawg_check_duration_seconds_sum{check="api"} 0.021
watchdawg_check_duration_seconds_count{check="api"} 1

# HELP watchdawg_hook_executions_total Total number of hook executions
# TYPE watchdawg_hook_executions_total counter
watchdawg_hook_executions_total{check="api",type="http",target="https://hooks.example.com/alert",trigger="on_failure",result="success"} 1
watchdawg_hook_executions_total{check="api",type="http",target="https://pagerduty.example.com/notify",trigger="on_failure",result="success"} 1

# HELP watchdawg_hook_duration_seconds Duration of each hook execution in seconds
# TYPE watchdawg_hook_duration_seconds histogram
watchdawg_hook_duration_seconds_bucket{check="api",type="http",target="https://hooks.example.com/alert",trigger="on_failure",le="0.005"} 0
watchdawg_hook_duration_seconds_bucket{check="api",type="http",target="https://hooks.example.com/alert",trigger="on_failure",le="0.01"} 0
watchdawg_hook_duration_seconds_bucket{check="api",type="http",target="https://hooks.example.com/alert",trigger="on_failure",le="0.025"} 0
watchdawg_hook_duration_seconds_bucket{check="api",type="http",target="https://hooks.example.com/alert",trigger="on_failure",le="0.05"} 1
watchdawg_hook_duration_seconds_bucket{check="api",type="http",target="https://hooks.example.com/alert",trigger="on_failure",le="+Inf"} 1
watchdawg_hook_duration_seconds_sum{check="api",type="http",target="https://hooks.example.com/alert",trigger="on_failure"} 0.038
watchdawg_hook_duration_seconds_count{check="api",type="http",target="https://hooks.example.com/alert",trigger="on_failure"} 1
```
