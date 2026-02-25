from utils import wait_for
from assertpy import assert_that
import logging


class ReceivedWebhooks:
    def __init__(self, webhook_receiver):
        self.webhook_receiver = webhook_receiver

    def success(self):
        return self.webhook_receiver.get_success()

    def failure(self):
        return self.webhook_receiver.get_failure()

    def expect_success(self, check_name, times_expected=1, timeout=30):
        wait_for(lambda: len(self.success()) == times_expected,
                 timeout=timeout,
                 description=f"on_success webhook for {check_name}")
        successes = self.success()
        logging.info(f"Received successful results via webhook for check '{check_name}'")
        assert_that(len(successes)).is_greater_than_or_equal_to(times_expected)
        for result in successes:
            assert_that(result['json']['healthy']).is_true()
            assert_that(result['json']['check_name']).is_equal_to(check_name)
        return successes

    def expect_failure(self, check_name=None, times_expected=1, timeout=30):
        wait_for(lambda: len(self.failure()) == times_expected,
                 timeout=timeout,
                 description=f"on_failure webhook for {check_name}")
        failures = self.failure()
        logging.info(f"Received failed results via webhook for check '{check_name}'")
        assert_that(len(failures)).is_equal_to(times_expected)
        for result in failures:
            assert_that(result['json']['healthy']).is_false()
            assert_that(result['json']['check_name']).is_equal_to(check_name)
        return failures
