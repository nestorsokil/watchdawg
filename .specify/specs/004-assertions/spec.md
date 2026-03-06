# Feature Specification: Assertions

**Feature Branch**: `004-assertions`
**Created**: 2026-03-06
**Status**: Implemented

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Simple Boolean Expression (Priority: P1)

An operator wants to add a quick one-liner validation to a check without learning a scripting language. They just want to express something like "the body must contain 'ok'" in plain code.

**Why this priority**: Simple expressions cover the majority of assertion use cases and are the lowest barrier to entry.

**Independent Test**: Configure a check with a single-line assertion. Verify the check passes when the expression is truthy and fails when it is falsy.

**Acceptance Scenarios**:

1. **Given** `assertion: "status_code == 200 and 'ok' in body"`, **When** the condition is true, **Then** the check is healthy
2. **Given** the same assertion, **When** the condition is false, **Then** the check is unhealthy
3. **Given** a single-line string with no assignment or `def` keywords, **When** the check runs, **Then** it is evaluated as a simple boolean expression
4. **Given** an expression that raises an error (e.g. accessing a missing key), **When** the check runs, **Then** the check is unhealthy and the error is surfaced

---

### User Story 2 - Full Script with Complex Logic (Priority: P2)

An operator wants to express multi-step validation logic: conditionals, helper values, and a custom message explaining why the check failed.

**Why this priority**: Full scripts unlock arbitrary validation logic while remaining embedded in the config. They extend the simple expression model.

**Independent Test**: Configure a multi-line assertion that sets `valid` and `message`. Verify the check result reflects those values.

**Acceptance Scenarios**:

1. **Given** a script that sets `valid = True` and `message = "all good"`, **When** the check runs, **Then** the result is healthy with that message
2. **Given** a script that sets `valid = False` and `message = "bad status"`, **When** the check runs, **Then** the result is unhealthy with that message
3. **Given** a string containing `valid =`, `healthy =`, `message =`, or `def `, **When** the check runs, **Then** it is treated as a full script, not a simple expression
4. **Given** a script that defines a `check()` function and returns a dict with `valid` and `message`, **When** the check runs, **Then** the result reflects the dict

---

### User Story 3 - Parsed Input Validation (Priority: P3)

An operator wants to validate structured data (JSON or XML response body, JSON Kafka message value) by accessing it as a parsed object rather than a raw string.

**Why this priority**: Parsing is a convenience on top of assertions; it requires the check's `format` field to be set first.

**Independent Test**: Configure `format: "json"` and an assertion that references `result`. Verify `result` is a dict/list/scalar matching the parsed body.

**Acceptance Scenarios**:

1. **Given** `format: "json"` and a JSON response body, **When** the assertion runs, **Then** `result` contains the parsed object
2. **Given** `format: "json"` and `assertion: "result['uptime'] > 0"`, **When** the field is present and positive, **Then** the check is healthy
3. **Given** `format: "json"` and a non-JSON body, **When** the check runs, **Then** the check is unhealthy before the assertion runs
4. **Given** a script that also sets a variable named `result`, **When** the outcome is extracted, **Then** a script-set `result` dict with `valid`/`healthy` keys takes priority over the pre-injected parsed input

---

### Edge Cases

- What if the assertion string is empty? Treated as no assertion; no validation runs.
- What if a script sets both `valid` and a `result` dict? The `result` dict with `valid`/`healthy` keys takes priority.
- What if `result` is injected as parsed input but the script does not set it? The injected value is available as input data only, not used for outcome extraction.
- What if `message` is not set in a full script? The result message falls back to a default outcome description.
- What if the script starts with `import`? Treated as a full script.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST treat a single-line string with no `valid =`, `healthy =`, `message =`, `def `, or leading `import` as a simple boolean expression
- **FR-002**: System MUST treat all other assertion strings as full scripts
- **FR-003**: System MUST evaluate simple expressions as boolean values directly
- **FR-004**: For full scripts, system MUST extract the outcome in this priority order: (1) a `result` dict set by the script containing `valid` or `healthy`; (2) global `valid` (falling back to `healthy`); (3) global `message`
- **FR-005**: System MUST NOT use a pre-injected `result` variable (parsed input) as the script outcome
- **FR-006**: System MUST inject check-type-specific variables before running the assertion (see per-check specs for variable lists)
- **FR-007**: System MUST mark the check unhealthy when an assertion raises an error, surfacing the error separately
- **FR-008**: When `format` is set on the check, system MUST parse the raw input and inject it as `result` before the assertion runs
- **FR-009**: System MUST mark the check unhealthy and skip the assertion when `format` is set but parsing fails

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An operator can add custom validation to any check with a one-line expression and no boilerplate
- **SC-002**: An operator can express multi-condition logic with branching and custom failure messages in a full script
- **SC-003**: An operator can validate JSON/XML content by accessing it as structured data rather than parsing it manually in the assertion
- **SC-004**: Assertion errors are clearly distinguishable from validation failures in the check result
