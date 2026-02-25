import time



def wait_for(condition, timeout=30, interval=1, description="condition"):
    deadline = time.time() + timeout
    while time.time() < deadline:
        if condition():
            return True
        time.sleep(interval)
    raise TimeoutError(f"Timed out waiting for: {description}")