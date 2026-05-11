# brainstorm: CLI-first open-source productization

## Goal

Reposition `grok-search-go` from "MCP server with a CLI mode" into a CLI-first open-source search/research tool that also exposes MCP tools for Codex/Claude clients. The goal is to make the project easier to install, configure, diagnose, explain, and share without weakening the current Grok endpoint pool, fallback chain, and research workflow.

## What I already know

* User wants to start the productization/open-source-oriented refactor now.
* User is concerned that asking people to manually create extra config files is unfriendly.
* The project already has both MCP and CLI surfaces.
* The current README already documents `grok-search cli <subcommand>`.
* Existing CLI commands include `search`, `fetch`, `exa-search`, `exa-contents`, `map`, `crawl`, `probe`, `plan`, `research`, and `tinyfish-bench`.
* Latest user direction rejects multiple config sources, hidden home-directory config, environment-variable runtime config, and compatibility fallback layers.
* Current `probe` already masks keys and probes `/models`, but the name is less beginner-friendly than `doctor`.
* Current local article and `smartsearch` comparison suggest a useful framing: Skill routes, CLI executes, MCP remains a client integration layer.

## Assumptions (temporary)

* Do not remove MCP; instead make CLI the primary documented/runtime path and MCP the integration path.
* Use one explicit config file as the product contract: default `./grok-search.json`, override with `--config`.
* Do not preserve compatibility for env vars, `~/.config/grok-search/*`, or legacy `endpoints.json`.
* Preserve MCP tool names and core CLI command names where they still fit the single-file design.
* Keep Go single-binary deployment as a project advantage.

## Open Questions

* Which public-facing install path should be the first-class target for v1: Go-native install only, or a wrapper distribution such as npm/homebrew later?

## Requirements (evolving)

* Add a beginner-friendly CLI configuration surface:
  * `grok-search cli doctor` as an alias or successor to `probe`.
  * `grok-search cli config path`.
  * `grok-search cli config files --json`, showing only the active single config file and loading notes.
  * `grok-search cli config list --json`, masking secrets.
  * `grok-search cli setup` that writes the active `grok-search.json`.
* Avoid forcing users to manually create config files:
  * `setup` should create the single config file without requiring manual JSON.
  * Error messages should tell users the fastest next command.
* Update docs and skills to explain the new positioning:
  * CLI-first for reproducibility and high-frequency local agent use.
  * MCP for Codex/Claude/Cherry Studio integration.
  * CLI fallback when MCP session/connection is slow or unavailable.
* Preserve existing behavior:
  * Existing `probe` continues to work.
  * Existing MCP tools continue to work.

## Acceptance Criteria (evolving)

* [x] A fresh user can discover the config path with one command.
* [x] A user with historical config files can see that only the active single file is loaded and hidden legacy files are ignored.
* [x] A fresh user can see masked effective config without opening files manually.
* [x] A fresh user can run `config list --json` and get actionable next steps when the config file is missing.
* [x] Existing `probe` usage still works.
* [x] `grok-search cli setup --non-interactive` writes the existing config format and refuses to overwrite unless `--force` is passed.
* [x] README positions the project as CLI-first + MCP integration.
* [x] Codex skills document when to prefer MCP vs CLI.
* [x] Tests cover new CLI command dispatch, setup behavior, and config output masking.

## Definition of Done (team quality bar)

* Tests added/updated where appropriate.
* `go test ./...` passes.
* Docs/skills updated if behavior changes.
* Single-file migration path considered and documented.
* No real API keys or local secrets are printed.

## Out of Scope (explicit)

* Removing MCP.
* Replacing the Go binary with a Python/Node implementation.
* Preserving old env/home/legacy config behavior.
* Publishing to npm/homebrew in this first task, unless explicitly chosen as v1 scope.
* Adding unrelated providers or changing the search/fetch fallback order.

## Technical Notes

* Existing CLI dispatcher: `internal/cli/cli.go`
* Existing probe command: `internal/cli/probe.go`
* Existing config loader: `internal/config/config.go`
* Existing README CLI/config sections: `README.md`
* Existing Codex skills: `.codex/skills/grok-search-mcp/SKILL.md`, `.codex/skills/grok-search-cli/SKILL.md`
* New config contract: `./grok-search.json` by default, `--config /path/to/grok-search.json` when explicit; no env/home/legacy fallback.
* Reference tool: `konbakuyomu/smartsearch` uses CLI-first framing, `setup`, `doctor`, masked config listing, and npm wrapper distribution.
* Reference article: local downloaded Markdown argues that long-running local agent workflows benefit from Skill + CLI, with MCP as optional/protocol integration.
* Implemented first no-regret tranche: `doctor` alias, `config path`, `config list`, exported config path helpers, README repositioning, CLI/MCP skill updates, tests, `go test ./...`.
* Implemented setup tranche: `grok-search cli setup` supports interactive prompts and non-interactive flags, writes the active `grok-search.json`, masks JSON output, refuses overwrite by default, supports optional Tavily/Exa/Jina/TinyFish keys, and keeps prompts on stderr.
* Updated code-spec for CLI config/setup surfaces in `.trellis/spec/backend/quality-guidelines.md` and indexed it from `.trellis/spec/backend/index.md`.
* Adjusted direction after user feedback: remove env/home/legacy compatibility, make one active file the only runtime config source, and document how to copy historical fields into `grok-search.json`.
* Added global `--config` for server and CLI mode so the one file can live outside the current directory when needed.
