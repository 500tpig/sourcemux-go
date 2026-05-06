# TinyFish docs summary

Sources fetched on 2026-05-06 via project grok-search MCP:

* https://docs.tinyfish.ai/search-api
* https://docs.tinyfish.ai/search-api/reference
* https://docs.tinyfish.ai/fetch-api
* https://docs.tinyfish.ai/fetch-api/reference
* https://docs.tinyfish.ai/agent-api
* https://docs.tinyfish.ai/browser-api
* https://docs.tinyfish.ai/common-patterns
* https://docs.tinyfish.ai/authentication
* https://docs.tinyfish.ai/error-codes
* https://docs.tinyfish.ai/mcp-integration
* https://docs.tinyfish.ai/anti-bot-guide
* https://www.tinyfish.ai/blog/search-and-fetch-are-now-free-for-every-agent-everywhere
* https://docs.tinyfish.ai/api-reference/automation/run-browser-automation-with-sse-streaming
* https://docs.tinyfish.ai/api-reference/automation/run-browser-automation-synchronously
* https://docs.tinyfish.ai/api-reference/automation/start-automation-asynchronously
* https://docs.tinyfish.ai/api-reference/automation/start-multiple-automations-asynchronously
* https://docs.tinyfish.ai/api-reference/runs/list-and-search-runs
* https://docs.tinyfish.ai/key-concepts/runs
* https://docs.tinyfish.ai/examples/bulk-requests-async

Key facts:

* Search API: `GET https://api.search.tinyfish.ai`, returns structured ranked results with title/snippet/url. Requires `X-API-Key`. Search does not use credits. Reference states rate limits are per API key: Free 5 rpm, Pay As You Go 10 rpm, Starter 20 rpm, Pro 50 rpm.
* Fetch API: `POST https://api.fetch.tinyfish.ai`, renders pages in a real browser and extracts content as markdown/html/json. Supports up to 10 URLs per request. Requires `X-API-Key`. Fetch does not use credits. Reference states Free 25 URLs/min, Pay As You Go 50, Starter 100, Pro 250. Fetch rejects private IP, localhost, and metadata endpoints.
* Fetch error model: HTTP errors apply to whole request; per-URL failures return in `errors[]` with codes like `timeout`, `bot_blocked`, `empty_content`, `invalid_url`, `proxy_error`, `fetch_error`.
* Agent API: `POST https://agent.tinyfish.ai/v1/automation/run`, `/run-async`, `/run-sse`. Natural-language goal drives a real website workflow. Best when TinyFish should decide browser actions.
* Browser API: `POST https://api.browser.tinyfish.ai`, creates an isolated remote browser session and returns `cdp_url` for Playwright/CDP. Startup can take 10-30s; recommended client timeout at least 60s. Sessions auto-terminate after 1 hour inactivity and have no explicit delete endpoint.
* MCP integration exposes tools including `run_web_automation`, async/batch run tools, `search`, `fetch_content`, usage listing, and browser session creation. MCP auth uses OAuth 2.1, while REST API uses API keys.
* Common patterns include Search + Fetch and Search + Agent workflows, retry with stealth mode/proxy, result validation, rate-limit handling, and authenticated automation via vault credentials.
* Error docs state rate-limit code `RATE_LIMIT_EXCEEDED` maps to HTTP 429; the page says Retry-After is not currently returned, so callers should use exponential backoff.
* Blog published 2026-05-04 says Search and Fetch are free across REST, MCP, SDKs, CLI, and integrations, with free-tier limits of 5 Search queries/min and 25 Fetch URLs/min.

Assessment notes:

* TinyFish Search overlaps with existing Exa/Tavily-style source-first fallback more than Grok's synthesized answer path.
* TinyFish Fetch is a stronger candidate than Search for this repo because current primary fetch is Jina Reader and the code comments treat JS-heavy/Jina-blocked pages as fallback territory.
* Agent/Browser are not natural fits for existing `web_search`/`web_fetch` semantics; they are separate workflow/browser-control products and would require new MCP tools rather than hidden fallback.
* Multi-account pooling for TinyFish would add credential lifecycle, per-key rate accounting, fairness, error quarantine, and ToS/abuse risk. Prefer one official key with plan upgrade or a documented endpoint slot, not formal account-pool support.

Additional Agent/Run facts:

* SSE endpoint: `POST https://agent.tinyfish.ai/v1/automation/run-sse`. Required body fields are `url` and `goal`; optional fields include `browser_profile` (`lite`/`stealth`), `proxy_config`, `agent_config`, `capture_config`, `webhook_url`, `use_vault`, `use_profile`, `credential_item_ids`, and `output_schema`. Stream sends `STARTED`, optional `STREAMING_URL`, `PROGRESS`, `COMPLETE`, and heartbeat events.
* Sync endpoint: `POST /v1/automation/run`; returns final run object with `status`, timestamps, `num_of_steps`, `result`, and `error`. Runs created this way cannot be cancelled.
* Async endpoint: `POST /v1/automation/run-async`; returns `run_id` immediately. Poll runs separately.
* Batch endpoint: `POST /v1/automation/run-batch`; supports 1-100 runs; all-or-nothing creation; no idempotency key, so retrying a failed request may duplicate runs.
* Runs listing: `GET /v1/runs` supports filters by status, goal, created_after/before, cursor, and limit. Run statuses are `PENDING`, `RUNNING`, `COMPLETED`, `FAILED`, `CANCELLED`.
* Completed run does not always mean goal-level success. Docs recommend inspecting `result` for failure markers such as `status: failure` or `error`.
* For evaluation harnesses, prefer async/sync endpoints for reproducible measurement; use SSE mainly for diagnostics and streaming URL collection, not as the first integration surface.
