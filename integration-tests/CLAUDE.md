# Integration Tests

## Architecture

Tests run inside Docker Compose as the `integration-tests` service (profile: `test`). Two in-process Flask stubs start as daemon threads via session fixtures in `conftest.py` and are accessible from Watchdawg at `integration-tests:<port>`.

### Stubs

| Stub | Port | Path | Purpose |
|------|------|------|---------|
| `healthcheck_target` | 8080 | `GET /target/health` | HTTP endpoint Watchdawg checks |
| `webhook_receiver` | 9090 | `POST /callback/success` / `POST /callback/failure` | Captures webhook notifications |

### Fixture Pattern

Each stub follows the same three-layer pattern:
1. **Module** (`stubs/<name>.py`) — Flask app with thread-safe state and control functions
2. **Wrapper class** (`stubs/<name>_helpers.py`) — thin object passed to tests
3. **Fixtures** (`conftest.py`) — session fixture starts the thread; function fixture resets state before/after each test

The module is imported with a `_mod` alias to avoid naming conflicts with the fixture:
```python
from stubs import healthcheck_target as _healthcheck_target_mod

@pytest.fixture
def healthcheck_target():
    _healthcheck_target_mod.reset()
    yield HealthcheckTarget(_healthcheck_target_mod)
    _healthcheck_target_mod.reset()
```

## Writing Tests

```python
def test_example(received_webhooks, healthcheck_target):
    received_webhooks.expect_success("dynamic_check")   # wait for a success webhook
    healthcheck_target.fail_next(amount=1)              # next N checks return 503
    received_webhooks.expect_failure("dynamic_check")   # wait for a failure webhook
```

`expect_success` / `expect_failure` poll with a 30s timeout and assert on `check_name` and `healthy`.

## Adding a New Stub

1. Create `stubs/<name>.py` — Flask app + state control functions + `main()`
2. Create `stubs/<name>_helpers.py` — wrapper class exposing test-facing methods
3. Export the wrapper from `stubs/__init__.py`
4. Add session + function fixtures to `conftest.py`
