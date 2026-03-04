# Assertions

Assertions are optional Starlark expressions or scripts that can be attached to
HTTP and Kafka checks to perform custom validation beyond what the built-in
`expected` fields support.

An assertion failure marks the check unhealthy. An assertion error (script crash,
parse failure) also marks the check unhealthy and surfaces the error separately
in the result.

Each check type injects its own set of variables — see the relevant check spec
for the variable list.

---

## Expressions vs. full scripts

A string is treated as a **simple expression** when all of the following are
true:

- It is a single line.
- It contains none of: `valid =`, `healthy =`, `message =`, `def `.
- It does not start with `import `.

Simple expressions are evaluated as a boolean directly — no boilerplate needed.

Everything else is a **full script**. A full script signals its result by
setting globals (see below).

---

## Full script result extraction

After a full script executes, the outcome is read in this priority order:

1. If `result` is a dict containing a `valid` or `healthy` key, use that dict.
2. Otherwise read the `valid` global (bool); fall back to `healthy` if absent.
3. Read the `message` global (string) if present.

> Note: priority (1) only applies when the script itself sets `result` as a
> validation dict. A `result` variable pre-injected by the check (parsed body
> or message value) is only used as input data, not as the outcome.

---

## Parsed input (`format`)

When a check's `format` field is set to `"json"` (or `"xml"` for HTTP), the
raw input (response body or message value) is parsed and injected as `result`
before the script runs. If parsing fails, the check is marked unhealthy and the
script does not run.

JSON objects become dicts, arrays become lists, and primitives become their
scalar equivalents.

---

## Examples

### Simple expression

```python
status_code == 200 and 'ok' in body
```

Evaluated directly as a boolean.

### Full script with `valid` and `message`

```python
if status_code == 200:
    valid = 'error' not in body
    message = 'body looks clean'
else:
    valid = False
    message = 'unexpected status: ' + str(status_code)
```

### Script returning a result dict

```python
def check():
    if result.get('status') != 'ok':
        return {'valid': False, 'message': 'bad status: ' + result.get('status', '?')}
    return {'valid': True}
```

### Parsed JSON input

With `"format": "json"`, `result` contains the parsed body:

```python
result['uptime'] > 0 and result['status'] == 'healthy'
```
