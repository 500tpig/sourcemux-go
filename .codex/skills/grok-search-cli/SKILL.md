---
name: grok-search-cli
description: Use the local `grok-search cli` binary as a one-shot CLI alternative to the MCP server when a task needs Grok-pool web search, Jina/Tavily fetch, site mapping, endpoint probing, or a deterministic search plan — especially for content blocked by Cloudflare or hosted on X/Twitter that generic search engines miss.
---

# Grok Search CLI

The `grok-search` binary in this repo has a `cli` mode that exposes the same engine as the MCP server, but as plain shell commands. Prefer this mode when:

- You can `run_command` but can't (or don't want to) connect to a stdio MCP server.
- You need the Grok pool's coverage of X/Twitter posts, Cloudflare-protected pages, or other content that generic web search misses.
- You want machine-readable JSON to feed into a follow-up step.

The MCP server is still the right choice for interactive multi-turn research. Use whichever fits the task.

## Subcommands

- `search <query>` — Grok pool web search; falls back to Tavily Search if every Grok endpoint fails or returns empty.
- `fetch <url>` — Fetch a URL as Markdown via Jina Reader; Tavily Extract fallback.
- `map <url>` — Discover URLs on a site via Tavily Map (needs `TAVILY_API_KEY`).
- `probe` — Print configured endpoints, key-status (masked), and `/models` probe per endpoint.
- `plan <query>` — Deterministic, offline multi-step search plan. No network calls. Useful before launching multi-round research.

Common flags (subcommand-dependent):

- `--json` — emit machine-readable JSON instead of human text.
- `--platform <name>` — focus a platform, e.g. `Twitter` or `GitHub, Reddit`. Especially useful for X content and CF-blocked sites.
- `--model <name>` — one-shot Grok model override (e.g. `grok-4.20-fast`); does not edit any config file.
- `--timeout <dur>` — per-call timeout, e.g. `60s`, `2m`.
- `--depth <quick|standard|deep>` — only on `plan`.

Flags can appear before or after positional args (`cli search "q" --platform Twitter` works).

## Workflow

1. **First time on a fresh machine**: run `./grok-search cli probe --json` to confirm endpoints are configured and reachable. If it shows `(not set)` for every endpoint, ask the user to populate `~/.config/grok-search/config.json` or set `GROK_ENDPOINTS_JSON`. Don't guess keys.
2. **For current/factual web research**: call `cli search "<question>" --json`. Parse `content` for the answer and `source_urls` for citations.
3. **For X/Twitter or CF-blocked content**: add `--platform Twitter` (or relevant platform name). The Grok pool typically reaches sources that generic engines miss.
4. **For full text of a URL**: `cli fetch "<url>" --json`. If the page is paywalled or JS-heavy, Tavily Extract usually does better — the CLI already falls back automatically.
5. **For site discovery**: `cli map "<url>" --limit 50 --json` (only if Tavily is configured).
6. **For complex multi-step research**: `cli plan "<topic>" --depth deep` first, then execute the suggested searches/fetches.
7. **Never echo full API keys.** `probe` already masks them.

## Configuration

Reuses the MCP server's config chain in this exact order:

1. Environment variables (`GROK_ENDPOINTS_JSON`, `GROK_API_URL`+`GROK_API_KEY`, `TAVILY_API_KEY`, `JINA_API_KEY`, etc.).
2. `~/.config/grok-search/config.json` (`grokEndpoints`, `tavily`, `jina`).
3. Legacy `~/.config/grok-search/endpoints.json` (Grok endpoints only).

If you configured the MCP server, the CLI Just Works.

## Examples

```bash
grok-search cli probe --json
grok-search cli search "X 上 grok 4.20 的最新评价" --platform Twitter --json
grok-search cli search "某 CF 墙后产品的实际口碑" --model grok-4.20-fast
grok-search cli fetch  "https://example.com/article" --json
grok-search cli map    "https://example.com" --limit 50 --json
grok-search cli plan   "OpenAI Atlas 上线情况" --depth deep
```
