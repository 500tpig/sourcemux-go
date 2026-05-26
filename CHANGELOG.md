# Changelog

## Unreleased

- Added open-source baseline documentation and CI checks.
- Added safe example config files for search and reasoning endpoints.
- Documented `smart_answer` and `reasoningEndpoints`.
- Improved the missing `reasoningEndpoints` error for `smart_answer`.
- Clarified heavy/multi-agent search routing: `--no-fallback` is diagnostics-only; user-facing heavy search should preserve TinyFish/Exa/Tavily fallback.
- Updated `grokPoolTimeoutSec` recommendation to 300 s for multi-agent models.
- Extended generated routing skill with mode-selection table, profile policy, and diagnostics workflow.
