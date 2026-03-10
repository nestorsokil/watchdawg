from assertpy import assert_that
from utils import Prometheus, wait_for

prometheus = Prometheus()


def test_continuous_monitoring(received_webhooks, healthcheck_target):
    with prometheus.counter('watchdawg_check_executions_total{check="dynamic_check",result="success"}') as executions:
        received_webhooks.expect_success("dynamic_check")
        assert_that(executions.has_increased()).is_true()
        assert_that(prometheus.counter('watchdawg_check_duration_seconds_sum{check="dynamic_check"}').load()).is_greater_than(0)

    up = prometheus.gauge('watchdawg_check_up{check="dynamic_check"}')
    wait_for(lambda: up.is_not_zero())

    healthcheck_target.fail_next("dynamic_check", amount=10)
    received_webhooks.expect_failure("dynamic_check")
    wait_for(lambda: up.is_zero())
    
def test_kafka_assertion(received_webhooks, kafka_hooks):
    with prometheus.gauge('watchdawg_check_up{check="kafka_assertion"}') as up:
        kafka_hooks.send_json({"status": "ok"})
        received_webhooks.expect_success("kafka_assertion")
        wait_for(lambda: up.is_not_zero())
        
        kafka_hooks.send_json({"status": "bad"})
        received_webhooks.expect_failure("kafka_assertion")
        wait_for(lambda: up.is_zero())

        kafka_hooks.send_json({"status": "ok"})
        received_webhooks.expect_success("kafka_assertion")
        wait_for(lambda: up.is_not_zero())
        assert_that(prometheus.gauge('watchdawg_check_message_age_seconds{check="kafka_assertion"}').load()).is_greater_than_or_equal_to(0)


def test_kafka_liveness(kafka_hooks):
    up = prometheus.gauge('watchdawg_check_up{check="kafka_liveness"}')
    message_age = prometheus.gauge('watchdawg_check_message_age_seconds{check="kafka_liveness"}')

    kafka_hooks.send()
    kafka_hooks.expect_success("kafka_liveness")
    
    wait_for(lambda: up.is_not_zero())
    assert_that(message_age.load()).is_greater_than_or_equal_to(0)

    # Stop sending. After the check interval (5 s) the last message is stale →
    # Watchdawg fires the on_failure hook to the failure Kafka topic.
    # Timeout is generous to account for scheduler jitter (up to 2× interval).
    kafka_hooks.expect_failure("kafka_liveness", timeout=30)
    wait_for(lambda: up.is_zero())


def test_multi_hook(received_webhooks, healthcheck_target):
    # When a check fails, all on_failure hooks should fire.
    # Use a generous fail_count so multi_hook_check is guaranteed to see at least one failure
    # even if dynamic_check (running on the same target) picks up some of the failures first.
    with prometheus.counter(
        'watchdawg_hook_executions_total{check="multi_hook_check",'
        'result="success",'
        'target="http://integration-tests:9090/callback/failure",'
        'trigger="on_failure",'
        'type="http"}'
    ) as failed_counter:
        healthcheck_target.fail_next("multi_hook_check")
        received_webhooks.expect_failure("multi_hook_check", count=2)
        assert_that(failed_counter.has_increased(by=2)).is_true()
    with prometheus.counter(
        'watchdawg_hook_duration_seconds_sum{check="multi_hook_check",'
        'target="http://integration-tests:9090/callback/failure",'
        'trigger="on_failure",'
        'type="http"}'
    ) as hook_duration:
        assert_that(hook_duration.value).is_greater_than(0)


def test_grpc_server_health(received_webhooks, grpc_stub):
    up = prometheus.gauge('watchdawg_check_up{check="grpc_server_health"}')

    received_webhooks.expect_success("grpc_server_health")
    wait_for(lambda: up.is_not_zero())

    grpc_stub.set_not_serving()
    received_webhooks.expect_failure("grpc_server_health")
    wait_for(lambda: up.is_zero())


def test_grpc_service_health(received_webhooks, grpc_stub):
    up = prometheus.gauge('watchdawg_check_up{check="grpc_service_health"}')

    received_webhooks.expect_success("grpc_service_health")
    wait_for(lambda: up.is_not_zero())

    grpc_stub.set_not_serving(service="integration.TestService")
    received_webhooks.expect_failure("grpc_service_health")
    wait_for(lambda: up.is_zero())
