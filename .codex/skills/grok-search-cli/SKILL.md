---
name: grok-search-cli
description: Use the local `grok-search cli` binary as a one-shot CLI alternative to the MCP server when a task needs Grok-pool web search, Jina/Exa/Tavily fetch, site mapping, endpoint probing, or a deterministic search plan — especially for content blocked by Cloudflare or hosted on X/Twitter that generic search engines miss.
---

# Grok Search CLI

The `grok-search` binary in this repo has a `cli` mode that exposes the same engine as the MCP server, but as plain shell commands. Treat the CLI as the reproducible execution surface. Prefer this mode when:

- You can `run_command` but can't (or don't want to) connect to a stdio MCP server.
- You are in a long-running/high-frequency local agent workflow and want stable one-shot process boundaries.
- You need the Grok pool's coverage of X/Twitter posts, Cloudflare-protected pages, or other content that generic web search misses.
- You want machine-readable JSON to feed into a follow-up step.
- You need a command that can be pasted into a bug report, PR, CI job, or terminal.

The MCP server is still useful as a client integration layer for Codex/Claude tools. If MCP feels slow, unavailable, or hard to reproduce, switch to this CLI without changing the single config file.

## Subcommands

- `search <query>` — Grok pool web search; falls back to TinyFish Search, Exa Search, then Tavily Search if every Grok endpoint fails or returns empty/unavailable.
- `fetch <url>` — Fetch a URL as Markdown via Jina Reader; TinyFish Fetch, Exa Contents, then Tavily Extract fallback.
- `map <url>` — Discover URLs on a site via Tavily Map (needs `tavily.apiKey` in `grok-search.json`).
- `doctor` — Print configured endpoints, key-status (masked), and `/models` probe per endpoint.
- `probe` — Backward-compatible alias for `doctor`.
- `config path` — Print the active single config file path.
- `config files` — Print the active config file status and loading notes without reading hidden legacy config.
- `config list` — Print effective config with all secrets masked; does not probe network.
- `setup` — Write `grok-search.json` so users do not have to hand-edit JSON.
- `plan <query>` — Deterministic, offline multi-step search plan. No network calls. Useful before launching multi-round research.
- `research <query>` — Run the bounded research workflow and return a compact research pack.

Common flags (subcommand-dependent):

- `--json` — emit machine-readable JSON instead of human text.
- `--config <path>` — global flag; select one explicit JSON config file instead of the default `./grok-search.json`.
- `--platform <name>` — focus a platform, e.g. `Twitter` or `GitHub, Reddit`. Especially useful for X content and CF-blocked sites.
- `--model <name>` — one-shot Grok model override (e.g. `grok-4.20-fast`); does not edit any config file.
- `--timeout <dur>` — per-call timeout, e.g. `60s`, `2m`.
- `--depth <quick|standard|deep>` — only on `plan`.

Flags can appear before or after positional args (`cli search "q" --platform Twitter` works).

## Workflow

1. **First time on a fresh machine**: run `./grok-search cli config path`, then `./grok-search cli setup --non-interactive ...` if no `./grok-search.json` exists. Use `./grok-search cli --config /path/to/grok-search.json ...` when the user wants the single file outside the current directory.
2. **Connectivity check**: run `./grok-search cli doctor --json` to confirm endpoints are configured and reachable.
3. **For current/factual web research**: call `cli search "<question>" --json`. Parse `content` for the answer and `source_urls` for citations.
4. **For X/Twitter or CF-blocked content**: add `--platform Twitter` (or relevant platform name). The Grok pool typically reaches sources that generic engines miss.
5. **For full text of a URL**: `cli fetch "<url>" --json`. If Jina misses, Exa Contents is tried before Tavily Extract — the CLI falls back automatically.
6. **For site discovery**: `cli map "<url>" --limit 50 --json` (only if Tavily is configured).
7. **For complex multi-step research**: `cli research "<topic>" --depth deep --json`, or `cli plan "<topic>" --depth deep` first if you want to review the search plan.
8. **Never echo full API keys.** `config list` and `doctor` already mask them.

## Configuration

The CLI and MCP server read one file only:

- default: `./grok-search.json`
- explicit: `grok-search cli --config /path/to/grok-search.json ...`

There is no runtime environment-variable config chain, no `~/.config/grok-search` loading, and no legacy `endpoints.json` fallback. If historical files exist, tell the user to copy the needed fields into the single active file; do not rely on hidden files.

Use `grok-search cli setup --non-interactive --api-url <url> --api-key <key> --json` to create the single file without hand-editing JSON. It refuses to overwrite unless `--force` is passed.

## Examples

```bash
grok-search cli config path
grok-search cli config files --json
grok-search cli config list --json
grok-search cli --config /secure/path/grok-search.json config list --json
grok-search cli setup --non-interactive --api-url "https://your-endpoint/v1" --api-key "sk-..." --json
grok-search cli doctor --json
grok-search cli search "X 上 grok 4.20 的最新评价" --platform Twitter --json
grok-search cli search "某 CF 墙后产品的实际口碑" --model grok-4.20-fast
grok-search cli fetch  "https://example.com/article" --json
grok-search cli map    "https://example.com" --limit 50 --json
grok-search cli plan   "OpenAI Atlas 上线情况" --depth deep
grok-search cli research "OpenAI Atlas 上线情况" --depth deep --json
```
