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

    def _matching(self, results, check_name):
        return [r for r in results if r['json'] and r['json'].get('check_name') == check_name]

    def expect_success(self, check_name, timeout=30):
        wait_for(
            lambda: len(self._matching(self.success(), check_name)) >= 1,
            timeout=timeout,
            description=f"on_success webhook for '{check_name}'",
        )
        matching = self._matching(self.success(), check_name)
        logging.info(f"Received {len(matching)} success webhook(s) for check '{check_name}'")
        for result in matching:
            assert_that(result['json']['healthy']).is_true()
            assert_that(result['json']['check_name']).is_equal_to(check_name)
        return matching

    def expect_failure(self, check_name, timeout=30):
        wait_for(
            lambda: len(self._matching(self.failure(), check_name)) >= 1,
            timeout=timeout,
            description=f"on_failure webhook for '{check_name}'",
        )
        matching = self._matching(self.failure(), check_name)
        logging.info(f"Received {len(matching)} failure webhook(s) for check '{check_name}'")
        for result in matching:
            assert_that(result['json']['healthy']).is_false()
            assert_that(result['json']['check_name']).is_equal_to(check_name)
        return matching


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
