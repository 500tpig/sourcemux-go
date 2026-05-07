# AI Fund Decision Prompt Pack Integration

## Goal

Turn the external prompt pack into a practical first version of the project research-layer capabilities. The implementation should improve `grok-search-go` as a research gateway for downstream agents without over-building the full roadmap in one pass.

## What I already know

* The source prompt pack is at `/Users/johnsmith/Project/Study/fund-analysis/.trellis/tasks/05-06-ai-fund-decision-assistant/research/grok-search-go-prompt-pack-2026-05-06.md`.
* The recommended staged order is: `web_crawl` first, then `research_run`, then Exa advanced modes, then quality cleanup.
* This repository already has `web_search`, `web_fetch`, `web_map`, `get_sources`, `search_planning`, and CLI mirrors for search/fetch/map/plan/probe.
* Tavily Search/Extract/Map already lives in `internal/engine/tavily.go`; Crawl is not wrapped yet.
* The current README routing already includes Grok pool -> TinyFish -> Exa -> Tavily for search and Jina -> TinyFish -> Exa -> Tavily for fetch.

## Assumptions

* The first useful slice is `web_crawl`, because it is a missing primitive and a prerequisite for any later deep research orchestration.
* `research_run` and advanced Exa modes should remain out of scope for this task unless the crawl layer finishes cleanly with enough time.
* Tavily must remain an optional configured provider; when no Tavily key exists, the new MCP/CLI surface should return a clear unavailable error without changing existing routes.

## Requirements

* Add a Tavily Crawl engine wrapper in `internal/engine/tavily.go`.
  * Request fields: `url`, `instructions`, `max_depth`, `max_breadth`, `limit`, `extract_depth`, `format`, `include_images`.
  * Response fields preserved: `base_url`, `results[].url`, `results[].raw_content`, `response_time`.
  * Reuse existing retry behavior and auth/header conventions.
* Add MCP tool `web_crawl`.
  * Tool description must clearly distinguish crawl+extract from `web_map`.
  * Expose a compact parameter set: `url`, `instructions`, `max_depth`, `max_breadth`, `limit`, `extract_depth`, `format`, `include_images`.
  * Output must be concise and LLM-readable.
* Add CLI subcommand `crawl`.
  * Mirror existing CLI style.
  * Support `--json`, `--max-depth`, `--max-breadth`, `--limit`, `--instructions`, plus practical optional `--extract-depth`, `--format`, `--include-images`, `--timeout`.
* Update README.
  * Add `web_crawl` to architecture/features/CLI command table/examples.
  * Explain `web_map` discovers URLs only; `web_crawl` also extracts content.
* Add tests.
  * Tavily Crawl request construction, retry path, and response parsing.
  * CLI JSON shape and/or dispatch coverage for the new `crawl` surface.

## Acceptance Criteria

* [x] `go test ./...` passes.
* [x] `web_crawl` is registered by the MCP server.
* [x] `grok-search cli crawl <url> --json` is available and produces stable JSON shape.
* [x] Unit tests do not call live Tavily APIs.
* [x] Existing search/fetch/map behavior and fallback ordering remain unchanged.

## Definition of Done

* Tests added/updated.
* Lint/typecheck equivalent (`go test ./...`) passes.
* README updated for changed public surface.
* Rollback is simple: remove the new crawl files/registration and Tavily Crawl methods.

## Out of Scope

* Full `research_run` / `deep_research` orchestration.
* Advanced Exa mode expansion.
* New external crawler frameworks or database persistence.
* Changing default search/fetch fallback ordering.

## Technical Approach

Implement the smallest complete vertical slice:

1. Extend the existing Tavily client with typed Crawl request/result structs and a `Crawl` method.
2. Add `internal/tools/crawl.go` for MCP registration and LLM-readable formatting.
3. Add `internal/cli/crawl.go` and dispatch/usage updates.
4. Register the tool in `internal/server/server.go`.
5. Add targeted tests and README docs.

## Decision (ADR-lite)

**Context**: The prompt pack describes multiple research-layer improvements, but `web_crawl` is the prerequisite primitive and has clear bounded scope.

**Decision**: Implement `web_crawl` first as a complete MCP + CLI + engine slice. Defer `research_run` and Exa advanced modes.

**Consequences**: This ships immediately useful site-level extraction while keeping the codebase stable. Later research orchestration can compose `web_search`, `get_sources`, `web_fetch`, `web_map`, and the new `web_crawl`.

## Research References

* [`research/prompt-pack-source.md`](research/prompt-pack-source.md) - local summary of the external prompt pack and chosen slice.

## Technical Notes

* Relevant code inspected:
  * `README.md`
  * `internal/engine/tavily.go`
  * `internal/engine/tavily_test.go`
  * `internal/tools/map.go`
  * `internal/tools/fetch.go`
  * `internal/tools/search.go`
  * `internal/cli/cli.go`
  * `internal/cli/map.go`
  * `internal/cli/fetch.go`
  * `internal/cli/cli_test.go`
  * `internal/server/server.go`
* Relevant spec:
  * `.trellis/spec/backend/index.md`
  * `.trellis/spec/backend/quality-guidelines.md`
* Tavily Crawl API reference checked on 2026-05-06:
  * `https://docs.tavily.com/documentation/api-reference/endpoint/crawl`
