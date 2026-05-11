# brainstorm: DeepSeek smart answer integration

## Goal

Add a low-cost "smart answer" path that keeps the existing grok-search search/fetch pipeline for evidence gathering, then uses DeepSeek V4 Flash/Pro as the reasoning and synthesis layer for higher-quality final answers without buying SuperGrok/Heavy.

## What I already know

* The user has many free/basic grok2api accounts and wants better answer quality without expensive Grok subscriptions.
* Existing `web_search` routes through Grok pool, then TinyFish, Exa, and Tavily fallback.
* Existing `research_run` already plans queries, searches, collects sources, fetches pages, and returns a compact research pack.
* DeepSeek V4 Flash/Pro are OpenAI-compatible chat completion models and should be used for reasoning, not as a replacement for source search.

## Assumptions (temporary)

* MVP should not disrupt existing `web_search`, `web_fetch`, or `research_run` behavior.
* DeepSeek endpoints should live in a separate config block, not in `grokEndpoints`.
* Default smart model is DeepSeek V4 Flash; V4 Pro is opt-in for hard/high-value questions.
* Secrets should remain masked in diagnostics and never printed in full.

## Open Questions

* None blocking for MVP; implementation will choose conservative defaults.

## Requirements

* Add configuration support for reasoning/synthesis endpoints, starting with DeepSeek-compatible OpenAI chat completions.
* Add an MCP tool `smart_answer` that:
  * accepts a user query,
  * runs the existing research workflow to gather evidence,
  * sends the compact research pack to a configured reasoning endpoint,
  * returns a final answer with evidence-aware caveats.
* Keep `web_search` provider fallback order unchanged.
* Allow model selection by endpoint name or configured default.
* Add a CLI equivalent for reproducible local testing.

## Acceptance Criteria

* [x] Config can load one or more `reasoningEndpoints` without affecting existing `grokEndpoints`.
* [x] `smart_answer` works when a reasoning endpoint is configured.
* [x] `smart_answer` returns a helpful error when no reasoning endpoint is configured.
* [x] Existing search/research tests continue to pass.
* [x] New tests cover config parsing and smart answer orchestration with fake providers.
* [x] README documents minimal DeepSeek configuration and usage.

## Definition of Done

* Tests added/updated.
* `go test ./...` passes.
* Docs/notes updated for new behavior.
* Existing routes remain backwards-compatible.

## Out of Scope

* Replacing `web_search` with DeepSeek.
* Adding account pooling, retry policy, or billing controls beyond existing HTTP timeout/retry patterns.
* Supporting non-chat-completions provider APIs in the MVP.
* Building a UI.

## Technical Approach

Use the existing `ResearchExecutor` as the evidence layer. Add a small OpenAI-compatible `ReasoningClient` and `ReasoningPool` for final synthesis. Register a new MCP tool and CLI command that compose:

```text
research_run/search+fetch -> compact evidence prompt -> DeepSeek reasoning endpoint -> final answer
```

## Decision (ADR-lite)

**Context**: Grok free Fast accounts are useful for search and volume but are weak as a final reasoning model. DeepSeek V4 Flash/Pro provide low-cost reasoning via OpenAI-compatible APIs.

**Decision**: Add DeepSeek as a separate reasoning layer behind a new `smart_answer` surface instead of inserting it into `grokEndpoints`.

**Consequences**: Existing behavior stays stable; users can opt into smarter synthesis. The system has a slightly larger config surface and must clearly distinguish search endpoints from reasoning endpoints.

## Technical Notes

* Likely files:
  * `internal/config/config.go`
  * `internal/engine/`
  * `internal/tools/research.go`
  * `internal/tools/`
  * `internal/server/server.go`
  * `internal/cli/`
  * `README.md`
* Existing search chain: `GrokPool -> TinyFish -> Exa -> Tavily`.
* Existing `ResearchExecutor` can be reused instead of duplicating source collection logic.
