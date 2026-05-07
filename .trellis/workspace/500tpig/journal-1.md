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
