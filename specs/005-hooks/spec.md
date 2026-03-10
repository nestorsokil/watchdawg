# Feature Specification: Hooks

**Feature Branch**: `005-hooks`
**Created**: 2026-03-06
**Status**: Implemented

## User Scenarios & Testing *(mandatory)*

### User Story 1 - HTTP Webhook Notification (Priority: P1)

An operator wants to receive an HTTP POST request when a check transitions to unhealthy (or healthy), so they can integrate Watchdawg with their alerting or automation system.

**Why this priority**: HTTP webhooks are the most common notification mechanism. This is the baseline use case for hooks.

**Independent Test**: Configure an `on_failure` HTTP hook. Trigger a check failure. Verify the webhook receives a POST request with the check result as the body.

**Acceptance Scenarios**:

1. **Given** a check with `on_failure` containing an HTTP hook, **When** the check result is unhealthy, **Then** the hook sends a POST to the configured URL
2. **Given** a check with `on_success` containing an HTTP hook, **When** the check result is healthy, **Then** the hook sends a POST to the configured URL
3. **Given** an HTTP hook with no `body_template`, **When** the hook fires, **Then** the body is the full check result serialized as JSON
4. **Given** the webhook server returns a non-2xx status, **When** the hook fires, **Then** the hook is recorded as failed but the check result is unaffected
5. **Given** multiple hooks in `on_failure`, **When** one hook fails, **Then** the remaining hooks still fire

---

### User Story 2 - Kafka Notification (Priority: P2)

An operator wants to publish a message to a Kafka topic when a check succeeds or fails, feeding results into an event-driven pipeline.

**Why this priority**: Kafka hooks extend the notification surface to event-driven architectures. They share the same trigger and template model as HTTP hooks.

**Independent Test**: Configure an `on_failure` Kafka hook. Trigger a check failure. Verify a message appears on the configured topic.

**Acceptance Scenarios**:

1. **Given** a check with `on_failure` containing a Kafka hook, **When** the check result is unhealthy, **Then** a message is published to the configured topic
2. **Given** a Kafka hook with no `message_template`, **When** the hook fires, **Then** the message value is the full check result serialized as JSON
3. **Given** a Kafka hook fires, **When** the message is published, **Then** no key or headers are set on the message

---

### User Story 3 - Custom Message Templates (Priority: P3)

An operator wants to control the content of hook notifications — e.g., to post a human-readable alert instead of a JSON blob, or to fit a specific downstream API contract.

**Why this priority**: Default JSON output works for most integrations; custom templates are a power-user feature.

**Independent Test**: Configure a hook with a `body_template` or `message_template`. Trigger the check. Verify the notification body matches the rendered template.

**Acceptance Scenarios**:

1. **Given** `body_template: "Check '{{.CheckName}}' is down: {{.Message}}"`, **When** the hook fires, **Then** the HTTP body contains the rendered string
2. **Given** `message_template: "{{.CheckName}} failed in {{.Duration}}ms"`, **When** the Kafka hook fires, **Then** the message value contains the rendered string
3. **Given** an HTTP hook with no `Content-Type` header override, **When** the hook fires, **Then** `Content-Type: application/json` is set automatically

---

### Edge Cases

- What if a hook's HTTP target is unreachable? The hook is logged as failed; other hooks in the list still fire.
- What if a template references an unknown field? The template rendering fails; the hook is recorded as failed.
- What if both `on_success` and `on_failure` are empty? No hooks fire; the check runs normally.
- What if the same URL appears in multiple hooks? Each hook fires independently.
- Do hooks affect the check result? No. Hook errors are reported but never change `healthy`.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST support `on_success` and `on_failure` hook lists per check, each containing any number of hooks
- **FR-002**: System MUST fire all hooks in a triggered list in parallel
- **FR-003**: A hook failure MUST NOT prevent other hooks in the list from firing
- **FR-004**: Hook errors MUST be reported but MUST NOT affect the check's `healthy` result
- **FR-005**: System MUST support HTTP hooks: POST to a configured URL with optional custom headers and body template
- **FR-006**: System MUST set `Content-Type: application/json` on HTTP hook requests unless overridden in headers
- **FR-007**: System MUST consider an HTTP hook failed when the server returns a non-2xx status
- **FR-008**: System MUST support Kafka hooks: publish a message to a configured topic with optional message template
- **FR-009**: Kafka hook messages MUST have no key or headers set
- **FR-010**: When no template is configured, system MUST use the full check result serialized as JSON as the body/message
- **FR-011**: Templates MUST be Go `text/template` strings rendered with the check result, exposing: `.CheckName`, `.Healthy`, `.Message`, `.Error`, `.Duration`, `.Attempt`, `.Timestamp`

### Key Entities

- **Hook Result**: Whether the hook succeeded, duration, error if any — reported separately from check result

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An operator receives a notification within one schedule interval of a check state change
- **SC-002**: All hooks in a list fire regardless of individual hook failures
- **SC-003**: An operator can integrate Watchdawg notifications with any HTTP-based alerting system without modifying Watchdawg
- **SC-004**: An operator can customize notification content to match downstream API contracts using template syntax
