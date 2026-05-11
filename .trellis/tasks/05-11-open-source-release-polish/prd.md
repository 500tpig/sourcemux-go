# brainstorm: open-source release polish and smart answer UX

## Goal

Prepare `grok-search-go` for a cleaner public/open-source release by improving project packaging, documentation, safe example configuration, and the first-run experience around `smart_answer` / `reasoningEndpoints`.

## What I already know

* The core product already works: CLI, MCP, `web_search`, `web_fetch`, `research_run`, `smart_answer`, endpoint pools, and tests are in place.
* The user wants to avoid creating a new project and instead polish this repo.
* README is usable for technical users, but still needs open-source release polish.
* `smart_answer` currently works as a research-then-reasoning pipeline, but setup and user-facing docs can be friendlier.
* The project currently has README-level MIT mention, but needs a real `LICENSE` file before public release.

## Assumptions (temporary)

* Keep this task focused on public-release readiness and small UX improvements, not major architecture changes.
* Do not add a web UI, paid-account manager, or true multi-agent orchestration in this task.
* Avoid committing real API keys, private endpoints, or personal absolute paths in examples.

## Open Questions

* Should `.trellis/`, `.agents/`, and `.codex/skills/` remain in the public repo, or should some of them be excluded before release?

## Requirements (evolving)

* Add or verify open-source baseline files:
  * `LICENSE`
  * `SECURITY.md`
  * optionally `CONTRIBUTING.md`
  * optionally `CHANGELOG.md`
* Add safe example config files:
  * base Grok config example
  * DeepSeek / reasoning endpoint example
* Improve README for strangers:
  * clearer Quick Start
  * remove or generalize personal paths
  * clarify CLI vs MCP usage
  * clarify what `smart_answer` is and is not
* Add focused docs if README becomes too long:
  * `docs/QUICKSTART.md`
  * `docs/SMART_ANSWER.md`
  * `docs/TROUBLESHOOTING.md`
  * MCP setup docs for Codex/Claude if needed
* Improve `smart_answer` usability where low-risk:
  * friendlier error when no `reasoningEndpoints` are configured
  * clear docs for Flash vs Pro usage
  * setup/config guidance for reasoning endpoints
* Add CI for baseline quality checks:
  * `go test ./...`
  * `go vet ./...`
  * `go build ./...`

## Acceptance Criteria (evolving)

* [ ] A fresh user can understand the project, install it, create a config, and run one CLI command from the README/Quick Start.
* [ ] A Codex/Claude user can configure MCP without relying on personal absolute paths.
* [ ] Example configs contain no real secrets or private endpoints.
* [ ] `smart_answer` setup is documented clearly, including `reasoningEndpoints`.
* [ ] Missing reasoning endpoint errors point users to the correct config fix.
* [ ] CI runs Go test/vet/build on push or PR.
* [ ] Public-release files exist and are internally consistent.

## Definition of Done (team quality bar)

* Tests added/updated where behavior changes.
* `go test ./...`, `go vet ./...`, and build pass.
* Docs updated for all user-visible behavior.
* No real secrets, private endpoints, or local-only paths in public examples.
* Rollback is simple: docs/config additions and small UX changes only.

## Out of Scope

* Rewriting the CLI/MCP architecture.
* Building a web UI.
* Implementing paid account management.
* Implementing true multi-agent planner/searcher/critic/writer orchestration.
* Publishing binaries or Homebrew formula unless explicitly pulled into this task later.

## Technical Notes

* Main docs entrypoint: `README.md`.
* Existing handoff docs: `docs/HANDOFF.md`.
* Smart answer implementation:
  * `internal/tools/smart_answer.go`
  * `internal/cli/smart_answer.go`
  * `internal/engine/reasoning.go`
  * `internal/server/server.go`
* Config surface:
  * `internal/config/config.go`
  * `internal/cli/setup.go`
  * `internal/cli/config.go`
* Current validation baseline:
  * `go test ./...`
  * `go vet ./...`
  * `go build ./...`
