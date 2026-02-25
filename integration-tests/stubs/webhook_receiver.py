import threading
from flask import Flask, request

app = Flask(__name__)

_lock = threading.Lock()
SUCCESS = []
FAILURE = []


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


def get_success():
    with _lock:
        return list(SUCCESS)


def get_failure():
    with _lock:
        return list(FAILURE)


def clear():
    with _lock:
        SUCCESS.clear()
        FAILURE.clear()


def main():
    app.run(host="0.0.0.0", port=9090, debug=False)
