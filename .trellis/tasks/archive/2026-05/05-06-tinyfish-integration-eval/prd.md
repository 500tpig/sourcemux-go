# TinyFish integration evaluation and benchmark harness

## Goal

Evaluate whether TinyFish is worth integrating into the current grok-search-go project, then implement a local benchmark harness to measure TinyFish Search, Fetch, and Agent behavior across one or more locally configured API keys before changing production MCP routing.

## What I already know

* User wants analysis first, no code changes.
* Output must start with conclusion, then evidence with file paths/functions/call chains, then integration plan, then risks and recommendations.
* Must explicitly assess Search, Fetch, Agent/Browser fit and multi-account pool value.
* User confirmed implementation can start.
* User can configure about 5 TinyFish accounts and wants to evaluate availability, query quality, interface quality, and capability.
* The implementation should benchmark multiple keys without formally adding multi-account pooling to normal `web_search` / `web_fetch` routing.

## Requirements

* Use `rg` first to map search/fetch/provider/fallback/source/MCP/API routing code entrypoints.
* Describe actual project architecture: search entry, fetch entry, provider/fallback organization, config/model/rate-limit/error handling locations.
* Compare current grok-search implementation and TinyFish based on code plus TinyFish docs, not product copy alone.
* Identify existing extension points similar to TinyFish.
* If code changes are worth doing, provide a plan only and wait for user confirmation.
* Implement a local TinyFish benchmark harness, not production TinyFish routing.
* Support locally configured multiple keys via environment variables or a local-only config path; never print full keys.
* Benchmark TinyFish Search, Fetch, and Agent surfaces:
  * Search: structured result availability, latency, status/error, result count, source URLs.
  * Fetch: extracted content availability, latency, status/error, text length, per-URL errors.
  * Agent: sync or async run behavior, final status, result/error, steps if available, latency.
* Produce machine-readable JSON output so results can be compared later.
* Include a sample cases file without secrets.
* Keep benchmark code isolated from production MCP routing.
* Add tests for request construction, response parsing, and key masking / config loading where practical.

## Out of Scope

* Do not add TinyFish to production `web_search` or `web_fetch` fallback yet.
* Do not implement formal multi-account pooling for production routes.
* Do not store API keys in repo files.
* Do not call live TinyFish APIs in tests.

## Technical Notes

* Repository: `/Users/johnsmith/Project/Study/grok-search-go`
* TinyFish docs: `https://docs.tinyfish.ai/`
* Research summary: `.trellis/tasks/05-06-tinyfish-integration-eval/research/tinyfish-docs-summary.md`
