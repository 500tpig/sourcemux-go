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
