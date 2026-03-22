"""Integration test: Starlark check that uses http_request.

The test configures Watchdawg with a `type: starlark` check whose script
calls http_request against the shared healthcheck_target Flask stub,
then asserts that the scheduler fires the expected webhook.

Requires Docker — do NOT run without explicit approval.
"""

from assertpy import assert_that
from utils import Prometheus, wait_for

prometheus = Prometheus()


def test_starlark_http_request_healthy(received_webhooks, healthcheck_target):
    """Starlark check using http_request succeeds when the target returns 200."""
    with prometheus.gauge('watchdawg_check_up{check="starlark_http_check"}') as up:
        received_webhooks.expect_success("starlark_http_check")
        wait_for(lambda: up.is_not_zero())


def test_starlark_http_request_unhealthy_on_target_failure(received_webhooks, healthcheck_target):
    """Starlark check using http_request reports unhealthy when target returns 503."""
    healthcheck_target.fail_next("starlark_http_check", amount=10)
    received_webhooks.expect_failure("starlark_http_check")
    up = prometheus.gauge('watchdawg_check_up{check="starlark_http_check"}')
    wait_for(lambda: up.is_zero())
