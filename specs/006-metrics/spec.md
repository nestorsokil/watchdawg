# Feature Specification: Metrics

**Feature Branch**: `006-metrics`
**Created**: 2026-03-06
**Status**: Implemented

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Check Health Metrics (Priority: P1)

An operator running Watchdawg in production wants to scrape check health status and execution counts into their existing monitoring stack (e.g. Prometheus + Grafana), so they can build dashboards and alerts without polling Watchdawg logs.

**Why this priority**: Health metrics (up/down gauge, execution counter, duration histogram) are the core observability value. Everything else builds on having these scraped.

**Independent Test**: Enable metrics with a bind address. Run a check. Scrape the endpoint. Verify `watchdawg_check_up`, `watchdawg_check_executions_total`, and `watchdawg_check_duration_seconds` are present with correct values.

**Acceptance Scenarios**:

1. **Given** a `metrics` block with a valid `address`, **When** Watchdawg starts, **Then** a metrics endpoint is available at that address
2. **Given** no `metrics` block in config, **When** Watchdawg starts, **Then** no metrics endpoint is started
3. **Given** a check that just ran healthy, **When** the endpoint is scraped, **Then** `watchdawg_check_up{check="..."}` is `1`
4. **Given** a check that just ran unhealthy, **When** the endpoint is scraped, **Then** `watchdawg_check_up{check="..."}` is `0`
5. **Given** a check with retries that fails twice then succeeds, **When** the endpoint is scraped, **Then** `watchdawg_check_executions_total` shows 2 failures and 1 success
6. **Given** a check that has never run, **When** the endpoint is scraped, **Then** `watchdawg_check_up` is not present for that check
7. **Given** an unrecognised `type` value, **When** Watchdawg starts, **Then** startup fails with a configuration error

---

### User Story 2 - Hook Execution Metrics (Priority: P2)

An operator wants to monitor the performance and reliability of their hooks — detecting slow webhook endpoints or Kafka publish failures — using the same scrape pipeline.

**Why this priority**: Hook metrics are complementary to check metrics and essential for diagnosing notification pipeline issues.

**Independent Test**: Configure a hook. Trigger it. Scrape the endpoint. Verify `watchdawg_hook_executions_total` and `watchdawg_hook_duration_seconds` are present with correct labels.

**Acceptance Scenarios**:

1. **Given** an `on_failure` HTTP hook that fires successfully, **When** the endpoint is scraped, **Then** `watchdawg_hook_executions_total{type="http", trigger="on_failure", result="success"}` is incremented
2. **Given** a hook that fails, **When** the endpoint is scraped, **Then** `watchdawg_hook_executions_total{..., result="failure"}` is incremented
3. **Given** two HTTP hooks with different URLs on the same check, **When** both fire, **Then** each is counted separately by the `target` label

---

### User Story 3 - Kafka Message Age Metric (Priority: P3)

An operator monitoring Kafka checks wants to track how stale the most recently seen message is at each check execution, enabling lag-based alerting.

**Why this priority**: This metric is specific to Kafka checks and only meaningful once the core check and hook metrics are in place.

**Independent Test**: Configure a Kafka check. Receive a message. Scrape the endpoint. Verify `watchdawg_check_message_age_seconds` appears with the correct age for that check.

**Acceptance Scenarios**:

1. **Given** a Kafka check that has received at least one message, **When** the endpoint is scraped, **Then** `watchdawg_check_message_age_seconds{check="..."}` reflects the age at the time of the last execution
2. **Given** a Kafka check that has not yet received any message, **When** the endpoint is scraped, **Then** `watchdawg_check_message_age_seconds` is not present for that check

---

### Edge Cases

- What if the metrics address is already in use? Watchdawg fails to start with an error.
- What if only `address` is configured (no `type`)? Defaults to `"prometheus"`.
- Does the metrics server shut down cleanly on SIGINT/SIGTERM? Yes, it shuts down with the daemon.
- Are metrics namespaced to avoid collisions? Yes, all metrics use the `watchdawg` namespace.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST expose a metrics endpoint only when a `metrics` block is present in config
- **FR-002**: System MUST bind the endpoint to the configured `address`
- **FR-003**: System MUST default `type` to `"prometheus"` when not specified; reject any other value as a config error
- **FR-004**: System MUST expose `watchdawg_check_up` (gauge, per check) reflecting the most recent execution outcome: 1 = healthy, 0 = unhealthy; absent before first run
- **FR-005**: System MUST expose `watchdawg_check_executions_total` (counter, per check + result label) counting every attempt including retries
- **FR-006**: System MUST expose `watchdawg_check_duration_seconds` (histogram, per check) measuring each attempt from start to result, including assertion time but excluding retry delay
- **FR-007**: System MUST expose `watchdawg_check_message_age_seconds` (gauge, per check) for checks that consume messages; absent until at least one message has been received
- **FR-008**: System MUST expose `watchdawg_hook_duration_seconds` (histogram, per check + type + target + trigger) measuring each hook execution from invocation to completion
- **FR-009**: System MUST expose `watchdawg_hook_executions_total` (counter, per check + type + target + trigger + result) counting each hook execution
- **FR-010**: System MUST shut down the metrics server cleanly when the daemon receives a termination signal

### Key Entities

- **Metric Labels**: `check` (check name), `result` (success/failure), `type` (http/kafka), `target` (URL or topic), `trigger` (on_success/on_failure)

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An operator can build a dashboard showing real-time check health status without parsing Watchdawg logs
- **SC-002**: An operator can alert on check execution frequency dropping below expected rate
- **SC-003**: An operator can detect slow or failing hooks by monitoring hook duration and failure counters
- **SC-004**: An operator can alert on Kafka topic message lag using the message age gauge
