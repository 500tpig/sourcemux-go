---
name: grok-search-mcp
description: Use when a task needs current web search, webpage fetching, source URL retrieval, or site mapping through this project's grok-search MCP server.
---

# Grok Search MCP

Use the `grok-search` MCP server as the primary web-research tool when current or source-backed information is needed.

## Tools

- `web_search`: search via configured Grok endpoint pool, with Exa then Tavily fallback when configured.
- `get_sources`: retrieve source URLs from a prior `web_search` result using `session_id`.
- `web_fetch`: fetch a page as Markdown via Jina Reader, with Exa Contents then Tavily Extract fallback when configured.
- `web_map`: map URLs on a site via Tavily; requires `TAVILY_API_KEY`.
- `get_config_info`: inspect configured endpoints and `/models` probe status.
- `search_planning`: create a staged search plan for complex research before running searches/fetches.

## Workflow

1. For current facts, call `web_search` first.
2. If the answer needs citation quality, call `get_sources` with the returned `session_id`, then `web_fetch` the most relevant URLs.
3. For complex or ambiguous research tasks, call `search_planning` before multi-round search.
4. Prefer concise summaries with source links over long copied text.
5. For site exploration, use `web_map` only when Tavily is configured.
6. If a tool reports missing configuration, ask the user to add endpoint keys rather than guessing.

## Configuration expectations

The server needs at least one Grok-compatible endpoint:

- Preferred: `~/.config/grok-search/endpoints.json`
- Alternative: `GROK_ENDPOINTS_JSON`
- Legacy: `GROK_API_URL` + `GROK_API_KEY`

Optional keys:

- `TAVILY_API_KEY` for search/fetch fallback and `web_map`
- `EXA_API_KEY` for source-first search/fetch fallback
- `JINA_API_KEY` for higher Jina Reader rate limits

Never print full API keys. Use `get_config_info` for masked diagnostics.
