def test_continuous_monitoring(received_webhooks, healthcheck_target):
    received_webhooks.expect_success("dynamic_check")
    healthcheck_target.fail_next(amount=1)
    received_webhooks.expect_failure("dynamic_check")


def test_kafka_assertion(received_webhooks, kafka_hooks):
    # Send a message that fails the assertion (status != 'ok')
    kafka_hooks.send_json({"status": "bad"})
    received_webhooks.expect_failure("kafka_assertion")

    # Send a corrected message that passes the assertion
    kafka_hooks.send_json({"status": "ok"})
    received_webhooks.expect_success("kafka_assertion")


def test_kafka_liveness(kafka_hooks):
    # Send a message to the target topic → WatchDawg detects a recent message →
    # fires the on_success hook to the success Kafka topic.
    kafka_hooks.send()
    kafka_hooks.expect_success("kafka_liveness")

    # Stop sending. After the check interval (5 s) the last message is stale →
    # WatchDawg fires the on_failure hook to the failure Kafka topic.
    # Timeout is generous to account for scheduler jitter (up to 2× interval).
    kafka_hooks.expect_failure("kafka_liveness", timeout=30)
