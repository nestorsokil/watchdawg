# WatchDawg

A dynamic, extensible healthchecking service built with Go that executes user-defined health checks and sends webhook notifications on success or failure.

## Features

- **Multiple Check Types**
  - HTTP health checks with customizable methods, headers, and expected status codes
  - Starlark-based checks for complex custom logic
  - Support for retries and timeouts
  - Future support planned for gRPC and Kafka

- **Flexible Scheduling**
  - Simple interval format (`30s`, `5m`, `1h`)
  - Standard cron expressions
  - Per-check scheduling configuration

- **Webhook Notifications**
  - Success and failure webhooks
  - Customizable request methods and headers
  - Template support for custom notification formats

- **Extensibility**
  - Starlark scripts for response validation
  - Starlark-only checks for complex scenarios
  - HTTP client available to Starlark scripts (future)

## Quick Start

### Prerequisites

- Go 1.25 or higher

### Installation

```bash
# Clone the repository
git clone <your-repo-url>
cd watchdawg

# Build
go build -o bin/watchdawg ./cmd/watchdawg
```

### Basic Usage

1. Create a configuration file (or use the example):

```bash
cp config.example.json config.json
# Edit config.json with your health checks
```

2. Run WatchDawg:

```bash
./bin/watchdawg -config config.json
```

## Configuration

### Loading Config

WatchDawg supports three config sources:

**File (default)**
```bash
./bin/watchdawg -config configs/config.json
```

**Stdin** — pipe from anywhere
```bash
# From a remote URL
curl -s https://example.com/watchdawg.json | ./bin/watchdawg -config -

# From a secret manager
vault kv get -field=config secret/watchdawg | ./bin/watchdawg -config -

# From a YAML file (requires yq)
yq -o=json configs/config.example.yaml | ./bin/watchdawg -config -
```

A YAML reference config is available at `configs/config.example.yaml`.

**Environment variable substitution** — use `$VAR` or `${VAR}` anywhere in the JSON:
```json
{
  "http": {
    "url": "https://${API_HOST}/health",
    "headers": { "Authorization": "Bearer $API_TOKEN" }
  }
}
```
Variables are expanded from the process environment before parsing. Unset variables expand to an empty string.

### Basic Structure

```json
{
  "healthchecks": [
    {
      "name": "my-api-check",
      "type": "http",
      "schedule": "30s",
      "retries": 2,
      "timeout": 5000000000,
      "http": { /* HTTP config */ },
      "on_success": { /* webhook config */ },
      "on_failure": { /* webhook config */ }
    }
  ]
}
```

### Check Types

#### HTTP Checks

Basic HTTP check:
```json
{
  "name": "api-health",
  "type": "http",
  "schedule": "1m",
  "retries": 3,
  "timeout": 10000000000,
  "http": {
    "url": "https://api.example.com/health",
    "method": "GET",
    "headers": {
      "Authorization": "Bearer token123"
    },
    "expected": {
      "status_code": 200
    }
  }
}
```

HTTP check with multiple acceptable status codes:
```json
{
  "name": "api-with-validation",
  "type": "http",
  "schedule": "1m",
  "retries": 3,
  "timeout": 10000000000,
  "http": {
    "url": "https://api.example.com/data",
    "method": "GET",
    "expected": {
      "status_code": [200, 201, 202]
    },
    "assertion": "'success' in body"
  }
}
```

HTTP check with JSON parsing and simple expression:
```json
{
  "name": "api-json-validation",
  "type": "http",
  "schedule": "1m",
  "retries": 2,
  "timeout": 10000000000,
  "http": {
    "url": "https://api.example.com/product",
    "method": "GET",
    "expected": {
      "status_code": 200,
      "format": "json"
    },
    "assertion": "result['product_sku'] == 10012"
  }
}
```

HTTP check with header validation and assertion:
```json
{
  "name": "api-with-headers-check",
  "type": "http",
  "schedule": "1m",
  "retries": 3,
  "timeout": 10000000000,
  "http": {
    "url": "https://api.example.com/data",
    "method": "GET",
    "expected": {
      "status_code": 200,
      "format": "json",
      "headers": {
        "Content-Type": "application/json"
      }
    },
    "assertion": "result['status'] == 'ok' and result.get('count', 0) > 5"
  }
}
```

HTTP check with self-signed certificate (TLS verification disabled):
```json
{
  "name": "dev-api-health",
  "type": "http",
  "schedule": "1m",
  "retries": 2,
  "timeout": 10000000000,
  "http": {
    "url": "https://dev-api.example.com/health",
    "method": "GET",
    "expected": {
      "status_code": 200,
      "verify_tls": false
    }
  }
}
```

#### Expected Response Configuration

The `expected` object defines what constitutes a successful response:

```json
"expected": {
  "status_code": 200,           // Required: Single status code
  // OR
  "status_code": [200, 201, 202], // Required: Array of acceptable codes

  "format": "json",             // Optional: "json" or "xml" - auto-parses body
  "headers": {                  // Optional: Expected response headers
    "Content-Type": "application/json"
  },
  "verify_tls": false           // Optional: Set to false to skip TLS cert validation
}
```

**All fields except `status_code` are optional:**
- **`status_code`** (required): Expected HTTP status code - can be a single integer (e.g., `200`) or an array of acceptable codes (e.g., `[200, 201, 202]`)
- **`format`** (optional): Response format - `"json"` or `"xml"`. When set, body is parsed and available as `result` in assertions
- **`headers`** (optional): Map of expected headers that must match exactly
- **`verify_tls`** (optional): TLS certificate verification (default: `true`). Set to `false` to skip certificate validation - useful for self-signed certificates in dev/test environments

**Note:** Response time limits are controlled by the `timeout` field at the health check level, not in the `expected` object.

#### Assertion Modes

The `assertion` field supports three modes:

**1. Simple Expression (recommended for basic checks)**
```json
"assertion": "status_code == 200 and 'success' in body"
```
- Single-line boolean expression
- Automatically wrapped as `valid = <expression>`
- No need to set variables

**2. Simple Expression with Parsed Data**
```json
"expected": {
  "status_code": 200,
  "format": "json"
},
"assertion": "result['status'] == 'healthy'"
```
- Use `format` to automatically parse JSON/XML responses
- Access parsed data via the `result` variable
- Clean, simple syntax for common validations

**3. Full Starlark Script (for complex logic)**
```json
"assertion": "valid = status_code == 200\nif valid:\n  message = 'Success'\nelse:\n  message = 'Failed'"
```
- Multi-line scripts with full Starlark capabilities
- Must explicitly set `valid` or `healthy` variables
- Optional `message` variable for custom messages

#### Available Variables in Assertions

Without `format`:
- `status_code` (int): HTTP status code
- `body` (string): Response body
- `body_size` (int): Size of response body in bytes
- `headers` (dict): Response headers

With `format: "json"` or `"xml"`:
- All above variables, plus:
- `result` (dict/list): Parsed JSON/XML response body

#### Starlark Checks

```json
{
  "name": "custom-logic",
  "type": "starlark",
  "schedule": "2m",
  "retries": 1,
  "timeout": 15000000000,
  "starlark": {
    "script": "healthy = True\nmessage = 'Check passed'\n",
    "globals": {
      "threshold": 100,
      "api_url": "https://api.example.com"
    }
  }
}
```

### Scheduling

WatchDawg supports two schedule formats:

1. **Interval format**: `30s`, `5m`, `1h`, `2h30m`
2. **Cron format**: `0 */5 * * * *` (with seconds)

### Webhook Notifications

```json
{
  "on_success": {
    "url": "https://webhook.site/your-webhook",
    "method": "POST",
    "headers": {
      "Content-Type": "application/json"
    },
    "body_template": "Check {{.CheckName}} passed at {{.Timestamp}}"
  },
  "on_failure": {
    "url": "https://webhook.site/your-webhook",
    "method": "POST",
    "body_template": "ALERT: {{.CheckName}} failed: {{.Message}}"
  }
}
```

If `body_template` is not provided, the full check result is sent as JSON.

## Project Structure

```
.
├── cmd/
│   └── watchdawg/              # Application entrypoint
├── internal/
│   ├── healthcheck/            # Health check executors
│   │   ├── http.go            # HTTP health checker
│   │   ├── starlark.go        # Starlark health checker
│   │   ├── webhook.go         # Webhook notifier
│   │   └── scheduler.go       # Check scheduler
│   ├── config/                 # Configuration management
│   │   └── loader.go          # Config file loader
│   └── models/                 # Data models
│       ├── config.go          # Configuration structures
│       └── result.go          # Result structures
├── config.json                 # Default configuration
├── config.example.json         # Example configuration
└── go.mod                      # Go module definition
```

## Roadmap

- [ ] Starlark HTTP client for making requests from scripts
- [ ] Metrics and monitoring endpoint
- [ ] Check result history and reporting

## License

TBD
