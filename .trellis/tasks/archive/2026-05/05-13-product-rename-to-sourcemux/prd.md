# brainstorm: product rename to SourceMux

## Goal

Rename the project from `grok-search-go` / `grok-search` to `SourceMux`, so the public identity matches the current product scope: MCP-native search, fetch, docs, and research routing across multiple providers.

## What I already know

* User agrees the current name is too narrow and wants a better product name.
* Proposed direction is `SourceMux`.
* User asked whether GitHub also needs to be modified.
* Original repository remote was `https://github.com/500tpig/grok-search-go.git`.
* Original module path was `github.com/500tpig/grok-search-go`.
* Original binary / CLI examples used `grok-search`.
* Original release config used `project_name: grok-search` and published `grok-search` Homebrew/Scoop artifacts.
* Original docs and examples contained many references to `grok-search-go`, `grok-search`, `grok-search.json`, and `Grok Search`.

## Assumptions (temporary)

* Public product name should become `SourceMux`.
* GitHub repository should likely become `500tpig/sourcemux-go`.
* CLI binary should likely become `sourcemux`.
* User approved the compatibility rename path: SourceMux becomes the primary brand while the old `grok-search` command/config story remains documented during migration.

## Open Questions

* None blocking.

## Requirements (evolving)

* Rename public-facing project identity to `SourceMux`.
* Use compatibility rename policy:
  * Primary binary/package/docs name becomes `sourcemux`.
  * Keep a `cmd/grok-search` compatibility command for one migration window where practical.
  * Keep legacy `grok-search.json` support documented; prefer avoiding automatic hidden config chains beyond local explicit/default config behavior.
* Update GitHub-facing references:
  * repository URL references,
  * README badges,
  * GoReleaser GitHub release owner/name target,
  * Homebrew/Scoop homepage and package naming if command changes.
* Update Go module import path if the repository is renamed.
* Update internal imports to the new module path.
* Update binary name, usage text, docs examples, MCP registration examples, and install commands according to the chosen compatibility policy.
* Update docs and specs so future tasks use the new name.
* Preserve a clear migration story from existing `grok-search` installs/configs.
* Document user-operated GitHub repository rename steps; do not perform remote rename automatically.

## Acceptance Criteria (evolving)

* [x] `go test ./...`, `go vet ./...`, and `go build ./...` pass after the rename.
* [x] README and docs consistently present the new project name.
* [x] Go module/import paths match the target GitHub repo path.
* [x] GoReleaser, Homebrew, and Scoop configuration use the target release/package names.
* [x] CLI usage and MCP examples use the target binary name.
* [x] Legacy `grok-search` compatibility behavior is either implemented or explicitly documented.
* [x] Backward compatibility policy is documented.
* [x] Local git remote rename steps are documented for the user.

## Definition of Done (team quality bar)

* Tests added/updated where behavior or strings change.
* Required Go checks pass.
* Docs/notes updated for install, migration, release, MCP setup, and config usage.
* Rollout/rollback documented for GitHub repo rename and package manager rename.

## Out of Scope (explicit)

* Actually renaming the GitHub repository via GitHub UI/API unless the user explicitly asks for remote operations.
* Creating or pushing external Homebrew/Scoop satellite repositories.
* Publishing a release tag.

## Technical Notes

* Likely affected files include `go.mod`, all Go imports under `cmd/` and `internal/`, `.goreleaser.yaml`, `.github/workflows/release.yml`, README, docs, config examples, and tests with CLI text assertions.
* `rg` found many current-name references across README, docs, config examples, internal CLI usage strings, release config, and `.trellis/spec/backend/quality-guidelines.md`.
* GitHub repo rename should be coordinated with:
  * GitHub repository name (`grok-search-go` -> `sourcemux-go`),
  * local `origin` URL,
  * Go module path,
  * release workflow target repo,
  * `go install` path,
  * Homebrew/Scoop package naming.
