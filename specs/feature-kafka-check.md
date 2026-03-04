# Kafka Check

A Kafka health check verifies that messages are actively flowing on a configured
topic. It answers the question: "has this topic received a message recently?"

The check uses a **background consumer** that runs continuously between ticks.
Each scheduled execution inspects the consumer's state rather than performing a
live request.

---

## Configuration

```json
{
  "name": "order-events-liveness",
  "schedule": "30s",
  "timeout": 5000000000,
  "kafka": {
    "brokers": ["localhost:9092"],
    "topic": "orders",
    "group_id": "watchdawg-order-events",
    "format": "json",
    "assertion": "result.get('status') in ('completed', 'pending')"
  }
}
```

| Field            | Required | Description |
|------------------|----------|-------------|
| `name`           | yes      | Unique check identifier. |
| `schedule`       | yes      | Interval as a duration string (e.g. `"30s"`, `"5m"`). Defines the maximum acceptable gap between messages. |
| `timeout`        | no       | Deadline for each Execute call. Does not affect the background consumer. |
| `kafka.brokers`  | yes      | List of Kafka broker addresses. |
| `kafka.topic`    | yes      | Topic to consume from. |
| `kafka.group_id` | no       | Consumer group ID. Defaults to a name derived from the check name. |
| `kafka.format`   | no       | Parse message values as `"json"` to make them available as `result` in assertions. |
| `kafka.assertion`| no       | Starlark expression or script validated against the most recently received message. |

> **Note:** Kafka checks do not support `retries`. Each execution reflects a
> point-in-time snapshot of the background consumer's state.

---

## Consumer lifecycle

A background consumer is started for each Kafka check when the scheduler
initialises. It runs until the scheduler shuts down.

The consumer starts from the **latest offset**: it ignores any message backlog
and only tracks messages that arrive after startup. This ensures that the check
reflects live traffic, not historical data.

If the consumer encounters a transient error (e.g. a broker connection blip) it
logs the error and retries automatically. If it crashes unexpectedly it restarts
itself, provided the scheduler has not shut down.

---

## Execution

On each scheduled tick, the check:

1. Reads the consumer's current state (time of last message, content of last message).
2. Applies liveness and assertion checks against that snapshot.
3. Returns a result immediately — no network calls are made.

### Liveness check

| Consumer state | Outcome |
|----------------|---------|
| No messages received since startup | **Healthy** — waiting for the first message |
| Last message within the schedule interval | Proceed to assertion (if configured) |
| Last message older than the schedule interval | **Unhealthy** — gap exceeds the expected interval |

The schedule interval (e.g. `"30s"`) serves a dual purpose: it sets both the
execution frequency and the maximum acceptable silence on the topic.

### Assertion

If an assertion is configured and the liveness check passes, it is evaluated
against the **most recently received message**. See
[feature-assertions.md](feature-assertions.md) for expression syntax and result
extraction rules.

Variables available in Kafka assertions:

| Variable  | Type   | Value |
|-----------|--------|-------|
| `value`   | string | Raw message value |
| `key`     | string | Message key |
| `headers` | dict   | Message headers (one string value per key) |
| `result`  | dict / list / scalar | Parsed message value — only present when `kafka.format` is set |

---

## Result

| Field      | Description |
|------------|-------------|
| `healthy`  | Whether the topic is considered live. |
| `message`  | Human-readable outcome, including topic name and message age where relevant. |
| `duration` | Elapsed milliseconds for this Execute call (does not include consumer time). |
| `attempt`  | Always `1` — Kafka checks do not retry. |
| `error`    | Set on assertion errors or consumer state errors. Empty for liveness failures. |

---

## Examples

### Basic liveness — any message within the interval

```json
{
  "kafka": {
    "brokers": ["localhost:9092"],
    "topic": "orders"
  }
}
```

Healthy as long as at least one message arrived on `orders` within the last
`schedule` interval.

### Assert on message content

```json
{
  "kafka": {
    "brokers": ["localhost:9092"],
    "topic": "payments",
    "format": "json",
    "assertion": "result.get('status') in ('completed', 'pending')"
  }
}
```

Healthy only if a recent message arrived **and** its `status` field is an
expected value.

### Assertion examples

See [feature-assertions.md](feature-assertions.md). Kafka-specific variables
(`value`, `key`, `headers`, `result`) are available.
