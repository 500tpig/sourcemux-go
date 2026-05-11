---
name: grok-search-mcp
description: Use when a task needs current web search, webpage fetching, source URL retrieval, or site mapping through this project's grok-search MCP server.
---

# Grok Search MCP

Use the `grok-search` MCP server when the host client already has this MCP server connected and the task benefits from in-client tool calls, source sessions, or MCP-native workflows. For reproducible one-shot runs, long/high-frequency local agent loops, CI, or when MCP feels slow/unavailable, prefer the matching `grok-search cli` commands instead.

## Tools

- `web_search`: search via configured Grok endpoint pool, with TinyFish, Exa, then Tavily fallback when configured.
- `get_sources`: retrieve source URLs from a prior `web_search` result using `session_id`.
- `web_fetch`: fetch a page as Markdown via Jina Reader, with TinyFish, Exa Contents, then Tavily Extract fallback when configured.
- `web_map`: map URLs on a site via Tavily; requires `tavily.apiKey` in the active config file.
- `get_config_info`: inspect configured endpoints and `/models` probe status.
- `search_planning`: create a staged search plan for complex research before running searches/fetches.
- `research_run`: execute bounded multi-step research and return a compact research pack.

## Workflow

1. For current facts inside an MCP-enabled client, call `web_search` first.
2. If the answer needs citation quality, call `get_sources` with the returned `session_id`, then `web_fetch` the most relevant URLs.
3. For complex or ambiguous research tasks, call `search_planning` before multi-round search.
4. Prefer concise summaries with source links over long copied text.
5. For site exploration, use `web_map` only when Tavily is configured.
6. If a tool reports missing configuration, ask the user to run `grok-search cli config path` / `grok-search cli config files --json` / `grok-search cli config list --json`, or create the single active config file with `grok-search cli setup`.

## Configuration expectations

The server and CLI share one explicit JSON config file:

- default: `./grok-search.json`
- explicit: start the server with `grok-search --config /path/to/grok-search.json`

There is no runtime environment-variable config chain, no `~/.config/grok-search` loading, and no legacy `endpoints.json` fallback. If historical files exist, copy the needed endpoint/provider blocks into the single active file.

For setup without hand-editing JSON, use `grok-search cli setup`; it writes the same single file used by MCP.

Optional provider blocks:

- `tavily.apiKey` for search/fetch fallback and `web_map`
- `exa.apiKey` for source-first search/fetch fallback
- `jina.apiKey` for higher Jina Reader rate limits

Never print full API keys. Use `get_config_info`, `grok-search cli config list`, or `grok-search cli doctor` for masked diagnostics.
