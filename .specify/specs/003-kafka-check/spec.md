# Feature Specification: Kafka Health Check

**Feature Branch**: `003-kafka-check`
**Created**: 2026-03-06
**Status**: Implemented

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Topic Liveness Monitoring (Priority: P1)

An operator wants to know whether messages are actively flowing on a Kafka topic. Silence longer than a configured interval means something upstream has stopped producing.

**Why this priority**: Liveness is the primary reason to use a Kafka check. Message content validation only makes sense when messages are actually arriving.

**Independent Test**: Configure a check for a topic. Publish a message, then wait longer than the schedule interval without publishing another. Verify the check goes unhealthy after the interval elapses.

**Acceptance Scenarios**:

1. **Given** a Kafka check on a topic with no messages since startup, **When** the check runs, **Then** the result is healthy (waiting for first message is not a failure)
2. **Given** a message arrived within the last schedule interval, **When** the check runs, **Then** the result is healthy
3. **Given** the last message arrived longer ago than the schedule interval, **When** the check runs, **Then** the result is unhealthy with the topic name and message age in the message
4. **Given** the check is configured with `schedule: "30s"`, **When** 31 seconds pass without a message, **Then** the check is unhealthy
5. **Given** the consumer starts up, **When** it begins consuming, **Then** it reads from the latest offset and ignores any backlog

---

### User Story 2 - Message Content Assertion (Priority: P2)

An operator wants to validate that recent messages on the topic have the expected structure or values, not just that messages are arriving.

**Why this priority**: Content validation adds correctness checking. It requires liveness to already be working.

**Independent Test**: Configure a check with `format: "json"` and an assertion. Publish a message that satisfies the assertion, then one that doesn't. Verify the check reflects the most recent message's validity.

**Acceptance Scenarios**:

1. **Given** `format: "json"` and `assertion: "result.get('status') in ('ok', 'pending')"`, **When** a recent message has `status: "ok"`, **Then** the check is healthy
2. **Given** the same config, **When** a recent message has `status: "error"`, **Then** the check is unhealthy
3. **Given** `format: "json"` and a message that is not valid JSON, **When** the check runs, **Then** the check is unhealthy before the assertion runs
4. **Given** an assertion is configured but the liveness check fails (no recent message), **When** the check runs, **Then** the assertion is not evaluated
5. **Given** an assertion that crashes (script error), **When** the check runs, **Then** the check is unhealthy and the error is surfaced

---

### User Story 3 - Custom Consumer Group (Priority: P3)

An operator wants to control the consumer group ID used by the check, e.g. to avoid group ID collisions or to align with naming conventions.

**Why this priority**: A reasonable default is provided; this is a minor operational concern.

**Independent Test**: Configure `group_id` explicitly. Verify the consumer joins Kafka using that group ID.

**Acceptance Scenarios**:

1. **Given** `group_id: "watchdawg-orders"`, **When** the consumer starts, **Then** it joins Kafka using that group ID
2. **Given** no `group_id` configured, **When** the consumer starts, **Then** it uses a group ID derived from the check name

---

### Edge Cases

- What if the Kafka brokers are unreachable at startup? The consumer retries in the background; the check is healthy (no messages received yet) until the silence interval elapses.
- What if the consumer crashes? It restarts automatically as long as the scheduler has not shut down.
- Can Kafka checks use `retries`? No. Each execution is a point-in-time snapshot; retries are not meaningful.
- What if `format` is set and the message value is empty? Parsing fails; the check is unhealthy.
- What variables are available in assertions? `value` (raw string), `key`, `headers` (dict), and `result` (parsed value, only when `format` is set).

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST start a background consumer per Kafka check when the scheduler initialises
- **FR-002**: System MUST consume from the latest offset, ignoring any message backlog
- **FR-003**: System MUST automatically restart the consumer on unexpected crashes while the scheduler is running
- **FR-004**: On each scheduled execution, system MUST snapshot the consumer state without making network calls
- **FR-005**: System MUST consider the check healthy when no messages have been received since startup
- **FR-006**: System MUST consider the check unhealthy when the most recent message is older than the schedule interval
- **FR-007**: System MUST run assertions only when the liveness check passes
- **FR-008**: System MUST parse message values as JSON when `format: "json"` is configured, making the result available in assertions
- **FR-009**: System MUST use a configurable consumer group ID, defaulting to a name derived from the check name
- **FR-010**: System MUST expose Kafka-specific assertion variables: `value`, `key`, `headers`, and `result` (when format is set)

### Key Entities

- **Check Result**: `healthy`, `message` (includes topic and message age), `duration`, `attempt` (always 1), `error`
- **Consumer State**: Time of last message, content of last message; updated by the background consumer

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An operator can detect topic silence within one schedule interval of the last message
- **SC-002**: An operator can validate message content without modifying the producer or adding custom consumers
- **SC-003**: The background consumer recovers from transient broker connectivity issues without operator intervention
- **SC-004**: Starting WatchDawg does not produce false alarms for topics that are simply quiet at startup
