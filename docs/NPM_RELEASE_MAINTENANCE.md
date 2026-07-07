# npm Release Maintenance

This document is for future maintainers and AI agents working on SourceMux's
npm release path. The public release procedure lives in `docs/RELEASE.md`; this
file focuses on what to maintain after normal feature work, what to verify
before a release, and what not to touch accidentally.

## Where this belongs

Keep this document in the SourceMux repository. It is operational knowledge for
the codebase and release workflow.

Use `kk-blog` only for public writeups and retrospectives. Blog posts can
explain what happened, but they are not the source of truth for release steps.

## Current npm shape

The npm distribution keeps SourceMux as a Go CLI. npm is only an install
channel.

```text
npm/package/                 # root package: sourcemux
npm/platforms/darwin-arm64/  # @500tpig/sourcemux-darwin-arm64
npm/platforms/darwin-x64/    # @500tpig/sourcemux-darwin-x64
npm/platforms/linux-arm64/   # @500tpig/sourcemux-linux-arm64
npm/platforms/linux-x64/     # @500tpig/sourcemux-linux-x64
npm/platforms/win32-x64/     # @500tpig/sourcemux-win32-x64
```

The root package exposes the `sourcemux` bin and depends on platform packages
through `optionalDependencies`. The matching platform package supplies the
native `sourcemux` or `sourcemux.exe` binary.

Every root/platform package must keep `repository.url` set to
`https://github.com/500tpig/sourcemux-go`. npm Trusted Publishing validates
that package metadata against the GitHub Actions provenance bundle; empty
platform package repository metadata causes publish to fail.

Do not expose the legacy `grok-search` command through npm unless a separate
task explicitly decides to extend npm migration support.

## Normal feature changes

Most SourceMux feature work does not require changing npm package manifests.

If you change CLI behavior but do not change npm wrapper behavior:

```bash
go test ./...
go vet ./...
go build ./...
npm --prefix npm/package test
npm --prefix npm/package run pack:dry-run
```

Do not manually bump npm package versions during ordinary development. The
release workflow derives the npm version from the pushed tag and runs:

```bash
node npm/scripts/set-package-version.js --version "$RELEASE_VERSION"
```

The checked-in version should normally remain the latest published baseline
until the next release changes it through a deliberate release task.

## Before each release

Start from a clean worktree and run the release preflight from `docs/RELEASE.md`.
For npm-specific confidence, run:

```bash
npm --prefix npm/package test
npm --prefix npm/package run pack:dry-run
```

If npm wrapper, platform mapping, release scripts, or packaging files changed,
also validate the release staging path with staged binaries before publishing.
Use generated or extracted throwaway binaries only for local smoke; do not
commit them:

```bash
node npm/scripts/verify-pack-dry-run.js --require-staged-binaries
git ls-files dist 'npm/platforms/*/bin/*' '*.tgz' sourcemux.json grok-search.json config.local.json .npmrc npmrc
```

The `git ls-files` command must print nothing.

## Release source of truth

Local builds are only for local smoke.

The automated npm release must use the versioned GitHub Release archives
created by GoReleaser. The workflow order must stay:

```text
goreleaser
-> npm-publish
   -> download GitHub Release assets
   -> set npm package versions from the tag
   -> stage platform packages from release assets
   -> verify npm packages
   -> publish platform packages
   -> publish root package
```

The `npm-publish` job must keep:

```yaml
needs: goreleaser
permissions:
  contents: read
  id-token: write
```

Do not add `NPM_TOKEN`, `.npmrc`, or generated npm credentials to the repo.
npm Trusted Publishing/OIDC is the preferred automation path.

## Trusted Publishing configuration

Each npm package needs a trusted publisher configured on npmjs.com:

```text
Provider: GitHub Actions
Organization or user: 500tpig
Repository: sourcemux-go
Workflow filename: release.yml
Allowed actions: npm publish
Environment name: empty
```

Configure all six packages:

```text
sourcemux
@500tpig/sourcemux-darwin-arm64
@500tpig/sourcemux-darwin-x64
@500tpig/sourcemux-linux-arm64
@500tpig/sourcemux-linux-x64
@500tpig/sourcemux-win32-x64
```

The npm UI expects `release.yml`, not `.github/workflows/release.yml`.

For public packages from this public GitHub Actions workflow, npm generates
provenance automatically. Do not add `--provenance` unless current npm official
docs require a change.

## Release asset mapping

Keep the GoReleaser asset names and npm platform names explicit:

```text
sourcemux_<version>_darwin_arm64.tar.gz  -> darwin-arm64
sourcemux_<version>_darwin_amd64.tar.gz  -> darwin-x64
sourcemux_<version>_linux_arm64.tar.gz   -> linux-arm64
sourcemux_<version>_linux_amd64.tar.gz   -> linux-x64
sourcemux_<version>_windows_amd64.zip    -> win32-x64
```

If this mapping changes, update all of these together:

```text
.goreleaser.yaml
.github/workflows/release.yml
npm/package/lib/platform.js
npm/scripts/stage-release-binaries.js
npm/package/test/launcher.test.js
npm/README.md
docs/RELEASE.md
docs/NPM_RELEASE_MAINTENANCE.md
.trellis/spec/backend/quality-guidelines.md
```

Then run:

```bash
npm --prefix npm/package test
npm --prefix npm/package run pack:dry-run
```

## After each release

Verify npm registry metadata:

```bash
npm view sourcemux name version dist-tags bin optionalDependencies --json
npm view @500tpig/sourcemux-darwin-arm64 name version dist-tags os cpu --json
npm view @500tpig/sourcemux-darwin-x64 name version dist-tags os cpu --json
npm view @500tpig/sourcemux-linux-arm64 name version dist-tags os cpu --json
npm view @500tpig/sourcemux-linux-x64 name version dist-tags os cpu --json
npm view @500tpig/sourcemux-win32-x64 name version dist-tags os cpu --json
```

Verify install smoke in an isolated prefix:

```bash
SMOKE_PREFIX="$(mktemp -d)"
npm install --prefix "$SMOKE_PREFIX" sourcemux@<version>
PATH="$SMOKE_PREFIX/node_modules/.bin:$PATH" sourcemux version
npm exec --package sourcemux@<version> -- sourcemux version
```

The printed version must match the release version.

After npm, Homebrew, Scoop, and GitHub Release assets are verified, update the
current public baseline in user-facing docs if needed.

## If npm publish partially fails

npm versions are immutable. Do not try to overwrite a published package version.

First identify which packages exist:

```bash
npm view sourcemux@<version> version
npm view @500tpig/sourcemux-darwin-arm64@<version> version
npm view @500tpig/sourcemux-darwin-x64@<version> version
npm view @500tpig/sourcemux-linux-arm64@<version> version
npm view @500tpig/sourcemux-linux-x64@<version> version
npm view @500tpig/sourcemux-win32-x64@<version> version
```

If the failure was transient or auth-related, prefer GitHub Actions **Re-run
failed jobs** for the failed `npm-publish` job. The workflow skips already
published versions and continues with missing packages.

If a platform package was published with the wrong binary, do not mutate it.
Publish a corrected follow-up version and deprecate the bad version with a clear
message.

If the root package was published before a platform package exists, publish the
missing platform package as soon as possible, then rerun the registry and
install smoke checks.

## When adding a new platform

Treat new platform support as a cross-release change. Update the GoReleaser
matrix, npm platform matrix, tests, docs, and npm trusted publisher settings in
one task.

Checklist:

```text
1. Add GoReleaser build/archive support.
2. Add npm/platforms/<target>/package.json.
3. Add target to npm/package/lib/platform.js.
4. Add release asset mapping in npm/scripts/stage-release-binaries.js.
5. Add or update tests in npm/package/test/launcher.test.js.
6. Add the package to npm/package/package.json optionalDependencies.
7. Add the package to docs/RELEASE.md and this file.
8. Publish the new npm package once.
9. Configure npm Trusted Publishing for the new package.
10. Run registry and install smoke checks.
```

Do not add a target to the root `optionalDependencies` until the corresponding
platform package can actually be published.

## When changing the binary name

Avoid changing the binary name. If it is unavoidable, update all of these
together:

```text
cmd/sourcemux
.goreleaser.yaml
npm/package/bin/sourcemux.js
npm/package/lib/platform.js
npm/scripts/stage-platform-binary.js
npm/scripts/stage-release-binaries.js
Homebrew/Scoop docs and manifests
release smoke commands
```

Then run Go checks, npm tests, package dry-run, and at least one local install
smoke.

## What AI agents must not do

Do not:

```text
commit npm tokens, .npmrc, provider keys, local sourcemux.json, or dashboard exports
commit npm/platforms/*/bin/* staged binaries
commit GoReleaser dist/* snapshots or *.tgz package artifacts
publish npm packages without explicit maintainer approval
rewrite SourceMux in Node as part of npm packaging
claim npm solves macOS signing or notarization
change package names/scopes casually
update public install docs before registry/release/tap/bucket verification
```

If a future AI is unsure whether a release channel is live, verify it from the
source system first: GitHub Release, raw tap cask, raw Scoop manifest, and npm
registry metadata.

## Separate future tasks

Keep these separate from npm release maintenance:

```text
macOS Developer ID signing and notarization
Scrapling/browser adapter exploration
```

npm is an install channel. It does not replace macOS signing/notarization or
runtime diagnostics. Use `sourcemux bootstrap status --config-status --json`
after npm/global install work to check `binary_status`,
`runtime_config_status`, `scope_status`, and `issues[].code` before changing
generated skills.
