def test_on_success_webhook_is_fired(received_webhooks):
    received_webhooks.expect_success("nginx-check")


def test_on_failure_webhook_is_fired(received_webhooks):
    received_webhooks.expect_failure("failing-check")
