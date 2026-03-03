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


def test_multi_hook(received_webhooks, healthcheck_target):
    # When a check fails, all on_failure hooks should fire.
    # Use a generous fail_count so multi_hook_check is guaranteed to see at least one failure
    # even if dynamic_check (running on the same target) picks up some of the failures first.
    healthcheck_target.fail_next(amount=10)
    received_webhooks.expect_failure("multi_hook_check", count=2)


def test_grpc_server_health(received_webhooks, grpc_stub):
    received_webhooks.expect_success("grpc_server_health")
    grpc_stub.set_not_serving()
    received_webhooks.expect_failure("grpc_server_health")


def test_grpc_service_health(received_webhooks, grpc_stub):
    received_webhooks.expect_success("grpc_service_health")
    grpc_stub.set_not_serving(service="integration.TestService")
    received_webhooks.expect_failure("grpc_service_health")