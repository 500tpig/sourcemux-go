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
  - `./grok-search.json` by default, or one explicit path supplied with `--config`.
  - Provider block pattern: `{ "enabled": true, "keys": [{"name": "...", "apiKey": "..."}], "<surface>URL": "..." }`
- Runtime config must not add environment-variable fallbacks or hidden user config files.
- Runtime placement:
  - Config parsing belongs in `internal/config/`.
  - Reusable provider REST clients and key pools belong in `internal/engine/`.
  - MCP routing belongs in `internal/tools/`.
  - CLI routing should mirror MCP routing when the CLI command represents the same user-facing capability.

#### 3. Contracts

- Secrets:
  - Accept production API keys only from the active single config file.
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
| Blank key in config list | Ignore during normalization |
| Missing optional key name | Fill stable generated name such as `key-N` |
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

### Scenario: Standalone external-API MCP and CLI surfaces

#### 1. Scope / Trigger

- Trigger: adding a user-facing MCP tool and matching CLI command that calls a third-party API directly, outside the search/fetch fallback chain.
- Scope: the surface may call the external service at runtime, but tests must use local test servers or fakes and must not call live APIs.

#### 2. Signatures

- Runtime placement:
  - Reusable REST client methods belong under `internal/engine/`.
  - MCP tool registration belongs under `internal/tools/`.
  - CLI command orchestration belongs under `internal/cli/`.
  - MCP registration is wired in `internal/server/server.go`.
- CLI command pattern:
  - `grok-search cli <capability> <url> --json`
  - Capability-specific flags should mirror MCP parameter names using CLI hyphen style, for example `max_depth` -> `--max-depth`.
- MCP tool pattern:
  - Tool name should be stable snake_case, for example `web_crawl`.
  - Required user input should be marked with `mcp.Required()`.

#### 3. Contracts

- Configuration:
  - Use the existing provider config block from the active single config file.
  - For Tavily surfaces, use the active config file's `tavily.apiKey`, `tavily.apiURL`, and `tavily.enabled`.
- Behavior:
  - If the provider is disabled or missing a required key, return a clear unavailable error without network calls.
  - Engine methods should set `Content-Type: application/json` and `Authorization: Bearer <key>`.
  - Engine methods should reuse `httpDoWithRetry` for 429/5xx and network errors.
  - Human/MCP output should be compact and LLM-readable; JSON output may preserve the full typed response.
  - When a tool has richer CLI output than MCP, keep MCP focused on metadata plus clipped content and point callers to the richer surface (`get_sources`, CLI text, or CLI `--json`) instead of returning full bodies.
- Documentation:
  - README must document the MCP tool, CLI command, required config, and how it differs from adjacent tools.

#### 4. Validation & Error Matrix

| Condition | Required behavior |
| --- | --- |
| Missing required URL/query | Return a usage/tool error before network calls |
| Provider disabled | Return unavailable error before network calls |
| Provider key missing | Return unavailable error before network calls |
| Upstream 429/5xx or transient network error | Retry via shared retry helper |
| Upstream non-retryable non-2xx | Return status code plus response body, with secrets redacted if applicable |
| Upstream 200 with empty required result set | Treat as failure and return a clear empty-result error |
| CLI `--json` requested | Emit stable machine-readable JSON with an `error` field on failure |

#### 5. Good/Base/Bad Cases

- Good: `grok-search cli crawl https://example.com/docs --instructions "Find API pages" --limit 10 --json` calls a configured Tavily test server in tests and returns typed crawl results.
- Base: the MCP tool returns a concise text envelope with source, base URL, result count, and page snippets.
- Bad: adding a CLI command that calls a live API in unit tests, or exposing a new MCP tool without documenting the matching CLI and config requirements.

#### 6. Tests Required

- Engine:
  - Request path, method, auth header, content type, and JSON body fields.
  - Success response parsing into typed structs.
  - Retry path for 429/5xx.
  - Empty input and empty response error paths.
- Tools/CLI:
  - MCP registration or formatter behavior for the new surface.
  - CLI JSON shape.
  - CLI config path using a local test server and dummy endpoint config.
- Docs:
  - README command table and MCP tool table include the new surface.

#### 7. Wrong vs Correct

Wrong:

```go
func runCrawl(args []string) int {
	resp, _ := http.Post("https://api.tavily.com/crawl", "application/json", body)
	_ = resp
	return 0
}
```

Correct:

```go
t := engine.NewTavilyClient(cfg.TavilyAPIURL, cfg.TavilyAPIKey)
result, err := t.Crawl(ctx, engine.TavilyCrawlRequest{URL: url, Limit: limit})
```

---

### Scenario: Composable research workflow surfaces

#### 1. Scope / Trigger

- Trigger: adding a higher-level MCP/CLI workflow that composes existing search, source retrieval, fetch, map, and crawl capabilities into one output pack.
- Scope: the workflow may call live providers at runtime through existing routing helpers, but unit tests must use fakes or local test servers and must not call live APIs.

#### 2. Signatures

- MCP tool:
  - Name: `research_run`
  - Inputs: `query` (required), `depth` (`quick` / `standard` / `deep`), `platform` (optional), `domains` (optional array), `max_fetches` (optional number).
- CLI command:
  - `grok-search cli research <query> --depth <quick|standard|deep> --platform <focus> --domain <domain> --max-fetches <n> --json`
  - CLI hyphen names mirror MCP snake_case fields, for example `max_fetches` -> `--max-fetches`.
- Runtime placement:
  - Pure orchestration helpers and MCP registration live under `internal/tools/`.
  - CLI parsing/wiring lives under `internal/cli/`.
  - Server registration is wired in `internal/server/server.go`.

#### 3. Contracts

- Composition:
  - Start from the existing planning logic (`BuildSearchPlan`) and parse the planned `web_search query=...` lines.
  - Execute searches through the shared web-search routing helper so provider fallback order remains unchanged.
  - Retrieve sources through the same source cache contract used by `get_sources`.
  - Fetch selected URLs through the shared web-fetch routing helper so fetch fallback order remains unchanged.
  - Use existing `web_map` / `web_crawl` clients for site expansion; do not duplicate crawler/provider code.
- Output:
  - JSON output must be stable and use empty arrays instead of `null` for list fields.
  - Required pack sections: `query`, `effective_depth`, `executed_searches`, `source_summary`, `fetched_pages_summary`, `high_signal_sources`, `confirmed_facts`, `likely_inferences`, `open_questions`.
  - Human/MCP output must be compact and LLM-readable, with fetched/crawled content clipped.
  - Even the thin MCP formatter should keep stable core section headings so hosts can rely on a predictable structure when packs are sparse or empty.
- Bounds:
  - Default `max_fetches`: quick=2, standard=4, deep=8.
  - First-version hard cap: `max_fetches <= 12`, to avoid exploding research packs and provider cost.

#### 4. Validation & Error Matrix

| Condition | Required behavior |
| --- | --- |
| Missing required query | Return usage/tool error before network calls |
| Unknown depth | Normalize to `standard` |
| `max_fetches <= 0` | Use the depth default |
| `max_fetches > 12` | Clamp to 12 and record the effective value |
| Provider disabled or unconfigured | Existing shared routing decides fallback/unavailable behavior |
| A search round fails | Preserve the failed query and error in `executed_searches`; continue with remaining searches |
| A selected fetch fails | Preserve the URL and error in `fetched_pages_summary`; downrank the source |
| No sources discovered | Return a valid pack with empty arrays and open questions, not malformed JSON |

#### 5. Good/Base/Bad Cases

- Good: `grok-search cli research "Grok Search MCP" --depth deep --domain github.com --max-fetches 6 --json` returns a bounded research pack with stable sections.
- Base: MCP `research_run` with only `query` produces planned searches, ranked sources, fetched page summaries, and heuristic facts/inferences.
- Bad: adding a separate crawler implementation for research, changing `web_search` fallback order only for research, or returning unbounded full page bodies.

#### 6. Tests Required

- Plan parsing:
  - Extract quoted and escaped `web_search query=...` values from planning output.
- URL/source handling:
  - Normalize and deduplicate URLs, strip tracking parameters, preserve meaningful query strings.
  - Rank official/repeated/relevant/recent sources above lower-signal sources.
  - Downrank failed fetches and boilerplate pages.
- Output:
  - JSON shape includes every required pack section and uses stable empty arrays.
  - Human output includes compact section headings and clipped excerpts.
- CLI:
  - Parse `--depth`, `--platform`, repeatable `--domain`, `--max-fetches`, and `--json`.

#### 7. Wrong vs Correct

Wrong:

```go
// Research path silently forks the provider route and returns full bodies.
res, _ := tavily.Search(ctx, query)
fmt.Println(res.Answer + fullFetchedPage)
```

Correct:

```go
pack, err := executor.Run(ctx, tools.ResearchOptions{
	Query:      query,
	Depth:      depth,
	Domains:    domains,
	MaxFetches: maxFetches,
})
fmt.Println(tools.FormatResearchPack(pack))
```

---

### Scenario: Evidence-grounded reasoning synthesis surfaces

#### 1. Scope / Trigger

- Trigger: adding a model-powered synthesis layer that consumes search/fetch/research evidence and calls a non-search LLM provider such as DeepSeek.
- Scope: synthesis providers may call external OpenAI-compatible APIs at runtime, but they must not change `web_search` / `research_run` evidence collection semantics.

#### 2. Signatures

- Config shape:
  - `reasoningEndpoints[]`: ordered OpenAI-compatible Chat Completions endpoint pool for final synthesis.
  - Fields: `name`, `baseURL`, `apiKey`, `model`.
- MCP tool:
  - `smart_answer(query, depth?, platform?, domains?, max_fetches?, reasoning_endpoint?, reasoning_model?)`.
- CLI command:
  - `grok-search cli smart-answer <query> --depth <quick|standard|deep> --domain <domain> --max-fetches <n> --reasoning-endpoint <name> --reasoning-model <model> --json`.
- Runtime placement:
  - Generic reasoning client/pool: `internal/engine/`.
  - Composition/MCP registration: `internal/tools/`.
  - CLI parsing/wiring: `internal/cli/`.
  - Server registration: `internal/server/server.go`.

#### 3. Contracts

- Search/fetch ownership:
  - `web_search` and `research_run` remain the evidence layer.
  - Reasoning endpoints synthesize only after research has produced a compact research pack.
  - Do not place non-search synthesis providers in `grokEndpoints`, because any successful response would short-circuit source-first fallbacks.
- Secrets:
  - Reasoning API keys are read only from the active single config file.
  - Config/diagnostic output must show masked key status only.
  - Upstream error bodies must redact the configured secret before surfacing errors.
- Routing:
  - `reasoning_endpoint` selects one named endpoint.
  - If no endpoint is selected, the pool tries configured `reasoningEndpoints` in priority order.
  - `reasoning_model` may override the selected endpoint's configured model for a single call.
- Output:
  - Human/MCP output includes endpoint name, model, research depth, source count, final answer, and high-signal URLs.
  - JSON output preserves the final answer plus the research pack for reproducibility.

#### 4. Validation & Error Matrix

| Condition | Required behavior |
| --- | --- |
| Missing query | Return usage/tool error before network calls |
| No `reasoningEndpoints` configured | Return clear unavailable error after/before synthesis; do not silently fall back to `grokEndpoints` |
| Named `reasoning_endpoint` not found | Return clear endpoint-not-found error |
| Reasoning endpoint missing `baseURL` or `apiKey` | Config load fails with `reasoningEndpoints` context |
| Reasoning endpoint missing model | Default to `deepseek-v4-flash` |
| Upstream 429/5xx or transient network error | Retry via shared retry helper |
| Upstream non-retryable non-2xx | Return status plus redacted clipped response body |
| Empty choices/content | Treat as failure and try the next reasoning endpoint when available |
| `research_run` fails | Preserve the partial research pack and return a synthesis-blocking error |

#### 5. Good/Base/Bad Cases

- Good: `smart_answer` gathers sources with existing research routing, then sends the compact pack to `deepseek-v4-flash` for final synthesis.
- Base: `reasoningEndpoints` contains two public/paid-compatible endpoints for the same model; the pool falls through when the first fails.
- Bad: adding DeepSeek to `grokEndpoints` for `web_search`, using `*-search` reasoning models when local search already gathered sources, or printing full reasoning API keys in diagnostics.

#### 6. Tests Required

- Config:
  - `reasoningEndpoints` load, `/v1` normalization, default names/models, invalid endpoint errors.
- Engine:
  - Chat Completions path, method, auth header, JSON body, response parsing, redacted error body, named endpoint selection.
- Tools:
  - `smart_answer` passes research options through, includes research pack in the reasoning prompt, returns endpoint/model metadata.
- CLI:
  - Parses `--depth`, repeatable `--domain`, `--max-fetches`, `--reasoning-endpoint`, `--reasoning-model`, and `--json`.
- Docs:
  - README documents config fields, MCP tool, CLI command, and why reasoning endpoints are separate from search endpoints.

#### 7. Wrong vs Correct

Wrong:

```json
{
  "grokEndpoints": [
    {"name":"deepseek","baseURL":"https://api.deepseek.com/v1","apiKey":"sk-...","model":"deepseek-v4-flash"}
  ]
}
```

Correct:

```json
{
  "grokEndpoints": [
    {"name":"grok-fast","baseURL":"https://grok2api.example/v1","apiKey":"sk-...","model":"grok-4.20-fast","sendSearchFlag":false}
  ],
  "reasoningEndpoints": [
    {"name":"deepseek-flash","baseURL":"https://api.deepseek.com/v1","apiKey":"sk-...","model":"deepseek-v4-flash"}
  ]
}
```

---

### Scenario: Grok Responses API built-in tool selection

#### 1. Scope / Trigger

- Trigger: changing how Grok/xAI Responses API endpoints declare server-side tools such as Web Search or X Search.
- Scope: this covers config parsing, CLI setup, Grok engine request construction, diagnostics, and docs. It must preserve the source-first search route and must not change Chat Completions endpoint behavior.

#### 2. Signatures

- Config shape:
  - `grokEndpoints[].apiType`: `""`, `"chat"`, or `"responses"`.
  - `grokEndpoints[].sendSearchFlag`: enables provider-specific search flags/tools.
  - `grokEndpoints[].responseTools`: optional string array for Responses API built-in tools.
- Supported initial tool names:
  - `"web_search"`
  - `"x_search"`
- CLI setup:
  - `grok-search cli setup ... --api-type responses --send-search-flag --response-tools web_search,x_search`
- Runtime placement:
  - Tool constants, validation, and request body construction belong in `internal/engine/`.
  - Config normalization belongs in `internal/config/`.
  - Setup/config/probe display belongs in `internal/cli/`.
  - MCP diagnostic output belongs in `internal/tools/config_tool.go`.

#### 3. Contracts

- Backward compatibility:
  - If `apiType == "responses"` and `sendSearchFlag == true` and `responseTools` is empty, the engine sends only `{"type":"web_search"}`.
  - Existing Chat Completions behavior remains `search:true` when `sendSearchFlag` is true.
- Validation:
  - Trim tool names.
  - Reject empty tool names.
  - Reject unsupported tool names.
  - Deduplicate while preserving first-seen order.
  - Reject non-empty `responseTools` unless `apiType == "responses"` to avoid silent no-op config.
- Diagnostics:
  - `config list`, `doctor`/`probe`, and `get_config_info` should show effective response tools when a Responses API endpoint will send them.
  - Do not print API keys while displaying tool config.

#### 4. Validation & Error Matrix

| Condition | Required behavior |
| --- | --- |
| `responseTools` omitted on Responses API endpoint | Search-enabled requests send `web_search` only |
| `responseTools: ["web_search", "x_search"]` | Request body includes both tools in order |
| Duplicate tool names | Deduplicate and preserve first occurrence |
| Blank tool name | Config/setup validation fails before network calls |
| Unsupported tool name | Config/setup validation fails before network calls |
| `responseTools` with `apiType: "chat"` or empty API type | Config/setup validation fails before network calls |
| `sendSearchFlag: false` | Do not send Responses API tools, even if the endpoint supports them |

#### 5. Good/Base/Bad Cases

- Good: Native xAI Responses endpoint uses `apiType: "responses"`, `sendSearchFlag: true`, and `responseTools: ["web_search", "x_search"]`.
- Base: Existing Responses endpoint with only `sendSearchFlag: true` continues to send `web_search`.
- Bad: Adding `responseTools` to a Chat Completions proxy, silently ignoring an unsupported tool, or moving a synthesis-only model into `grokEndpoints`.

#### 6. Tests Required

- Engine:
  - Default Responses API request sends `web_search` and not `x_search`.
  - Configured Responses API request sends every selected tool in order.
  - Tool normalization trims, rejects invalid entries, and deduplicates.
- Config:
  - `responseTools` loads and normalizes.
  - Invalid tool names fail with `grokEndpoints` context.
  - Non-empty `responseTools` requires `apiType: "responses"`.
- CLI:
  - `setup --response-tools web_search,x_search` writes a loadable config when `--api-type responses` is present.
  - `setup --response-tools ...` rejects invalid tools and rejects Chat Completions endpoints.
  - `config list --json` masks secrets and reports effective response tools.
- Docs:
  - README/Quickstart/Smart Answer docs explain X search opt-in and keep reasoning endpoints separate.

#### 7. Wrong vs Correct

Wrong:

```json
{
  "grokEndpoints": [
    {
      "baseURL": "https://grok-proxy.example/v1",
      "apiKey": "sk-...",
      "apiType": "chat",
      "responseTools": ["x_search"]
    }
  ]
}
```

Correct:

```json
{
  "grokEndpoints": [
    {
      "baseURL": "https://api.x.ai/v1",
      "apiKey": "sk-...",
      "apiType": "responses",
      "sendSearchFlag": true,
      "responseTools": ["web_search", "x_search"]
    }
  ]
}
```

---

### Scenario: CLI configuration and setup surfaces

#### 1. Scope / Trigger

- Trigger: adding or changing CLI commands that inspect, diagnose, or write local runtime configuration.
- Scope: config UX commands must preserve the single-file contract; do not add environment-variable config chains, hidden home-directory config, or legacy fallback files.

#### 2. Signatures

- Config inspection:
  - `grok-search cli config path [--json]`
  - `grok-search cli config files [--json]`
  - `grok-search cli config list [--json]`
- Global config selection:
  - `grok-search --config <path>` for MCP/server mode.
  - `grok-search cli --config <path> <command>` for CLI mode.
- Setup:
  - `grok-search cli setup [--non-interactive] --api-url <url> --api-key <key> [--model <model>] [--api-type chat|responses] [--send-search-flag] [--response-tools <csv>] [--tavily-key <key>] [--exa-key <key>] [--jina-key <key>] [--tinyfish-keys <csv>] [--tinyfish-key-names <csv>] [--force] [--json]`
- Diagnostics:
  - `grok-search cli doctor [--list-timeout <dur>] [--preview <n>] [--json]`
  - `grok-search cli probe ...` remains a backward-compatible alias.

#### 3. Contracts

- Config path:
  - Default config is `config.DefaultConfigPath()` (`./grok-search.json`).
  - `--config` selects exactly one explicit JSON file.
  - No environment variables, `~/.config/grok-search/*`, or legacy `endpoints.json` files are loaded.
- Config list:
  - Must call the same config loader used by MCP/CLI runtime.
  - Must mask all secrets with `keyStatus`; never print full API keys.
  - Must not probe network or call provider APIs.
- Config files:
  - Must show only the active config file by name/path/stat.
  - Must not read or print secret values.
  - Must explain that hidden home config and legacy endpoint files are ignored.
- Setup:
  - Must write the active `grok-search.json` shape, including `grokEndpoints`, optional provider blocks, and `logLevel`.
  - Must create the config file's parent directory automatically with user-only permissions when possible.
  - Must write the file with `0600` permissions when possible.
  - Must refuse to overwrite an existing config unless `--force` is passed.
  - Interactive prompts must go to stderr so `--json` stdout remains parseable.

#### 4. Validation & Error Matrix

| Condition | Required behavior |
| --- | --- |
| Explicit `--config ""` or `--config=` | Return usage error; do not silently fall back to default config |
| `config path` with no existing config | Return the active path and missing status; do not error |
| `config files` with historical hidden files elsewhere | Do not scan or load them; show only the active config file |
| `config list` with missing config file | Return clear error plus next steps |
| `config list` with provider-only config | Load successfully; Grok endpoints may be empty |
| Missing `--api-url` in `setup --non-interactive` | Return setup error before writing |
| Missing `--api-key` in `setup --non-interactive` | Return setup error before writing |
| Invalid `--api-type` | Return setup error before writing |
| Existing active config without `--force` | Refuse overwrite and keep existing file unchanged |
| Any JSON output path | Emit stable JSON to stdout and human diagnostics/prompts to stderr |
| Any output includes keys | Mask or omit secrets; never print raw keys |

#### 5. Good/Base/Bad Cases

- Good: `grok-search cli setup --non-interactive --api-url https://example.com/v1 --api-key sk-... --json` writes `./grok-search.json` and returns masked next steps.
- Base: `grok-search cli config list --json` shows endpoint/provider status with masked key values and no live network calls.
- Bad: adding a second CLI-only config file, reading `~/.config/grok-search`, requiring env vars for runtime config, asking users to hand-edit JSON as the only setup path, or printing raw keys in errors/tests/docs.

#### 6. Tests Required

- CLI dispatch:
  - `doctor --help` returns success and uses the doctor command name.
  - `setup --help` returns success.
- Config path/list:
  - `config path --json` reports the active `--config` path, absolute path, and existence status.
  - `config files --json` reports only the active single file and loading notes without leaking secrets.
  - `config list --json` masks all provider and endpoint secrets.
  - Missing config returns next steps.
- Setup:
  - Non-interactive setup writes a loadable config file.
  - Setup JSON output masks secrets.
  - Existing config is not overwritten unless `--force` is passed.

#### 7. Wrong vs Correct

Wrong:

```go
fmt.Printf("wrote apiKey=%s\n", opts.APIKey)
_ = os.WriteFile("grok-search-cli.json", data, 0o644)
_ = os.ReadFile(filepath.Join(os.Getenv("HOME"), ".config/grok-search/config.json"))
```

Correct:

```go
fmt.Printf("wrote key=%s\n", keyStatus(opts.APIKey))
_ = os.WriteFile(currentConfigPath(), data, 0o600)
```

---

### Scenario: Public-release repository hygiene

#### 1. Scope / Trigger

- Trigger: preparing this repository for public/open-source release, especially when local agent workflow directories, sample configs, CI, or user-facing setup docs are touched.
- Scope: public release changes must keep the tracked Git tree focused on product code, safe examples, and general-purpose documentation. Developer-local AI workflow state may remain on disk, but must not be tracked in the public tree.

#### 2. Signatures

- Local-only workflow directories:
  - `.trellis/`
  - `.agents/`
  - `.codex/`
  - `.claude/`
- Public reusable guidance belongs in:
  - `README.md`
  - `AGENTS.md`
  - `docs/*.md`
  - `configs/*.example.json`
- Baseline release files:
  - `LICENSE`
  - `SECURITY.md`
  - `CONTRIBUTING.md`
  - `CHANGELOG.md` when a release history is useful
- CI baseline:
  - `.github/workflows/ci.yml` must run `go test ./...`, `go vet ./...`, and `go build ./...`.

#### 3. Contracts

- Git tracking:
  - Remove local AI workflow directories from the Git index only; do not delete local copies during an active Trellis task.
  - Add ignore rules for the local workflow directories so they are not accidentally re-added.
  - If a local workflow file contains reusable project guidance, convert it into public docs instead of publishing the workflow file directly.
- Docs/examples:
  - Public docs must not contain personal absolute paths such as `/Users/<name>/...`.
  - Public docs must not contain private endpoint hostnames, real API keys, developer names, local journal/task history, or client-specific hooks.
  - Example configs must use placeholder secrets and generic endpoint URLs.
- Product behavior:
  - User-visible missing-config errors must point to the relevant config field or example docs.
  - Config examples must remain loadable JSON and align with the current config loader.

#### 4. Validation & Error Matrix

| Condition | Required behavior |
| --- | --- |
| `.trellis/`, `.agents/`, `.codex/`, or `.claude/` tracked before release | Remove from Git tracking with an index-only operation and ignore going forward |
| Local workflow directories absent from disk during an active Trellis task | Stop and restore/ask before proceeding; do not break the local workflow |
| Public docs contain `/Users/`, real names, private hosts, or real-looking keys | Generalize or remove before release |
| Example config has invalid JSON | Fix before merge |
| Example config requires real credentials to parse | Replace with safe placeholders |
| CI does not cover test, vet, and build | Add or fix CI workflow |

#### 5. Good/Base/Bad Cases

- Good: `.gitignore` marks `.trellis/`, `.agents/`, `.codex/`, and `.claude/` as local AI workflow state, while `docs/QUICKSTART.md` contains generic MCP setup instructions.
- Base: `configs/grok-search.example.json` and `configs/grok-search.reasoning.example.json` parse with `python3 -m json.tool` and use placeholders such as `sk-your-key`.
- Bad: committing `.trellis/tasks/archive/...`, `.codex/config.toml`, `.claude/settings.json`, or docs that tell users to run binaries from `/Users/johnsmith/...`.

#### 6. Tests Required

- Repository hygiene:
  - `git ls-files .trellis .agents .codex .claude` returns no tracked files for a public release branch.
  - Local workflow directories still exist on disk if the active task needs them.
- Docs/config:
  - Search public docs/examples for personal paths, developer names, private endpoint names, and raw secrets.
  - Validate every `configs/*.json` example with a JSON parser.
- Product quality:
  - Run `go test ./...`, `go vet ./...`, and `go build ./...`.
  - If missing-config UX changes, add focused tests for the new error message and no-unnecessary-network behavior.

#### 7. Wrong vs Correct

Wrong:

```bash
git rm -r .trellis .agents .codex .claude
```

Correct:

```bash
git rm -r --cached .trellis .agents .codex .claude
```

---

## Code Review Checklist

<!-- What reviewers should check -->

(To be filled by the team)
