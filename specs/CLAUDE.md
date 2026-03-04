# Specs

## File naming

Spec files are named `<type>-<subject>.md`. Current types:

- `feature` — a user-facing capability (e.g. `feature-http-check.md`)

---

Specs describe **what** a feature does and **what contract it exposes** to
callers. They are the source of truth for implementing and verifying a feature.

---

## What belongs in a spec

**Include** anything that is observable or contractual:

- The conditions under which the feature succeeds or fails
- Configuration fields: what each field controls, its type, defaults, and valid values
- Ordering and precedence when multiple rules apply
- What callers receive: result shape at a semantic level (what each field means)
- Distinctions between error categories when they are meaningful to callers (e.g. "infrastructure error" vs "validation failure")
- The public interface of embedded scripting or expression languages (variables injected, return conventions)

**Exclude** anything that is an implementation detail:

- Exact error message strings
- Internal log levels, log fields, or what gets logged
- Language-specific types, struct names, or field tags
- Library or API choices (which HTTP client, which TLS flag, etc.)
- Code structure (function names, method signatures, packages)

When in doubt: if changing the detail would not break a caller or a behavioral
test, it does not belong in the spec.

---

## Structure

A spec does not need to follow a rigid template. Use the sections that are
relevant to the feature. Common ones:

**Configuration** — list fields with their type, whether they are required, and
what they do. A realistic JSON example helps.

**Execution / Behavior** — describe what happens when the feature runs, in the
order it happens. Focus on observable effects and failure conditions.

**Result** — describe what the caller receives. Explain the meaning of each
field, not just its name.

**Examples** — short, concrete config snippets illustrating distinct use cases.
One example per meaningful variation.

---

## Tone and precision

Be precise where behavior is observable. Be silent where it is not.

Prefer prose over tables for behavior; prefer tables over prose for field
reference. Use examples to make abstract rules concrete.

Write in the present tense: *"the check fails"*, not *"the check will fail"*.
