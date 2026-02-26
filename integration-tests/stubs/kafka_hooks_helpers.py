from utils import wait_for
from assertpy import assert_that
import logging


class KafkaHooks:
    """Test-facing wrapper for the kafka_hooks module state."""

    def __init__(self, module):
        self._mod = module

    def send(self, value="ping"):
        """Send a message to the target topic WatchDawg is monitoring."""
        self._mod.send_target_message(value)

    def success(self):
        return self._mod.get_success()

    def failure(self):
        return self._mod.get_failure()

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
