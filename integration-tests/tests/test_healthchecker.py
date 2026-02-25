def test_on_failure_webhook_is_fired(received_webhooks, healthcheck_target):
    received_webhooks.expect_success("dynamic_check")
    healthcheck_target.fail_next(amount=1)
    received_webhooks.expect_failure("dynamic_check")
