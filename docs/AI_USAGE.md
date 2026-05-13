# AI Usage Guide

This document defines the recommended integration model for `sourcemux-go` across AI coding agents.

## Positioning

`sourcemux-go` should be treated as:

* **MCP-native** for compact interactive lookups inside an agent host
* **CLI-peer** for heavy, reproducible, or file-oriented workflows
* **Prompt/skill-routed** so each host knows when to choose MCP vs CLI

The Go engine stays shared. The choice is about the best invocation surface for the workflow.

In practice, this means MCP text responses should stay intentionally thin: enough metadata plus clipped summaries/excerpts for interactive use. The CLI text and especially `--json` outputs are the canonical full-output surfaces for reproducible or downstream processing workflows.

## What is portable vs host-specific

### Portable across agents

These should be considered the canonical, reusable layer:

* the `sourcemux` binary
* CLI command shapes
* JSON output contracts
* routing policy
* provider fallback behavior

### Host-specific

These need per-host adaptation:

* Codex `AGENTS.md`
* Codex skills under `~/.codex/skills`
* Claude Code `CLAUDE.md`
* MCP server wiring in each host

**Important:** a Codex skill is not a universal skill format for all agents. The policy can be shared, but the packaging is host-specific.

## Routing policy

### Prefer MCP when

Use MCP for compact interactive work:

* quick current-information lookups
* quick source discovery
* short citation verification in the same conversation
* short site discovery or crawl summaries
* bounded research where the output will remain compact
* summary-first inspection before deciding whether a full CLI run is needed

Recommended MCP chain for reliable citations:

1. `web_search`
2. `get_sources`
3. `web_fetch` on key URLs

### Prefer CLI when

Use CLI when the workflow should be reproducible or the result may be large:

* deep research
* long page fetches
* full JSON is required
* results should be written to files
* downstream shell/script processing is expected
* the host does not expose sourcemux MCP
* the result would otherwise flood the model context

Typical commands:

```bash
sourcemux cli search "query" --json
sourcemux cli fetch "https://example.com" --json
sourcemux cli research "topic" --depth standard --json
sourcemux cli smart-answer "question" --depth standard --json
```

## Recommended host setup

### Codex

Use both:

* a concise global `AGENTS.md`
* a Codex-specific skill for sourcemux routing

The prompt should keep only the high-level routing rules. The skill can carry the more detailed usage logic.

### Claude Code

Use:

* a global `CLAUDE.md`
* sourcemux MCP wiring
* explicit CLI path examples for heavy workflows

Claude Code does not natively consume Codex skill packages, so keep its routing rules in `CLAUDE.md`.

### Shell-only or generic agents

Use CLI directly. This is the most portable integration path because it relies only on:

* the binary
* the config file
* stable JSON output

## Language recommendation

For reusable cross-agent operational guidance, prefer English. It is easier to reuse across hosts and more consistent with tool names, command examples, and API terminology.

User-facing answers can still be localized per host.

## Guardrails

* Do not bypass server-side fallback routing unless the user explicitly asks.
* Do not override the model unless the user explicitly asks.
* Do not run diagnostics or benchmarks unless requested.
* Do not paste full fetched page text into the conversation unless the user explicitly needs it.
