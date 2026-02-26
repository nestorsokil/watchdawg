"""Kafka stub for integration tests.

Starts background consumer threads that listen on the on_success and on_failure
hook topics. Provides a producer helper to send messages to the target topic
that WatchDawg monitors.

State is module-level so the session fixture owns the lifetime.
"""
import json
import threading

from kafka import KafkaConsumer, KafkaProducer

BROKERS = ["kafka:9092"]
TARGET_TOPIC = "watchdawg-target"
SUCCESS_TOPIC = "watchdawg-success"
FAILURE_TOPIC = "watchdawg-failure"

_lock = threading.Lock()
_success_messages = []
_failure_messages = []


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


def send_target_message(value="ping"):
    """Produce a message to the target topic that WatchDawg is monitoring."""
    producer = KafkaProducer(bootstrap_servers=BROKERS)
    producer.send(TARGET_TOPIC, value.encode("utf-8"))
    producer.flush()
    producer.close()


def get_success():
    with _lock:
        return list(_success_messages)


def get_failure():
    with _lock:
        return list(_failure_messages)


def clear():
    with _lock:
        _success_messages.clear()
        _failure_messages.clear()


def main():
    """Start consumers and block. Used as a daemon thread entry point."""
    start()
    # Block indefinitely so the calling thread stays alive.
    threading.Event().wait()
