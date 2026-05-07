# Prompt Pack Source Summary

Source document:

`/Users/johnsmith/Project/Study/fund-analysis/.trellis/tasks/05-06-ai-fund-decision-assistant/research/grok-search-go-prompt-pack-2026-05-06.md`

## Extracted roadmap

1. Implement `web_crawl`.
2. Implement composite `research_run` / `deep_research`.
3. Enhance Exa advanced search/contents options without changing default routing.
4. Run a quality cleanup across the new research-layer surfaces.

## Chosen task slice

Implement Prompt 1 (`web_crawl`) as the MVP:

* Reuse Tavily client, no new crawler framework.
* Add engine method, MCP tool, CLI command, tests, and README docs.
* Keep defaults compact and LLM-readable.

## API reference checked

* Tavily Crawl official docs: `https://docs.tavily.com/documentation/api-reference/endpoint/crawl`
* Confirmed endpoint: `POST /crawl`
* Confirmed core request fields: `url`, `instructions`, `max_depth`, `max_breadth`, `limit`, `extract_depth`, `format`, `include_images`
* Confirmed core response fields: `base_url`, `results[].url`, `results[].raw_content`, `response_time`

## Deferred work

* `research_run` orchestration.
* Exa advanced modes.
* Broad quality cleanup of surfaces that do not exist yet.
