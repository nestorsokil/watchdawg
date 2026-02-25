import threading
import time
import logging
import pytest

from stubs import webhook_receiver, ReceivedWebhooks

COMPOSE_FILE = "docker-compose.yml"
WATCHDAWG_SERVICE = "watchdawg"

logger = logging.getLogger(__name__)


@pytest.fixture(scope="session", autouse=True)
def _start_webhook_receiver():
    t = threading.Thread(target=webhook_receiver.main, daemon=True)
    t.start()


@pytest.fixture
def received_webhooks():
    webhook_receiver.clear()
    yield ReceivedWebhooks(webhook_receiver)
    webhook_receiver.clear()
