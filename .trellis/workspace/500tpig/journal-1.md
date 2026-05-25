# Journal - 500tpig (Part 1)

> AI development session journal
> Started: 2026-05-02

---



## Session 1: TinyFish multi-key fallback

**Date**: 2026-05-06
**Task**: TinyFish multi-key fallback
**Branch**: `main`

### Summary

Integrated TinyFish multi-key Search/Fetch fallback into MCP and CLI routing, documented config/workflow, and verified with go test ./....

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `cdd1e4b` | (see git log) |
| `ff0fdc4` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 2: research_run 并发化 + Codex tool_timeout 调整

**Date**: 2026-05-07
**Task**: research_run 并发化 + Codex tool_timeout 调整
**Branch**: `main`

### Summary

Parallelized research_run search+fetch loops with sync.WaitGroup + semaphore (quick=2/standard=3/deep=4) and 25s per-call timeout, preserving executed_searches and fetched_pages_summary order via fixed-index writes. Added 3 tests (concurrency overlap, order preservation, timeout isolation); go test ./... and go test -race both green. Rebuilt grok-search binary. Updated ~/.codex/config.toml to set tool_timeout_sec=240 + GROK_POOL_TIMEOUT_SEC=60 env on the grok-search MCP server entry — the GUI does not expose timeout settings, must be edited in TOML.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `f1208f3` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 3: Cleanup: bootstrap docs + 05-06 surfaces archive sweep

**Date**: 2026-05-07
**Task**: Cleanup: bootstrap docs + 05-06 surfaces archive sweep
**Branch**: `main`

### Summary

Cleanup sweep over 4 long-running in_progress tasks. 45aa61e — populate backend quality-guidelines spec scenarios + AGENTS scaffolding for 00-bootstrap-guidelines. 550a7c9 — bundle landed code for 05-06-ai-fund-decision-prompt-pack (Tavily Crawl wrapper, web_crawl MCP+CLI, Exa advanced search/contents MCP+CLI, dispatch/config wiring, docs) and 05-06-deep-research-workflow (cli research subcommand, RegisterResearchRun in server.go); split was infeasible because both tasks touch server.go and README.md. 05-06-tinyfish-integration-eval had no uncommitted code (its work shipped earlier via 3515418/cdd1e4b/etc.), archived as bookkeeping. Working tree now fully clean.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `45aa61e` | (see git log) |
| `550a7c9` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 4: CLI-first single-file config productization

**Date**: 2026-05-11
**Task**: CLI-first single-file config productization
**Branch**: `main`

### Summary

Refactored runtime configuration to a single grok-search.json file with explicit --config support, added CLI setup/config surfaces and tests, updated docs/skills/specs, and verified CLI/MCP search/fetch behavior.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `469b4f1` | (see git log) |
| `f0a0c01` | (see git log) |
| `0563427` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 5: DeepSeek smart answer workflow

**Date**: 2026-05-11
**Task**: DeepSeek smart answer workflow
**Branch**: `main`

### Summary

Added a smart_answer MCP/CLI path that runs research evidence gathering first, then synthesizes final answers through configured OpenAI-compatible reasoning endpoints such as DeepSeek Flash/Pro.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `0f6acbb` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 6: Open-source release polish

**Date**: 2026-05-11
**Task**: Open-source release polish
**Branch**: `main`

### Summary

Prepared grok-search-go for public release: removed local AI workflow assets from the tracked tree, added OSS docs/example configs/CI, documented smart_answer setup, and improved missing reasoning endpoint errors.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `422e729` | (see git log) |
| `1bab28e` | (see git log) |
| `d276cca` | (see git log) |
| `4dc23fa` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 7: Add configurable Grok response tools

**Date**: 2026-05-12
**Task**: Add configurable Grok response tools
**Branch**: `main`

### Summary

Added responseTools support for xAI Responses API endpoints, including web_search/x_search validation, CLI setup/config diagnostics, docs, and tests.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `bc30eb7` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 8: Compact MCP outputs and AI routing docs

**Date**: 2026-05-12
**Task**: Compact MCP outputs and AI routing docs
**Branch**: `main`

### Summary

Made MCP search/fetch/research outputs compact, kept CLI as the full-output surface, and documented the new AI integration/routing model.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `8fff854` | (see git log) |
| `c10d615` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 9: Capability routing refactor and release README cleanup

**Date**: 2026-05-13
**Task**: Capability routing refactor and release README cleanup
**Branch**: `main`

### Summary

Implemented capability-typed routing with v2 config migration, route trace output, local-only doctor and mock smoke checks; aligned MCP-first diagnostics docs and added Chinese README/release hygiene updates after tracked-file secret audit.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `e2f4be1` | (see git log) |
| `c998431` | (see git log) |
| `e9b0dc9` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 10: Context7 provider and distribution setup

**Date**: 2026-05-13
**Task**: Context7 provider and distribution setup
**Branch**: `main`

### Summary

Completed Context7 docs_search provider and direct CLI surfaces, added GoReleaser/Homebrew/Scoop distribution setup, documented Context7 and release paths, updated setup wizard support, and recorded optional provider key-normalization spec guidance.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `87e950e` | (see git log) |
| `eaf7e1e` | (see git log) |
| `72f3359` | (see git log) |
| `b56f577` | (see git log) |
| `9c4ed57` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 11: Product rename to SourceMux

**Date**: 2026-05-14
**Task**: Product rename to SourceMux
**Branch**: `main`

### Summary

Renamed the project identity to SourceMux, updated module/import paths and primary sourcemux command, kept grok-search compatibility packaging, renamed config examples/default docs to sourcemux.json, and documented GitHub/release/config migration steps.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `92bf84e` | (see git log) |
| `acb1638` | (see git log) |
| `30e7a95` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 12: Multi-agent installer MVP

**Date**: 2026-05-14
**Task**: Multi-agent installer MVP
**Branch**: `main`

### Summary

Added SourceMux multi-agent install/uninstall surface with routing skill generation, first-tier MCP setup guidance, --binary support, and manifest-backed safe status/uninstall.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `fa5fb54` | (see git log) |
| `ca76667` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 13: Installer safe config merge

**Date**: 2026-05-14
**Task**: Installer safe config merge
**Branch**: `main`

### Summary

Implemented safe MCP client config merging for installer write-config/status flows with backups, warnings, tests, docs, and backend spec updates.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `945b5dd` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 14: SourceMux heavy search controls

**Date**: 2026-05-25
**Task**: SourceMux heavy search controls
**Branch**: `main`

### Summary

Added Grok endpoint profiles plus caller-controlled search fallback timing, no-fallback diagnostics, docs, generated skill guidance, and tests.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `09f0111` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete
