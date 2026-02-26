import threading
from flask import Flask, request
from utils import wait_for
from assertpy import assert_that
import logging

app = Flask(__name__)

_lock = threading.Lock()
SUCCESS = []
FAILURE = []

class ReceivedWebhooks:
    def success(self):
        with _lock:
            return list(SUCCESS)

    def failure(self):
        with _lock:
            return list(FAILURE)

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


@app.route("/callback/success", methods=["POST"])
def receive_success():
    with _lock:
        SUCCESS.append({
            "headers": dict(request.headers),
            "body": request.get_data(as_text=True),
            "json": request.get_json(silent=True),
        })
    return "", 200


@app.route("/callback/failure", methods=["POST"])
def receive_failure():
    with _lock:
        FAILURE.append({
            "headers": dict(request.headers),
            "body": request.get_data(as_text=True),
            "json": request.get_json(silent=True),
        })
    return "", 200


def clear():
    with _lock:
        SUCCESS.clear()
        FAILURE.clear()


def main():
    app.run(host="0.0.0.0", port=9090, debug=False)
