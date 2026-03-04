import threading
from flask import Flask

app = Flask(__name__)

_lock = threading.Lock()
_fail_counts: dict[str, int] = {}


class HealthcheckTarget:
    def fail_next(self, check_name: str, amount: int = 1):
        with _lock:
            _fail_counts[check_name] = _fail_counts.get(check_name, 0) + amount


@app.route("/target/health/<check_name>", methods=["GET"])
def health(check_name):
    with _lock:
        if _fail_counts.get(check_name, 0) > 0:
            _fail_counts[check_name] -= 1
            return "Service Unavailable", 503
    return "OK", 200


def reset():
    with _lock:
        _fail_counts.clear()


def main():
    app.run(host="0.0.0.0", port=8080, debug=False)
