# AI Usage Guide

This document defines the recommended integration model for `sourcemux-go` across AI coding agents.

## Positioning

`sourcemux-go` should be treated as:

* **Single-binary agent research router** for both local CLI use and stdio MCP
  server mode
* **MCP-native** for compact interactive lookups inside an agent host
* **CLI-peer** for heavy, reproducible, or file-oriented workflows
* **Prompt/skill-routed** so each host knows when to choose MCP vs CLI

The Go engine stays shared. The choice is about the best invocation surface for the workflow.

In practice, this means MCP text responses should stay intentionally thin: enough metadata plus clipped summaries/excerpts for interactive use. The CLI text and especially `--json` outputs are the canonical full-output surfaces for reproducible or downstream processing workflows.

## Why route through SourceMux

Use a single provider directly only for diagnostics or a provider-specific
experiment. For ordinary URL reads, use SourceMux `fetch --profile auto`; use
`fetch --profile cheap` only when the user explicitly asks for cheap,
zero-key, or quick sanity-check extraction. Use SourceMux when the agent
benefits from one or more of these stable
outputs:

* one CLI/MCP surface for search, fetch, docs search, bounded research, and
  synthesis
* fallback across configured providers without changing the agent prompt
* `get_sources` plus follow-up fetch verification for citation-sensitive work
* reproducible CLI commands and JSON output for logs, scripts, or handoff
* `profile=auto` research routing that can use configured heavy Grok search
  while keeping fallback available

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
sourcemux search "query" --profile auto --fallback-after 180s --timeout 300s --json
sourcemux search "query" --platform Twitter --profile auto --fallback-after 180s --timeout 300s --json
sourcemux docs-search "library or API question" --json
sourcemux exa-search "official docs API reference" --type deep --json
sourcemux exa-contents "https://example.com/docs" --subpages 3 --subpage-target api --json
sourcemux fetch "https://example.com" --profile auto --json
sourcemux fetch "https://example.com" --profile cheap --json
sourcemux firecrawl-scrape "https://example.com" --json
sourcemux firecrawl-map "https://example.com" --search "docs" --limit 100 --json
sourcemux plan "research question" --depth standard
sourcemux plan "deep research question" --json --depth deep
sourcemux research "topic" --depth standard --profile auto --json
sourcemux smart-answer "question" --depth standard --profile auto --json
```

Generated `sourcemux-routing` skills should derive one-shot search examples
from `searchPolicy.agentProfile`, `searchPolicy.fallbackAfterSec`, and
`searchPolicy.timeoutSec`. Public configs default raw `search` to
`searchPolicy.defaultProfile=default`; power users can set
`defaultProfile=auto` with `autoPreference=heavy-first` to make raw search
heavy-first.

### Capability selection for generated skills

Generated `sourcemux-routing` skills should route user intent to capabilities,
not just list commands. Treat the skill as the routing/decision layer and the
SourceMux CLI as the execution layer; MCP remains a generic compatibility
surface, not a place for provider-specific Firecrawl tools:

| User intent | Preferred surface |
| --- | --- |
| Fresh topics, community feedback, X/Twitter, controversy, release reaction | `search --platform Twitter --profile <searchPolicy.agentProfile> --fallback-after <searchPolicy.fallbackAfterSec>s --timeout <searchPolicy.timeoutSec>s --json` or the same without `--platform` |
| Official docs, SDK/API reference, product docs, pricing pages | `docs-search --json` |
| Exa-specific deep/source discovery, structured output, or low-noise source search | `exa-search --type deep --json` |
| Known URL content extraction | `fetch --profile auto --json`; GitHub URLs get repository-aware routing, ordinary pages use quality-first provider order |
| Cheap or zero-key known URL extraction | `fetch --profile cheap --json`; this is the Jina-first path |
| Difficult known URL extraction with Firecrawl-specific flags | `firecrawl-scrape --json` as an explicit SourceMux CLI command; ordinary difficult pages should start with `fetch --profile auto` |
| Known URL plus Exa subpage or documentation subtree discovery | `exa-contents --subpages ... --json` |
| Site structure discovery for hard sites, URL inventory, or relevance-filtered sections | `firecrawl-map --search ... --limit ... --json` as an explicit SourceMux CLI command; do not use it for ordinary single-page extraction |
| Explicit slow heavy/multi-agent Grok search | `search --profile heavy --fallback-after 180s --timeout 300s --json` |
| Grok/profile diagnostics only | `search "ping" --profile heavy --grok-pool-timeout 0 --no-fallback --timeout 120s --json` |
| Deep search, 深度搜索, deep research, 深度调研, complex comparison, or verification where decomposition helps | `plan --json --depth deep`, then `research --depth deep --profile auto --json` |
| Multi-source investigation with synthesis | `research --depth standard --profile auto --json` or `research --depth deep --profile auto --json` |
| Planning/decomposition without executing provider calls | `plan --json --depth standard` or `plan --json --depth deep`; use plain `plan --depth` for compatible text output |

Evidence policy:

1. Discover candidate URLs with policy-driven `search`, `docs-search`, `exa-search`, or `research`.
2. Fetch key URLs before high-risk, precise, or source-critical claims.
3. Cite fetched or source URL evidence in the final answer.
4. Treat the fetch provider label, such as `GitHub Provider`, `Firecrawl`, or `Jina Reader`, as URL verification metadata; it does not replace the original search engine/source route.
5. For known URLs, use `fetch --profile auto --json` first. This is policy-first / quality-first: GitHub URLs route through repository-aware enrichment first, ordinary pages prefer Firecrawl when configured, then fallback through Jina / Exa / Tavily / TinyFish.
6. Use `firecrawl-map` only for site structure discovery, URL inventory, or relevance-filtered URL discovery.
7. Use `fetch --profile cheap --json` only when the user asks for cheap, zero-key, or quick sanity-check extraction. Do not call Jina directly for default research.
8. Use `firecrawl-scrape` as an explicit SourceMux CLI direct command only when Firecrawl scrape flags matter.
9. Do not install, configure, or call Firecrawl MCP. Do not connect Firecrawl to `search`, `map`, or MCP direct routes.
10. Do not use `--no-fallback` for user-facing research/search. It is only for explicitly diagnosing whether the selected Grok profile itself can return.
11. For search-capable multi-agent Grok models, configure them in `grokEndpoints[]` with a profile such as `heavy`; `reasoningEndpoints[]` alone is only for final synthesis and will not be used by `search` or `research`.

Fetch routing note:

* Public/default `fetch --profile auto` is policy-first / quality-first. It
  classifies GitHub repo URLs before generic page extraction and prefers
  Firecrawl for ordinary pages when configured.
* `fetch --profile cheap` is the low-cost route: Jina -> Firecrawl -> Exa ->
  Tavily.
* In v2 configs, `capabilities.web_fetch.providers` can still express an
  explicit order for `--profile auto`; otherwise SourceMux chooses the policy
  order.
* For agents, the fetch provider label explains how that URL was extracted; it
  does not replace source discovery, citation review, or the research route.
* Top-level `firecrawl` enables Firecrawl direct commands and ordinary
  policy-first fetch when keys are configured. Firecrawl MCP is not used.

Firecrawl local smoke:

Do not send Firecrawl keys through chat. Put keys only in the local SourceMux
config file and set `enabled` explicitly. Use `apiKey` for one key, or `keys[]`
for a named key pool:

```json
{
  "firecrawl": {
    "apiURL": "https://api.firecrawl.dev/v2",
    "keys": [
      {"name": "acct-a", "apiKey": "fc-your-key-a"},
      {"name": "acct-b", "apiKey": "fc-your-key-b"}
    ],
    "enabled": true
  }
}
```

Explicit v2 fetch ordering is optional:

```json
{
  "version": 2,
  "minimum_profile": "off",
  "capabilities": {
    "main_search": {"providers": []},
    "docs_search": {"providers": []},
    "web_fetch": {
      "providers": [
        {
          "type": "firecrawl",
          "apiURL": "https://api.firecrawl.dev/v2",
          "keys": [
            {"name": "acct-a", "apiKey": "fc-your-key-a"},
            {"name": "acct-b", "apiKey": "fc-your-key-b"}
          ],
          "enabled": true
        },
        {"type": "jina", "apiURL": "https://r.jina.ai"}
      ]
    },
    "web_enhance": {"providers": []}
  }
}
```

After the local key is filled in, smoke these paths:

```bash
sourcemux --config ~/.config/sourcemux/sourcemux.json fetch "https://example.com" --profile auto --json \
  | jq -e '.policy.effective_profile == "auto" and (.content | length > 0)'

sourcemux --config ~/.config/sourcemux/sourcemux.json firecrawl-scrape "https://example.com" --json \
  | jq -e '.source == "Firecrawl Scrape" and .route_decision[0].provider == "firecrawl-scrape" and (.content | length > 0)'

sourcemux --config ~/.config/sourcemux/sourcemux.json firecrawl-map "https://example.com" --search "docs" --limit 100 --json \
  | jq -e '.source == "Firecrawl Map" and .route_decision[0].provider == "firecrawl-map" and (.count >= 0)'
```

Expected coverage: the first command exercises ordinary policy-first fetch; the
second verifies explicit scrape; the third verifies explicit map.

## Recommended host setup

### Public user mode

For normal users, install `sourcemux` on `PATH` and keep the provider config at
`~/.config/sourcemux/sourcemux.json`. Runtime commands should pass that path
explicitly:

```bash
sourcemux --config ~/.config/sourcemux/sourcemux.json doctor --json
sourcemux --config ~/.config/sourcemux/sourcemux.json search "query" --profile auto --fallback-after 180s --timeout 300s --json
```

Use the installer in user scope. `bootstrap --scope user` defaults generated
skills to `~/.config/sourcemux/sourcemux.json`, so user-scope skills do not
inherit a maintainer's source checkout path:

```bash
sourcemux bootstrap list-agents
sourcemux bootstrap codex claude-code gemini opencode --scope user --dry-run
sourcemux bootstrap codex --scope user
sourcemux bootstrap update codex --scope user
sourcemux bootstrap status --scope user --config-status
```

Use `--write-config` only when the user explicitly wants SourceMux to safely
merge supported MCP client config files:

```bash
sourcemux bootstrap codex --scope user --write-config --dry-run --json
```

### Project development mode

Use project scope only when working from a source checkout or intentionally
installing a skill for a repository. Project scope defaults generated skills to
`./sourcemux.json`.

```bash
go build -o sourcemux .
./sourcemux bootstrap codex --scope project --binary "$(pwd)/sourcemux"
./sourcemux bootstrap update codex --scope project --binary "$(pwd)/sourcemux"
./sourcemux bootstrap status --scope project --config-status
```

Pass `--binary` when running from a source checkout or through `go run`. The
binary path and configured `--config` path are embedded into generated CLI
examples. Without `--write-config`, generated skills are CLI-first and should
not tell the host to call SourceMux MCP tools.

Each generated skill directory gets a `.sourcemux-install.json` manifest. The
manifest records the target and content hash, so `bootstrap status` can report
managed/modified state, `bootstrap update` can refresh unmodified generated
skills, and `uninstall` can refuse to remove user-edited or pre-manifest files
unless `--force` backs them up first.
Status JSON also includes compact diagnostics for stale generated metadata:

* `binary_status` checks the manifest's SourceMux binary path and reports
  `missing_binary` or `stale_binary` issues when the path is gone, temporary, or
  differs from the current/`--binary` path.
* `runtime_config_status` is emitted with `--config-status`; it checks the
  manifest's SourceMux `--config` file path and reports `missing_config` or
  `stale_config` without searching hidden fallback locations.
* `scope_status` reports `wrong_scope` when a project-scope skill is found while
  checking user scope, or the reverse.
* `config_status` remains the supported MCP client config entry check; it is
  separate from `runtime_config_status`.

If a generated skill references a missing/stale binary or config path, run
`sourcemux bootstrap status <target> --scope <scope> --config-status --json`
and then update with the intended binary and explicit config path:

```bash
sourcemux bootstrap update <target> --scope <scope> --binary /absolute/path/to/sourcemux --config <intended-config>
```

Do not silently swap user-scope skills to a maintainer-local project config or
invent hidden config fallbacks.

Pass `--write-config` to safely merge supported MCP client config files for
Codex, Gemini, and OpenCode without invoking external agent CLIs. Existing
matching `sourcemux` entries are reported as unchanged; drifted entries may be
updated, but the plan and JSON output show that a timestamped backup will be
created first and why. Dry-runs show the same backup intent without writing
files. The current writers preserve config semantics, unrelated keys, and
unrelated MCP entries, but may reserialize/reformat Codex TOML, Gemini JSON,
and OpenCode JSONC; comments and original formatting are not guaranteed to be
preserved, so backups are the rollback path. `sourcemux uninstall <target> --write-config`
removes only the `sourcemux` MCP entry and preserves unrelated keys plus the
config file itself.

The first implementation uses a two-tier support model:

* full first-tier targets: `codex`, `claude-code`, `gemini`, `opencode`
* skill/JSON/profile first targets: `copilot`, `cursor`, `trellis`, `mcp-json`, `stdio`

For first-tier targets, official MCP setup guidance is emitted only when
`--write-config` is requested:

| Target | MCP guidance emitted |
| --- | --- |
| `codex` | `codex mcp add ...` plus a `config.toml` snippet |
| `claude-code` | `claude mcp add --transport stdio --scope ...` plus MCP JSON |
| `gemini` | `gemini mcp add --scope ...` plus a `settings.json` snippet |
| `opencode` | `opencode.json` / JSONC `mcp` snippet |

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
