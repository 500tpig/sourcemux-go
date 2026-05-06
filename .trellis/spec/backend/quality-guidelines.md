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

### Scenario: Production external-API fallback routing

#### 1. Scope / Trigger

- Trigger: adding a third-party provider to production MCP or CLI routing, especially when credentials, multiple keys, live network calls, or fallback ordering are involved.
- Scope: production fallback integrations may call external services at runtime, but unit tests must use local test servers or fakes and must not call live APIs.

#### 2. Signatures

- Config file shape:
  - `~/.config/grok-search/config.json`
  - Provider block pattern: `{ "enabled": true, "keys": [{"name": "...", "apiKey": "..."}], "<surface>URL": "..." }`
- Env override pattern:
  - `<PROVIDER>_ENABLED`
  - `<PROVIDER>_API_KEYS` for comma-separated multi-key pools
  - `<PROVIDER>_API_KEY` for single-key compatibility when useful
  - `<PROVIDER>_KEY_NAMES` for optional display names
  - `<PROVIDER>_<SURFACE>_URL` for endpoint overrides
- Runtime placement:
  - Config parsing belongs in `internal/config/`.
  - Reusable provider REST clients and key pools belong in `internal/engine/`.
  - MCP routing belongs in `internal/tools/`.
  - CLI routing should mirror MCP routing when the CLI command represents the same user-facing capability.

#### 3. Contracts

- Secrets:
  - Accept production API keys only from environment variables or user-local config files.
  - Do not commit real keys, generated key files, or provider dashboard exports.
  - Diagnostic tools may show key counts and masked key status only.
- Routing:
  - Preserve existing behavior when the provider is disabled or has no keys.
  - Insert new fallbacks at an explicit point in the chain and update README routing diagrams.
  - Source-first search providers must return an engine envelope plus source URLs for `get_sources`.
  - Fetch providers must return clean content and a source label; empty provider content is a fallback signal, not a success.
- Multi-key pools:
  - Start requests on a rotating key for basic fairness.
  - Try the remaining configured keys on upstream errors, rate limits, or empty provider results.
  - Aggregate per-key failures without leaking full keys.

#### 4. Validation & Error Matrix

| Condition | Required behavior |
| --- | --- |
| Provider disabled | Skip provider without error |
| Provider enabled but no keys | Skip provider without network calls |
| Blank key in config/env list | Ignore during normalization |
| Missing optional key name | Fill stable generated name such as `key-N` or `env-N` |
| Upstream 429 | Try another configured key before falling through to the next provider |
| Upstream 401/403/5xx or network error | Treat as key/provider failure and continue fallback routing |
| Provider returns 200 with empty content/results | Treat as fallback signal |
| Upstream body echoes a secret | Redact before surfacing errors |
| All provider keys fail | Continue to next provider in the route when available |

#### 5. Good/Base/Bad Cases

- Good: `web_fetch` tries `Jina Reader -> TinyFish Fetch -> Exa Contents -> Tavily Extract`, and TinyFish is skipped when no keys are configured.
- Base: one valid provider key succeeds and returns a clearly labeled source such as `Source: TinyFish Fetch (acct-a)`.
- Bad: printing a full API key in `get_config_info`, making live provider calls in unit tests, or adding MCP fallback without updating the matching CLI route and README.

#### 6. Tests Required

- Config:
  - file config loading
  - env override loading
  - blank key normalization and generated names
- Engine:
  - request construction and auth header
  - response parsing
  - multi-key fallback after upstream error/rate limit
  - rotation of starting key across calls
  - secret redaction in upstream error bodies
- Tools/CLI:
  - source URL caching for search fallbacks
  - output source labels for fetch fallbacks
  - disabled/unconfigured provider preserves previous behavior

#### 7. Wrong vs Correct

Wrong:

```go
client := engine.NewProviderClient(cfg.ProviderKeys[0].APIKey)
result, err := client.Fetch(ctx, url)
```

Correct:

```go
pool := engine.NewProviderPool(cfg.ProviderKeys, cfg.ProviderFetchURL)
result, err := pool.Fetch(ctx, request)
```

---

## Code Review Checklist

<!-- What reviewers should check -->

(To be filled by the team)
