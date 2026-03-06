# Feature Specification: gRPC Health Check

**Feature Branch**: `002-grpc-check`
**Created**: 2026-03-06
**Status**: Implemented

## User Scenarios & Testing *(mandatory)*

### User Story 1 - gRPC Server Liveness (Priority: P1)

An operator wants to know whether a gRPC server is up and accepting requests. They don't care about a specific service — they just need to know the server is alive.

**Why this priority**: Server-level liveness is the baseline. Named service checks build on it.

**Independent Test**: Configure a check pointing at a running gRPC server. Verify it reports healthy when the server responds SERVING, and unhealthy when it is down or unreachable.

**Acceptance Scenarios**:

1. **Given** a gRPC check with a valid `target`, **When** the server responds with status `SERVING`, **Then** the check result is healthy
2. **Given** a gRPC check with a valid `target`, **When** the server responds with `NOT_SERVING`, **Then** the check result is unhealthy
3. **Given** a gRPC check with a valid `target`, **When** the server is unreachable, **Then** the check result is unhealthy and the error is surfaced
4. **Given** `retries: 2` and two transient failures followed by a successful attempt, **When** the check runs, **Then** the result is healthy and reflects the correct attempt number

---

### User Story 2 - Named Service Check (Priority: P2)

An operator wants to verify that a specific named service on a gRPC server is healthy, not just the server as a whole.

**Why this priority**: In multi-service gRPC deployments, service-level health is more meaningful than server-level health.

**Independent Test**: Configure a check with `service` set to a known service name. Verify the check passes only when that service reports SERVING.

**Acceptance Scenarios**:

1. **Given** a check with `service: "my.package.MyService"`, **When** that service responds `SERVING`, **Then** the check is healthy
2. **Given** a check with `service: "my.package.MyService"`, **When** that service responds `SERVICE_UNKNOWN`, **Then** the check is unhealthy
3. **Given** a check with no `service` configured, **When** the server is checked, **Then** a server-level health check is performed

---

### User Story 3 - TLS and Plaintext Configuration (Priority: P3)

An operator wants to connect to gRPC servers with varying TLS configurations: plaintext for internal services, standard TLS for production, and relaxed TLS for dev environments with self-signed certificates.

**Why this priority**: TLS configuration is a deployment concern that doesn't affect the core health-checking logic.

**Independent Test**: Configure checks with `plaintext: true`, default TLS, and `verify_tls: false`. Verify connections succeed in each case.

**Acceptance Scenarios**:

1. **Given** `plaintext: true`, **When** the check runs, **Then** it connects without TLS regardless of other TLS settings
2. **Given** no TLS settings, **When** the check runs, **Then** it connects with TLS and validates certificates
3. **Given** `verify_tls: false` and a server with a self-signed certificate, **When** the check runs, **Then** the connection succeeds

---

### Edge Cases

- What if the server responds with `UNKNOWN` status? Treated as unhealthy; the raw status string is included in the result.
- What if `plaintext: true` and `verify_tls: false` are both set? `plaintext` takes precedence; no TLS is used.
- What happens if the RPC call itself fails (not a connection error)? Treated as unhealthy; the error is surfaced.
- What if `timeout` expires mid-retry? Remaining retries are abandoned; the result is unhealthy.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST connect to the configured `target` (host:port) and call the gRPC Health Checking Protocol endpoint
- **FR-002**: System MUST check the health of a named service when `service` is configured; otherwise perform a server-level check
- **FR-003**: System MUST consider only `SERVING` as a healthy outcome; all other statuses are unhealthy
- **FR-004**: System MUST include the raw health status string in the result whenever the server responded
- **FR-005**: System MUST support retry behaviour with the same model as HTTP checks (retries + 1 attempts, fixed pause, early return on success)
- **FR-006**: System MUST connect without TLS when `plaintext: true`
- **FR-007**: System MUST accept self-signed or invalid TLS certificates when `verify_tls: false` (only applies when `plaintext` is false)
- **FR-008**: System MUST surface connection and RPC-level errors separately in the result

### Key Entities

- **Check Result**: `healthy`, `message`, `duration`, `attempt`, `error`, `grpc.status` (raw status string from server)

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An operator can detect a non-SERVING gRPC server within one schedule interval
- **SC-002**: An operator can distinguish between a server being down and a service being degraded by inspecting the result
- **SC-003**: An operator can monitor both internal plaintext services and external TLS-secured services with the same check type
