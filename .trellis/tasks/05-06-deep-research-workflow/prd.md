# Add composable deep research workflow

## Goal

Add a composable deep research tool that executes existing planning, search, source retrieval, fetch, map, and crawl capabilities into a compact research pack for downstream LLM consumption, without adding LangGraph, CrewAI, AutoGen, or another agent framework.

## Requirements

* Expose an MCP tool named `research_run` unless a stronger naming reason is documented.
* Expose a matching CLI command named `research`.
* Accept inputs:
  * `query` required.
  * `depth`: `quick` / `standard` / `deep`.
  * `platform` optional.
  * `domains` optional.
  * `max_fetches` optional.
* First version is in-memory only; do not add database persistence.
* Workflow must call existing planning logic to generate a staged plan, extract planned queries, then execute multiple `web_search` rounds.
* Workflow must read each search round's sources, deduplicate URLs, rank/select top URLs, and execute `web_fetch` on top-N URLs.
* Workflow should use `web_map` or `web_crawl` when useful for target sites, while reusing existing Tavily map/crawl implementation.
* Output a unified research pack containing at least:
  * query
  * effective depth
  * executed searches
  * source summary
  * fetched pages summary
  * high-signal sources
  * confirmed facts
  * likely inferences
  * open questions
* Ranking/filtering is heuristic only in v1:
  * Official sites first.
  * Repeated URLs/domains first.
  * Recent pages first when dates are available.
  * Title/query relevance first.
  * Failed fetches or obvious boilerplate pages downranked.
* Do not change default provider routing priority for search/fetch/map/crawl.
* Prefer extracting shared helpers over copy-paste.
* Expose the workflow through a new MCP tool and matching CLI command.
* CLI output must support both human-readable text and stable `--json` output.
* Update README and AGENTS documentation for the new surface.

## Acceptance Criteria

* [x] Query plan parsing/extraction tests exist.
* [x] URL deduplication tests exist.
* [x] Source ranking/filtering tests exist.
* [x] Research pack output format tests exist.
* [x] CLI parameter parsing tests exist.
* [x] MCP registers `research_run`.
* [x] CLI exposes `research` with `--depth`, `--platform`, `--domain`, `--max-fetches`, and `--json`.
* [x] Existing `web_crawl` tests continue to pass without duplicating crawl implementation.
* [x] `go test ./...` passes.

## Definition of Done

* Tests added/updated.
* Docs updated when user-facing behavior changes.
* Lint/typecheck/test command passes.
* No live external API calls in unit tests.

## Technical Approach

Implement an orchestration layer that has pure helper functions for plan query extraction, URL normalization/deduplication, source ranking, content clipping, and research pack formatting. Keep provider calls behind small interfaces so tests can use fakes and avoid live API calls. MCP returns compact text for LLM consumption; CLI `--json` preserves the typed research pack.

## Decision (ADR-lite)

**Context**: The repo already has individual search/fetch/map/crawl tools and a deterministic `search_planning` helper. The new requirement is a higher-level composition workflow, not another crawler or external orchestration framework.

**Decision**: Add an in-memory `research_run` executor that composes existing provider/client functionality and shared pure helper functions, then wire it to MCP and CLI.

**Consequences**: The first version performs bounded execution and returns compact packs, but avoids durable state or external orchestration frameworks. Tests use fakes and focus heavily on pure helper behavior.

## Out of Scope

* No LangGraph / CrewAI / AutoGen integration.
* No new crawl provider or Tavily Crawl reimplementation.
* No changes to credential/config loading unless needed for docs.
* No complex model-based scoring in v1.

## Technical Notes

* Files inspected: README, AGENTS, existing tool registrations, existing CLI commands, Exa/Tavily clients, planning helper, and related tests.
* Existing `web_crawl` is already connected in MCP and CLI and has unit coverage.
* Partial planning-only `deep_research` code may exist from an earlier iteration; align final naming/behavior to `research_run` / `cli research` unless documenting a reason to keep `deep_research`.
* Relevant specs: backend quality guidelines and shared thinking guides.
