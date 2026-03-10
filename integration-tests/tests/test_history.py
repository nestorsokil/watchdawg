"""
Integration tests for execution history recording and API (T024, T025, T026).

Config: history.record_all_healthchecks=true, db_path=/tmp/watchdawg-history.db
Checks under test: dynamic_check (explicit record:true), grpc_server_health (via record_all)
"""
from assertpy import assert_that
from utils import wait_for


# --- T024: Recording (per-check record:true) ---

def test_history_records_are_written(received_webhooks, history_api):
    """Records are written for a check with record:true; all fields are present and correct."""
    received_webhooks.expect_success("dynamic_check")

    wait_for(
        lambda: len(history_api.get_check("dynamic_check")) >= 1,
        description="at least 1 history record for dynamic_check",
    )

    records = history_api.get_check("dynamic_check")
    assert_that(records).is_not_empty()

    record = records[0]
    assert_that(record).contains_key("timestamp", "healthy", "duration_ms", "error")
    assert_that(record["healthy"]).is_true()
    assert_that(record["duration_ms"]).is_greater_than_or_equal_to(0)
    assert_that(record["timestamp"]).is_not_empty()  # RFC3339 string


def test_history_records_failure(received_webhooks, healthcheck_target, history_api):
    """A failed execution is recorded with healthy=false and a non-empty error field."""
    healthcheck_target.fail_next("dynamic_check", amount=1)
    received_webhooks.expect_failure("dynamic_check")

    wait_for(
        lambda: any(not r["healthy"] for r in history_api.get_check("dynamic_check")),
        description="at least 1 failure record for dynamic_check",
    )

    failure_records = [r for r in history_api.get_check("dynamic_check") if not r["healthy"]]
    assert_that(failure_records).is_not_empty()

    record = failure_records[0]
    assert_that(record["healthy"]).is_false()
    assert_that(record["error"]).is_not_empty()


def test_history_records_are_newest_first(received_webhooks, history_api):
    """Records are returned in reverse-chronological order (newest first)."""
    # Wait for at least 2 records to verify ordering
    received_webhooks.expect_success("dynamic_check", count=2)

    wait_for(
        lambda: len(history_api.get_check("dynamic_check")) >= 2,
        description="at least 2 records for ordering check",
    )

    records = history_api.get_check("dynamic_check")
    for i in range(1, len(records)):
        assert_that(records[i - 1]["timestamp"] >= records[i]["timestamp"]).is_true()


# --- T025: record_all_healthchecks=true ---

def test_history_record_all_healthchecks(history_api):
    """With record_all_healthchecks=true, every check produces history records."""
    # grpc_server_health has no explicit record:true but is covered by record_all
    wait_for(
        lambda: len(history_api.get_check("grpc_server_health")) >= 1,
        description="at least 1 record for grpc_server_health (via record_all)",
    )

    all_checks = history_api.get_all()
    # All scheduled checks should appear in the history map
    for check_name in ("dynamic_check", "grpc_server_health", "grpc_service_health"):
        assert_that(all_checks).contains_key(check_name)
        assert_that(all_checks[check_name]).is_not_empty()


# --- T026: History REST API ---

def test_history_api_check_200(received_webhooks, history_api):
    """GET /history/{check_name} returns 200 with records for a known check."""
    received_webhooks.expect_success("dynamic_check")
    wait_for(
        lambda: len(history_api.get_check("dynamic_check")) >= 1,
        description="history record available for dynamic_check",
    )

    resp = history_api.get_check_raw("dynamic_check")
    assert_that(resp.status_code).is_equal_to(200)

    body = resp.json()
    assert_that(body).contains_key("checks")
    assert_that(body["checks"]).contains_key("dynamic_check")
    assert_that(body["checks"]["dynamic_check"]).is_not_empty()


def test_history_api_check_404(history_api):
    """GET /history/{check_name} returns 404 with error message for an unknown check."""
    resp = history_api.get_check_raw("nonexistent-check-xyz")
    assert_that(resp.status_code).is_equal_to(404)

    body = resp.json()
    assert_that(body).contains_key("error")
    assert_that(body["error"]).is_not_empty()


def test_history_api_check_limit(received_webhooks, history_api):
    """GET /history/{check_name}?limit=1 returns at most 1 record."""
    received_webhooks.expect_success("dynamic_check", count=2)
    wait_for(
        lambda: len(history_api.get_check("dynamic_check")) >= 2,
        description="at least 2 records before limit test",
    )

    records = history_api.get_check("dynamic_check", limit=1)
    assert_that(records).is_length(1)


def test_history_api_all_200(history_api):
    """GET /history/* returns 200 with a map of all recorded checks."""
    wait_for(
        lambda: len(history_api.get_all()) >= 1,
        description="at least 1 check in /history/*",
    )

    all_checks = history_api.get_all()
    assert_that(all_checks).is_instance_of(dict)
    assert_that(all_checks).is_not_empty()
    assert_that(all_checks).contains_key("dynamic_check")


def test_history_api_all_per_check_limit(history_api):
    """GET /history/*?limit=1 returns at most 1 record per check."""
    wait_for(
        lambda: any(
            len(records) >= 2
            for records in history_api.get_all().values()
        ),
        description="at least one check with 2+ records",
    )

    all_checks = history_api.get_all(limit=1)
    for check_name, records in all_checks.items():
        assert_that(len(records)).described_as(
            f"records for {check_name} with limit=1"
        ).is_less_than_or_equal_to(1)
