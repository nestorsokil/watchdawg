---
name: integration-test
description: Run WatchDawg integration tests using Docker Compose
user-invocable: true
allowed-tools: Bash
---

Run the WatchDawg integration test suite. Tests run inside a Docker container alongside nginx and watchdawg.

## Steps

1. **Confirm** with the user before proceeding, since this starts Docker containers.

2. **Run tests** from the project root — this builds images, starts all dependencies (nginx, watchdawg), and runs pytest:
   ```
   docker compose up --rm \
            --abort-on-container-exit \
            --exit-code-from=integration-tests \
            integration-tests
   ```

3. **Tear down** all services when done (pass or fail):
   ```
   docker compose down
   ```

## Notes

- Run from the project root (`/Users/nestorsokil/Data/Code/watchdawg`)
- `--rm` removes the integration-tests container after it exits
- If `docker compose run` fails, report the error and still run `docker compose down`
- Do not retry failing tests; stop and report results
