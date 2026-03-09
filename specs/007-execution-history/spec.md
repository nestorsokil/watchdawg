# Feature Specification: Execution History & Reporting

**Feature Branch**: `007-execution-history`
**Created**: 2026-03-06
**Status**: Draft
**Input**: User description: "Build a feature storing execution data in a persistent store. The end goal is allowing detailed reporting on select healthchecks."

## Clarifications

### Session 2026-03-09

- Direct: ExecutionRecord identifiers use UUID (not autoincrement integers).

### Session 2026-03-06

- Q: How should concurrent writes from parallel check executions be handled? → A: Concurrency control is delegated to the underlying storage mechanism; the choice of storage is a plan-phase decision.
- Q: How do operators access execution history? → A: Via REST API. HTML reporting is out of scope for this feature (planned as a subsequent feature).
- Q: What is the access scope for the REST API? → A: Localhost only, no authentication.
- Q: What happens when the persistent store is corrupted or unreadable at startup? → A: Daemon fails to start; operator must repair or delete the store.
- Q: What happens to records when a check is removed from config? → A: Records are retained indefinitely and remain accessible via API using the check name.
- Q: What happens when a record cannot be written due to insufficient disk space? → A: Recording is skipped for that execution, an error is logged, and the daemon continues normally.
- Direct: Retention enforcement is active — oldest records are evicted at write time when the limit is exceeded.
- Direct: The REST API exposes two endpoints under `/history`: one scoped to a specific check (`/history/{check_name}`) and one returning history across all checks (`GET /history/*`), each with its own `limit` query parameter.
- Direct: The persistent store must support flexible queries (e.g., by check name, time range) to enable future aggregation endpoints. Prometheus is explicitly out of scope for this feature; it handles aggregated metrics and is complementary, not a substitute.
- Direct: Datetime range filtering on query endpoints is not in scope for this iteration but the store must not preclude it.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Record Healthcheck Executions (Priority: P1)

An operator has several healthchecks running on a schedule. They want execution results for specific checks to be recorded persistently so they can review them after the fact — even across daemon restarts.

They opt specific checks into recording via config. Watchdawg stores each execution's outcome (pass/fail), timestamp, duration, and failure reason (if any) to a local store on disk. The store survives daemon restarts.

**Why this priority**: This is the foundation of the feature. Reporting is impossible without stored data. Delivers immediate value even before any reporting interface exists.

**Independent Test**: Can be fully tested by configuring a check with recording enabled, running it a few times, restarting the daemon, and verifying the stored records still exist and are accurate.

**Acceptance Scenarios**:

1. **Given** a check is configured with recording enabled, **When** it executes (pass or fail), **Then** the result, timestamp, duration, and error message (if failed) are written to the persistent store.
2. **Given** Watchdawg is restarted, **When** a recorded check executes again, **Then** the new record is appended to the existing history without data loss.
3. **Given** a check is configured without recording enabled, **When** it executes, **Then** no execution record is written to the store.

---

### User Story 2 - Query Execution History via API (Priority: P2)

An operator suspects a healthcheck has been failing intermittently. They call a REST API endpoint to retrieve the execution history for that check, receiving a structured list of past results with timestamps, durations, and failure details. They can also call a second endpoint to see history across all recorded checks in one response.

**Why this priority**: This is the primary reporting interface. Delivers the core user-facing value of the feature. HTML reporting is planned as a subsequent feature built on top of this API.

**Independent Test**: Can be fully tested by running a few recorded executions and then calling both API endpoints; verifies count, ordering, and content of returned records for the per-check endpoint and the aggregated `GET /history/*` endpoint.

**Acceptance Scenarios**:

1. **Given** a check has recorded executions, **When** the operator calls `GET /history/{check_name}`, **Then** results are returned in reverse-chronological order (most recent first) with timestamp, status, duration, and error message for each.
2. **Given** a check has no recorded executions, **When** the operator calls `GET /history/{check_name}`, **Then** an empty result set is returned with a clear indication.
3. **Given** a request to `GET /history/{check_name}` with a `limit` parameter (e.g., last 10), **When** executed, **Then** only that many records are returned.
4. **Given** a check name that does not exist in the store, **When** the operator calls `GET /history/{check_name}`, **Then** a 404 response with an informative message is returned.
5. **Given** multiple checks have recorded executions, **When** the operator calls `GET /history/*`, **Then** results for all recorded checks are returned, grouped or ordered by check name, each with their execution records in reverse-chronological order.
6. **Given** a request to `GET /history/*` with a `limit` parameter, **When** executed, **Then** at most that many records per check are returned.

---

### User Story 3 - Bounded Storage via Retention Policy (Priority: P3)

An operator runs Watchdawg continuously. Without limits, stored execution records would grow without bound. They configure a retention policy so the store stays manageable over time.

**Why this priority**: Necessary for production use but does not affect core functionality. Can be deferred without breaking P1/P2.

**Independent Test**: Can be tested by configuring a low retention limit, running more executions than the limit, and verifying only the most recent N records are retained.

**Acceptance Scenarios**:

1. **Given** a retention limit of N records per check, **When** the total exceeds N, **Then** the oldest record for that check is evicted so the count returns to N.
2. **Given** no retention limit is configured, **Then** a reasonable default limit is applied (see Assumptions).
3. **Given** a global default retention configured, **When** a check does not override it, **Then** the global default applies.

---

### Edge Cases

- If the persistent store is corrupted or unreadable, the daemon refuses to start and logs a clear diagnostic error.
- If recording fails due to insufficient disk space, the execution record is skipped, an error is logged, and the daemon continues normally.
- Records for checks removed from config are retained indefinitely and remain queryable via API.
- Concurrent writes from parallel check executions are handled by the storage layer (plan-phase concern).

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Operators MUST be able to opt individual healthchecks into execution recording via the config file.
- **FR-002**: System MUST record, for each opted-in check execution: a UUID identifier (generated at write time), status (pass/fail), start timestamp, execution duration, and error message (if failed). Autoincrement integer IDs MUST NOT be used.
- **FR-003**: Recorded data MUST persist across daemon restarts.
- **FR-004**: The API MUST expose a `GET /history/{check_name}` endpoint returning execution records for that specific check in reverse-chronological order.
- **FR-004b**: The API MUST expose a `GET /history/*` endpoint returning execution records for all recorded checks.
- **FR-005**: Both endpoints MUST include all recorded fields per execution (timestamp, status, duration, error message).
- **FR-006**: `GET /history/{check_name}` MUST accept a `limit` query parameter to restrict the number of returned records.
- **FR-006b**: `GET /history/*` MUST accept its own `limit` query parameter, applied independently per check in the response.
- **FR-007**: System MUST enforce a maximum record count per check to prevent unbounded storage growth; when the count exceeds the limit, the oldest records for that check MUST be evicted. This limit MUST be configurable per check and globally, with a sensible default.
- **FR-008**: Checks not opted into recording MUST have zero performance or storage impact from this feature.
- **FR-009**: If the persistent store is corrupted or unreadable at startup, the daemon MUST refuse to start and MUST emit a clear diagnostic error indicating the store path and nature of the failure.
- **FR-009b**: System MUST handle store write errors during normal operation gracefully (log and skip the record) without crashing the daemon or disrupting scheduled checks.
- **FR-010**: The REST API MUST return structured data suitable for consumption by a future HTML reporting frontend.
- **FR-011**: The REST API MUST only accept connections from localhost; network-external requests MUST be rejected.
- **FR-012**: The persistent store MUST support flexible queries by check name and time range to enable future aggregation endpoints (e.g., uptime percentage, p95 latency). Datetime range filtering on the API is not in scope for this iteration.
- **FR-013**: This feature is complementary to Prometheus metrics and does not replace them. Prometheus handles aggregated numeric metrics; this store handles individual execution records with full detail (including error messages).

### Key Entities

- **ExecutionRecord**: Represents a single check execution. Key attributes: id (UUID, globally unique, generated at write time), check name, timestamp (when execution started), duration (total execution time), status (pass/fail), error message (empty if passed). Autoincrement integer IDs are explicitly rejected; UUID is the identity scheme.
- **ExecutionStore**: The persistent container of all execution records, keyed by check name. Enforces retention policy, survives process restarts, and delegates concurrent write safety to the underlying storage mechanism.
- **RetentionPolicy**: Defines how many records to keep per check. Can be set globally or overridden per check. A global default is always in effect. Oldest records are evicted to enforce the limit.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Operators can retrieve the full execution history for any recorded check via the API within 1 second, even with thousands of stored records.
- **SC-002**: Enabling execution recording on a check adds no measurable latency to that check's scheduled execution (recording is non-blocking or occurs post-execution).
- **SC-003**: The persistent store survives daemon restarts without data loss under normal operating conditions.
- **SC-004**: Storage used by the execution history grows in a predictable, bounded manner when retention limits are configured.
- **SC-005**: Checks without recording enabled are completely unaffected — zero change in behavior, resource use, or output.

## Assumptions

- **Opt-in recording**: "Select healthchecks" means individual checks are opted in via config (e.g., a `record: true` flag). All-or-nothing global recording is not supported in this iteration.
- **REST API reporting**: The reporting interface is a REST API endpoint. HTML reporting is explicitly out of scope and planned as a subsequent feature.
- **Local storage only**: The persistent store is local to the machine running Watchdawg. Remote/centralized storage is out of scope.
- **Default retention**: 1000 records per check if not explicitly configured.
- **Per-execution granularity**: Each top-level check execution is one record (not per-retry attempt).
- **No alerting changes**: This feature does not change how webhooks or notifications are triggered.
- **Concurrent writes**: Safety under concurrent writes is delegated to the underlying storage technology, to be selected during planning.
- **API access**: The REST API is localhost-only with no authentication. No auth mechanism is in scope for this feature.
