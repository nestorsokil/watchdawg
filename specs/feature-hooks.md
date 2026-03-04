# Hooks

Hooks are outbound notifications fired after each check execution. They are
configured per check under `on_success` and `on_failure`. Each list may contain
any number of hooks of any supported type, mixed freely.

```json
{
  "on_success": [
    { "http": { "url": "https://example.com/notify", "method": "POST" } }
  ],
  "on_failure": [
    { "http": { "url": "https://example.com/alert", "method": "POST" } },
    { "kafka": { "brokers": ["localhost:9092"], "topic": "alerts" } }
  ]
}
```

---

## Triggers

| List         | Fires when |
|--------------|------------|
| `on_success` | The check result is healthy |
| `on_failure` | The check result is unhealthy |

All hooks in a list execute in parallel. A failure in one hook does not prevent
others from running. Hook errors are reported but do not affect the check result.

---

## Message body

Both hook types support an optional template field (`body_template` for HTTP,
`message_template` for Kafka). The template is a [Go `text/template`](https://pkg.go.dev/text/template)
string rendered with the check result as its data.

Available template fields:

| Field          | Type   | Description |
|----------------|--------|-------------|
| `.CheckName`   | string | Check identifier |
| `.Healthy`     | bool   | Whether the check passed |
| `.Message`     | string | Human-readable outcome |
| `.Error`       | string | Infrastructure error, if any |
| `.Duration`    | int    | Total elapsed milliseconds |
| `.Attempt`     | int    | 1-based attempt number |
| `.Timestamp`   | time   | Time of the attempt |

When no template is provided, the full check result is serialized as JSON and
used as the body/message.

---

## HTTP hook

Sends an HTTP request to a configured URL.

```json
{
  "http": {
    "url": "https://hooks.example.com/notify",
    "method": "POST",
    "headers": { "Authorization": "Bearer token" },
    "body_template": "Check '{{.CheckName}}' failed: {{.Message}}"
  }
}
```

| Field           | Required | Description |
|-----------------|----------|-------------|
| `url`           | yes      | Destination URL. |
| `method`        | no       | HTTP method. Defaults to `POST`. |
| `headers`       | no       | Additional request headers. |
| `body_template` | no       | Go template for the request body. Defaults to JSON-serialized result. |

`Content-Type: application/json` is set automatically unless overridden in
`headers`.

The hook fails if the server returns a non-2xx status.

---

## Kafka hook

Publishes a message to a Kafka topic.

```json
{
  "kafka": {
    "brokers": ["localhost:9092"],
    "topic": "health-alerts",
    "message_template": "Check '{{.CheckName}}' failed: {{.Message}}"
  }
}
```

| Field              | Required | Description |
|--------------------|----------|-------------|
| `brokers`          | yes      | List of Kafka broker addresses. |
| `topic`            | yes      | Topic to publish to. |
| `message_template` | no       | Go template for the message value. Defaults to JSON-serialized result. |

The message is published with the rendered (or JSON) body as the value. No key
or headers are set.

---

## Examples

### Notify a webhook on failure

```json
"on_failure": [
  {
    "http": {
      "url": "https://hooks.example.com/alert",
      "method": "POST",
      "body_template": "ALERT: '{{.CheckName}}' is down. Reason: {{.Message}}"
    }
  }
]
```

### Publish to Kafka on success

```json
"on_success": [
  {
    "kafka": {
      "brokers": ["localhost:9092"],
      "topic": "watchdawg-results",
      "message_template": "{{.CheckName}} passed in {{.Duration}}ms"
    }
  }
]
```

### Multiple hooks, mixed types

```json
"on_failure": [
  { "http":  { "url": "https://hooks.example.com/alert" } },
  { "kafka": { "brokers": ["localhost:9092"], "topic": "alerts" } }
]
```

Both fire in parallel.
