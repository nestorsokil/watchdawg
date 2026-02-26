import threading
from flask import Flask

app = Flask(__name__)

_lock = threading.Lock()
_fail_count = 0

class HealthcheckTarget:
    def fail_next(self, amount=1):
        global _fail_count
        with _lock:
            _fail_count += amount



@app.route("/target/health", methods=["GET"])
def health():
    global _fail_count
    with _lock:
        if _fail_count > 0:
            _fail_count -= 1
            return "Service Unavailable", 503
    return "OK", 200    


def reset():
        global _fail_count
        with _lock:
            _fail_count = 0


def main():
    app.run(host="0.0.0.0", port=8080, debug=False)
