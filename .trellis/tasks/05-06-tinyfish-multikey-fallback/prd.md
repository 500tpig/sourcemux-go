# TinyFish multi-key production fallback

## Goal

Integrate TinyFish Search and Fetch into production MCP routing with support for multiple configured API keys. The main value is improving `web_fetch` success on JS-heavy pages while also allowing TinyFish Search as a source-first search fallback.

## What I already know

* The user explicitly wants direct multi-key integration and accepts the policy/usage consequences.
* Existing TinyFish code is currently benchmark-only: `internal/engine/tinyfish.go`, `internal/cli/tinyfish_bench.go`, and README say it does not enter production MCP routing.
* Current `web_fetch` order is Jina Reader -> Exa Contents -> Tavily Extract.
* Current `web_search` order is Grok pool -> Exa Search -> Tavily Search.
* TinyFish docs summary from the previous evaluation task says Search/Fetch are currently free, rate-limited per key, and Fetch supports browser-rendered extraction.

## Requirements

* Add TinyFish runtime configuration under `~/.config/grok-search/config.json` and env vars.
* Support multiple TinyFish keys with names and round-robin/fallback behavior.
* Do not print full API keys in user-visible output or errors.
* Add TinyFish Fetch to production `web_fetch` routing after Jina and before Exa/Tavily.
* Add TinyFish Search to production `web_search` routing after Grok and before Exa/Tavily.
* Treat key-specific 429/401/403 errors as fallback signals so another configured key can be tried.
* Preserve existing behavior when TinyFish is disabled or unconfigured.
* Keep Agent/Browser out of production MCP routing in this task.

## Acceptance Criteria

* [x] `config.Load()` reads TinyFish config from `config.json` and env vars.
* [x] Server initializes a TinyFish pool only when enabled and keys exist.
* [x] `web_fetch` can return `Source: TinyFish Fetch (...)` when Jina fails and TinyFish succeeds.
* [x] `web_search` can return an engine envelope for TinyFish Search and cache source URLs for `get_sources`.
* [x] Tests cover config loading, pool key fallback, and search/fetch routing.
* [x] `go test ./...` passes.
* [x] README documents the new config shape and routing order.

## Out of Scope

* TinyFish Agent API integration.
* TinyFish Browser/CDP integration.
* Live TinyFish API calls in tests.
* Automatic creation or management of TinyFish accounts.

## Technical Notes

* Relevant specs: `.trellis/spec/backend/index.md`, `.trellis/spec/backend/quality-guidelines.md`, `.trellis/spec/guides/index.md`.
* Prior research: `.trellis/tasks/05-06-tinyfish-integration-eval/research/tinyfish-docs-summary.md`.
* Expected config shape:

```json
{
  "tinyfish": {
    "enabled": true,
    "keys": [
      {"name": "acct-a", "apiKey": "tf-key-1"},
      {"name": "acct-b", "apiKey": "tf-key-2"}
    ]
  }
}
```
