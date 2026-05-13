# Context7 Provider and Distribution Setup

## Goal

Continue the archived capability-routing plan by implementing PR2 (Context7 as a library-docs provider and direct CLI surface) and PR3 (Go-native distribution plus setup wizard polish) in small, releasable increments.

## What I already know

* User explicitly wants to continue with:
  * PR2: Context7 integration
    * Add a Context7 provider to `docs_search`.
    * Add `cli context7-library` and `cli context7-docs`.
  * PR3: distribution and setup wizard
    * GoReleaser.
    * Homebrew / Scoop.
    * Interactive setup.
* Archived planning already established:
  * Context7 is a library/framework/API-docs provider, not a general Exa replacement.
  * Context7 should not count toward `minimum_profile=standard`; Exa remains the standard `docs_search` requirement.
  * Distribution should stay Go-native: release binaries, Homebrew, Scoop; no npm/pip wrapper.
  * `setup --non-interactive` must remain script-friendly.
* Current repo already has:
  * Capability router primitives in `internal/capability`, `internal/router`, and `internal/router/adapters`.
  * `docs_search` capability type, but no concrete `docs_search` tool/CLI command wired yet.
  * `setup` interactive prompts today are basic stdin prompts in `internal/cli/setup.go`.
  * No `.goreleaser.yaml` or release workflow currently listed.

## Assumptions

* This task should reuse the archived architecture from `.trellis/tasks/archive/2026-05/05-13-capability-routing-refactor/prd.md`.
* We should implement PR2 first, then PR3, because PR2 changes runtime/provider behavior and PR3 changes packaging/onboarding.
* External satellite repos for Homebrew/Scoop cannot be created locally in this task; this repo should add the GoReleaser config, workflow, and docs assuming `500tpig/homebrew-tap` and `500tpig/scoop-bucket` exist or will be created before first release.

## Requirements

### PR2: Context7 integration

* Add a thin Context7 REST client under `internal/engine/context7.go`.
* Support library resolution and documentation context fetch:
  * `GET /api/v2/libs/search`
  * `GET /api/v2/context`
* Use Bearer auth and safe placeholder examples only.
* Add Context7 provider support to the `docs_search` capability.
* Only call Context7 for clearly applicable library/framework/API-docs requests; otherwise skip without an API call.
* Support named Context7 provider instances in config.
* Preserve transparent route decisions for skip/fallback behavior.
* Add direct CLI commands:
  * `grok-search cli context7-library <name> <query>`
  * `grok-search cli context7-docs <id> <query>`
  * support `--provider <name>` where applicable.
* Add tests for the client, CLI commands, routing eligibility, failure mapping, and config loading.
* Update README/QUICKSTART/MIGRATION or relevant docs for the user-visible config and CLI surface.

### PR3: Distribution and setup wizard

* Add GoReleaser configuration for cross-platform binary releases.
* Add GitHub Actions release workflow.
* Configure Homebrew tap and Scoop bucket publishing in GoReleaser.
* Add version metadata injection (`version`, `commit`, `date`) to the binary.
* Improve interactive `cli setup` into a grouped wizard experience while preserving current non-interactive flags and JSON behavior.
* Document Homebrew, Scoop, and `go install` install paths side by side.

## Acceptance Criteria

* [x] `docs_search` can route eligible library-docs requests to Context7 and non-eligible requests do not spend Context7 quota.
* [x] Context7 direct CLI commands work against local test servers with JSON and human output.
* [x] Config supports Context7 provider instances without leaking keys in `config list`, `doctor`, setup output, or errors.
* [x] `minimum_profile=standard` still requires Exa for `docs_search`; Context7 remains optional.
* [x] GoReleaser config passes local validation where the tool is available, or is structurally documented if the binary is not installed.
* [x] `setup --non-interactive` remains backward-compatible with existing tests.
* [x] Interactive setup groups questions by capability and includes Context7 where appropriate.
* [x] Docs cover Context7 usage, distribution install paths, and first-run setup.
* [x] `gofmt -w <modified-go-files>`, `go test ./...`, `go vet ./...`, and `go build ./...` pass.

## Definition of Done

* Tests added/updated for provider, CLI, config, setup, and release metadata surfaces.
* Required Go checks pass.
* Docs updated for all user-visible behavior.
* No real API keys, provider dashboard exports, or local credential files are committed.
* Release/distribution changes are reproducible from repository files.

## Out of Scope

* Automatically creating or pushing to external Homebrew/Scoop satellite repos from this task.
* Making Context7 the default or required `docs_search` provider.
* Context7 Skills registry installation or IDE skill injection.
* Code signing/notarization beyond practical GoReleaser/Homebrew/Scoop checksum flows.
* Live external API tests in CI.

## Technical Approach

Implement PR2 before PR3:

1. Add Context7 engine client and tests.
2. Add config model and client construction for named Context7 providers.
3. Add `docs_search` routing/provider behavior and direct CLI commands.
4. Update setup/config/doctor/docs for Context7.
5. Add release metadata plumbing, GoReleaser config, release workflow, and install docs.
6. Improve interactive setup, keeping non-interactive path stable.

## Decision (ADR-lite)

**Context**: Context7 adds high-signal library documentation but has quota and applicability limits. Distribution should be low-maintenance for a Go single-binary project.

**Decision**: Treat Context7 as an optional library-docs provider under `docs_search`, keep Exa as the standard `docs_search` requirement, and use GoReleaser plus Homebrew/Scoop for distribution.

**Consequences**: The implementation needs explicit eligibility checks and transparent route decisions to avoid wasting Context7 quota. Release automation needs one-time external repo/token setup, documented but not performed locally.

## Research References

* `../archive/2026-05/05-13-capability-routing-refactor/research/02-context7-integration.md` — Context7 API shape, quota implications, and implementation risks.
* `../archive/2026-05/05-13-capability-routing-refactor/research/03-distribution-homebrew-scoop.md` — GoReleaser/Homebrew/Scoop release design.
* `../archive/2026-05/05-13-capability-routing-refactor/prd.md` — parent architectural plan and PR2/PR3 decomposition.

## Technical Notes

* Current likely files:
  * `internal/config/config.go`
  * `internal/cli/cli.go`
  * `internal/cli/setup.go`
  * `internal/cli/config.go`
  * `internal/cli/doctor.go`
  * `internal/tools/search.go`
  * `internal/router/adapters/`
  * `internal/server/server.go`
  * `cmd/grok-search/main.go`
  * `README.md`
  * `docs/QUICKSTART.md`
  * `docs/MIGRATION.md`
* Project rule: tests must not call live external APIs; use local test servers/fakes.
* Project rule: runtime config stays single-file based; no env-var config chains or hidden home config fallbacks.
