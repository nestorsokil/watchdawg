"""Kafka stub for integration tests.

Starts background consumer threads that listen on the on_success and on_failure
hook topics. Provides a producer helper to send messages to the target topic
that WatchDawg monitors.

State is module-level so the session fixture owns the lifetime.
"""
import json
import threading
import logging
from utils import wait_for
from assertpy import assert_that
from kafka import KafkaConsumer, KafkaProducer

BROKERS = ["kafka:9092"]
TARGET_TOPIC = "watchdawg-target"
SUCCESS_TOPIC = "watchdawg-success"
FAILURE_TOPIC = "watchdawg-failure"

_lock = threading.Lock()
_success_messages = []
_failure_messages = []

class KafkaHooks:
    """Test-facing wrapper for the kafka_hooks module state."""
    def send(self, value="ping"):
        producer = KafkaProducer(bootstrap_servers=BROKERS)
        producer.send(TARGET_TOPIC, value.encode("utf-8"))
        producer.flush()
        producer.close()

    def success(self):
        with _lock:
            return list(_success_messages)

    def failure(self):
        with _lock:
            return list(_failure_messages)

    def expect_success(self, check_name, timeout=30):
        wait_for(
            lambda: len(self.success()) >= 1,
            timeout=timeout,
            description=f"kafka on_success hook for '{check_name}'",
        )
        messages = self.success()
        logging.info(f"Received kafka success hook for check '{check_name}'")
        assert_that(len(messages)).is_greater_than_or_equal_to(1)
        latest = messages[-1]
        assert_that(latest["healthy"]).is_true()
        assert_that(latest["check_name"]).is_equal_to(check_name)
        return messages

    def expect_failure(self, check_name, timeout=30):
        wait_for(
            lambda: len(self.failure()) >= 1,
            timeout=timeout,
            description=f"kafka on_failure hook for '{check_name}'",
        )
        messages = self.failure()
        logging.info(f"Received kafka failure hook for check '{check_name}'")
        assert_that(len(messages)).is_greater_than_or_equal_to(1)
        latest = messages[-1]
        assert_that(latest["healthy"]).is_false()
        assert_that(latest["check_name"]).is_equal_to(check_name)
        return messages



def _consume_loop(topic, store):
    consumer = KafkaConsumer(
        topic,
        bootstrap_servers=BROKERS,
        auto_offset_reset="latest",
        group_id=f"integration-test-consumer-{topic}",
        consumer_timeout_ms=200,
    )
    while True:
        # consumer_timeout_ms causes StopIteration when idle; loop keeps running
        try:
            for msg in consumer:
                parsed = json.loads(msg.value.decode("utf-8"))
                with _lock:
                    store.append(parsed)
        except StopIteration:
            pass


def start():
    """Launch background consumer threads for the success and failure topics."""
    for topic, store in [(SUCCESS_TOPIC, _success_messages), (FAILURE_TOPIC, _failure_messages)]:
        t = threading.Thread(target=_consume_loop, args=(topic, store), daemon=True)
        t.start()

def clear():
    with _lock:
        _success_messages.clear()
        _failure_messages.clear()


def main():
    """Start consumers and block. Used as a daemon thread entry point."""
    start()
    # Block indefinitely so the calling thread stays alive.
    threading.Event().wait()
