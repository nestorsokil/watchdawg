import threading
import logging
import pytest

from stubs import webhook_receiver
from stubs import healthcheck_target as _healthcheck_target_mod
from stubs import kafka_hooks as _kafka_hooks_mod

logging.getLogger("kafka").setLevel(logging.WARNING)

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
    yield webhook_receiver.ReceivedWebhooks()
    webhook_receiver.clear()


@pytest.fixture
def healthcheck_target():
    _healthcheck_target_mod.reset()
    yield _healthcheck_target_mod.HealthcheckTarget()
    _healthcheck_target_mod.reset()


@pytest.fixture
def kafka_hooks():
    _kafka_hooks_mod.clear()
    yield _kafka_hooks_mod.KafkaHooks()
    _kafka_hooks_mod.clear()
