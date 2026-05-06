# Quality Guidelines

> Code quality standards for backend development.

---

## Overview

<!--
Document your project's quality standards here.

Questions to answer:
- What patterns are forbidden?
- What linting rules do you enforce?
- What are your testing requirements?
- What code review standards apply?
-->

(To be filled by the team)

---

## Forbidden Patterns

<!-- Patterns that should never be used and why -->

(To be filled by the team)

---

## Required Patterns

<!-- Patterns that must always be used -->

(To be filled by the team)

---

## Testing Requirements

<!-- What level of testing is expected -->

### Scenario: Local external-API benchmark CLI tools

#### 1. Scope / Trigger

- Trigger: adding a local benchmark/evaluation command for a third-party API, especially when credentials, multiple keys, or live network calls are involved.
- Scope: benchmark commands are allowed to call external services at runtime, but unit tests must not call live APIs.

#### 2. Signatures

- CLI command pattern:
  - `grok-search cli <provider>-bench --cases <path> --json`
  - Optional local credential input: `--keys-file <path>`
  - Optional surface selection: `--surfaces <comma-separated-list>`
- Provider clients belong under `internal/engine/` when they are pure REST clients reusable by CLI or future tools.
- CLI orchestration belongs under `internal/cli/` and must not add the provider into production MCP routing unless the task explicitly requires that.

#### 3. Contracts

- Secrets:
  - Accept secrets only from environment variables or local files intentionally supplied by the user.
  - Do not add sample secrets to repo files.
  - Never print full API keys; CLI JSON/text output must use masked key status only.
- Cases:
  - Sample cases in `docs/` must be runnable examples without credentials.
  - Cases should be data-only JSON so users can adjust inputs without recompiling.
- Output:
  - Benchmark commands must support `--json` with stable machine-readable fields.
  - Include enough fields for comparison: case name, surface, status, latency, result counts or content lengths, and error text/code.

#### 4. Validation & Error Matrix

| Condition | Required behavior |
| --- | --- |
| No keys configured | Return usage/runtime error without network calls |
| Key file unreadable or invalid JSON | Return a clear file/config error |
| Empty benchmark case set | Return validation error |
| Unknown surface name | Return validation error |
| Upstream non-2xx response | Include status and redacted response body |
| Upstream echoes a secret in an error body | Redact the configured secret before surfacing error |
| Per-item upstream failures in a 200 response | Preserve provider-specific error code/message in JSON output |

#### 5. Good/Base/Bad Cases

- Good: `grok-search cli tinyfish-bench --cases docs/tinyfish-benchmark-cases.sample.json --json` with keys supplied via env/local file.
- Base: run a single surface with `--surfaces fetch` to isolate one provider capability.
- Bad: committing a JSON file containing real API keys, or routing benchmark traffic through MCP `web_search`/`web_fetch` without an explicit production integration task.

#### 6. Tests Required

- Request construction:
  - method, endpoint path, query/body fields, and auth header.
- Response parsing:
  - success bodies, structured per-item errors, optional metadata fields.
- Secret handling:
  - output masking and upstream error-body redaction.
- CLI/config:
  - env key loading, local key file loading, case validation, surface parsing.

#### 7. Wrong vs Correct

Wrong:

```go
fmt.Printf("key=%s error=%s\n", apiKey, upstreamBody)
```

Correct:

```go
fmt.Printf("key=%s error=%s\n", keyStatus(apiKey), redact(upstreamBody, apiKey))
```

---

## Code Review Checklist

<!-- What reviewers should check -->

(To be filled by the team)
