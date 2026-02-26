import threading
import logging
import pytest

from stubs import webhook_receiver, ReceivedWebhooks, HealthcheckTarget, KafkaHooks
from stubs import healthcheck_target as _healthcheck_target_mod
from stubs import kafka_hooks as _kafka_hooks_mod

logger = logging.getLogger(__name__)


@pytest.fixture(scope="session", autouse=True)
def _start_webhook_receiver():
    t = threading.Thread(target=webhook_receiver.main, daemon=True)
    t.start()


@pytest.fixture(scope="session", autouse=True)
def _start_healthcheck_target():
    t = threading.Thread(target=_healthcheck_target_mod.main, daemon=True)
    t.start()


@pytest.fixture(scope="session", autouse=True)
def _start_kafka_consumers():
    _kafka_hooks_mod.start()


@pytest.fixture
def received_webhooks():
    webhook_receiver.clear()
    yield ReceivedWebhooks(webhook_receiver)
    webhook_receiver.clear()


@pytest.fixture
def healthcheck_target():
    _healthcheck_target_mod.reset()
    yield HealthcheckTarget(_healthcheck_target_mod)
    _healthcheck_target_mod.reset()


@pytest.fixture
def kafka_hooks():
    _kafka_hooks_mod.clear()
    yield KafkaHooks(_kafka_hooks_mod)
    _kafka_hooks_mod.clear()
