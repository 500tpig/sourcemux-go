# Release

This project uses GoReleaser for tagged releases.

For ongoing npm release upkeep, read `docs/NPM_RELEASE_MAINTENANCE.md` before
changing npm package manifests, platform mappings, release scripts, or the
`npm-publish` workflow job.

## Current public channel state

The current verified public baseline is `v0.2.1` (checked 2026-06-03):

* GitHub Release: `https://github.com/500tpig/sourcemux-go/releases/tag/v0.2.1`
* Homebrew cask: `500tpig/homebrew-tap`, `Casks/sourcemux.rb`, version `0.2.1`
* Scoop manifest: `500tpig/scoop-bucket`, `sourcemux.json`, version `0.2.1`
* npm: `sourcemux`, version `0.2.1`

For future releases, do not update public docs to claim Homebrew, Scoop, or npm
availability until the corresponding GitHub Release assets, raw tap cask, raw
bucket manifest, and npm registry packages have been verified.

## One-time setup

Create the satellite repositories used by GoReleaser:

* `500tpig/homebrew-tap`
* `500tpig/scoop-bucket`

Then add repository secrets to `500tpig/sourcemux-go`:

* `HOMEBREW_TAP_TOKEN` with contents write access to `500tpig/homebrew-tap`
* `SCOOP_BUCKET_TOKEN` with contents write access to `500tpig/scoop-bucket`

The default `GITHUB_TOKEN` publishes the GitHub Release in this repository, but
it cannot push to the separate tap/bucket repositories.

For npm automation, configure npm Trusted Publishing for every published npm
package before relying on the release workflow:

* `sourcemux`
* `@500tpig/sourcemux-darwin-arm64`
* `@500tpig/sourcemux-darwin-x64`
* `@500tpig/sourcemux-linux-arm64`
* `@500tpig/sourcemux-linux-x64`
* `@500tpig/sourcemux-win32-x64`

Each trusted publisher should point at the public GitHub repository
`500tpig/sourcemux-go` and workflow filename `release.yml` (the npm UI expects
only the filename, not `.github/workflows/release.yml`). The workflow uses
GitHub OIDC (`id-token: write`) and npm Trusted Publishing; for public packages
from this public GitHub Actions workflow, npm generates provenance
automatically. Do not add `NPM_TOKEN`, `.npmrc`, generated credentials, or
long-lived npm tokens to the repository.

If this is the first release after the product rename, rename the GitHub
repository from `500tpig/grok-search-go` to `500tpig/sourcemux-go` before
tagging. Existing local clones should then run:

```bash
git remote set-url origin https://github.com/500tpig/sourcemux-go.git
```

## Release flow

### Preflight

Start from a clean worktree and run the normal quality gate:

```bash
git status --short
go test ./...
go vet ./...
go build ./...
```

If the npm wrapper changed, also run:

```bash
node --test npm/package/test
```

If GoReleaser is installed, also validate the release config and create a local
snapshot before tagging:

```bash
goreleaser check
goreleaser release --snapshot --clean
```

Local `dist/*` files from snapshot runs are generated artifacts and are ignored
by Git. They are useful for inspecting shape only; they are not source of truth
for public release state and may be stale. Verify public state from GitHub
Releases plus the raw tap/bucket files instead.

### Tag and publish

```bash
git tag v0.4.0
git push origin v0.4.0
```

The release workflow runs tests, vet, build, then `goreleaser release --clean`.
GoReleaser builds:

* macOS amd64 / arm64
* Linux amd64 / arm64
* Windows amd64

The archives include the primary `sourcemux` binary and the legacy
`grok-search` compatibility binary for one migration window.

It also publishes:

* GitHub Release archives and `checksums.txt`
* Homebrew cask in `500tpig/homebrew-tap`
* Scoop manifest in `500tpig/scoop-bucket`

After the GoReleaser job succeeds, the npm publish job downloads the versioned
GitHub Release archives and stages each npm platform package from the matching
released `sourcemux` binary:

* `sourcemux_<version>_darwin_arm64.tar.gz` -> `@500tpig/sourcemux-darwin-arm64`
* `sourcemux_<version>_darwin_amd64.tar.gz` -> `@500tpig/sourcemux-darwin-x64`
* `sourcemux_<version>_linux_arm64.tar.gz` -> `@500tpig/sourcemux-linux-arm64`
* `sourcemux_<version>_linux_amd64.tar.gz` -> `@500tpig/sourcemux-linux-x64`
* `sourcemux_<version>_windows_amd64.zip` -> `@500tpig/sourcemux-win32-x64`

The workflow sets all npm package versions to the tag version, verifies the
packed file lists with staged binaries required, publishes platform packages
first, then publishes the root `sourcemux` package. It does not use or require
an npm token when Trusted Publishing is configured.

`brew install sourcemux` only works if SourceMux is later accepted into
Homebrew core; the release tap path is:

```bash
brew tap 500tpig/tap
brew install --cask sourcemux
```

Scoop uses the custom bucket:

```powershell
scoop bucket add 500tpig https://github.com/500tpig/scoop-bucket.git
scoop install 500tpig/sourcemux
```

## Release closeout checklist

### 1. GitHub Release artifacts

Verify the intended tag exists and is not draft/prerelease unless that was
intentional:

```text
https://api.github.com/repos/500tpig/sourcemux-go/releases/latest
https://github.com/500tpig/sourcemux-go/releases/tag/<tag>
```

Check that the release has:

* `checksums.txt`
* `sourcemux_<version>_darwin_amd64.tar.gz`
* `sourcemux_<version>_darwin_arm64.tar.gz`
* `sourcemux_<version>_linux_amd64.tar.gz`
* `sourcemux_<version>_linux_arm64.tar.gz`
* `sourcemux_<version>_windows_amd64.zip`

Archive names use the version without the leading `v`, for example
`sourcemux_0.2.1_linux_amd64.tar.gz`.

### 2. Package-manager manifests

Verify the raw Homebrew cask:

```text
https://raw.githubusercontent.com/500tpig/homebrew-tap/main/Casks/sourcemux.rb
```

Check:

* `version` matches the release.
* darwin/linux URLs point at the release tag and expected asset names.
* SHA256 values match `checksums.txt`.
* Both `binary "sourcemux"` and `binary "grok-search"` are present for the
  migration window.

Verify the raw Scoop manifest:

```text
https://raw.githubusercontent.com/500tpig/scoop-bucket/main/sourcemux.json
```

Check:

* `version` matches the release.
* the Windows amd64 URL points at `sourcemux_<version>_windows_amd64.zip`.
* the hash matches `checksums.txt`.
* `bin` includes both `sourcemux.exe` and `grok-search.exe` for the migration
  window.

### 3. npm wrapper pack and publish smoke

The npm package is a public channel once `sourcemux` and every platform package
are published and verified in the npm registry.

For local smoke before a future npm publish, stage only the current platform's
`sourcemux` binary into its platform package:

```bash
go build -o /tmp/sourcemux-local ./cmd/sourcemux
node npm/scripts/stage-platform-binary.js --target darwin-arm64 --binary /tmp/sourcemux-local
```

Use the target that matches the binary you staged:

* `darwin-x64`
* `darwin-arm64`
* `linux-x64`
* `linux-arm64`
* `win32-x64`

Then pack the root package and the matching platform package into a temporary
directory and inspect the tarballs before installing them into an isolated
prefix. First run the all-package dry-run verifier:

```bash
npm --prefix npm/package run pack:dry-run
```

After staging every platform binary for an approved release, require the staged
binary paths as well:

```bash
node npm/scripts/verify-pack-dry-run.js --require-staged-binaries
```

```bash
PACK_DIR="$(mktemp -d)"
PLATFORM_TARBALL="$(npm pack ./npm/platforms/darwin-arm64 --pack-destination "$PACK_DIR" --json | node -e 'let input=""; process.stdin.on("data", c => input += c); process.stdin.on("end", () => process.stdout.write(JSON.parse(input)[0].filename));')"
ROOT_TARBALL="$(npm pack ./npm/package --pack-destination "$PACK_DIR" --json | node -e 'let input=""; process.stdin.on("data", c => input += c); process.stdin.on("end", () => process.stdout.write(JSON.parse(input)[0].filename));')"
npm install --prefix "$PACK_DIR/smoke" "$PACK_DIR/$PLATFORM_TARBALL" "$PACK_DIR/$ROOT_TARBALL"
PATH="$PACK_DIR/smoke/node_modules/.bin:$PATH" sourcemux version
```

Before npm publication, do a separate publication precheck:

* Recheck package-name availability and ownership for `sourcemux`; use
  `@500tpig/sourcemux` only as the fallback root package name if unscoped
  publication is not possible.
* Remove `private: true` only after publication is approved.
* Keep root package version and all platform package versions identical to the
  SourceMux release version.
* Run `npm --prefix npm/package run pack:dry-run` and, after staging every
  platform binary, `node npm/scripts/verify-pack-dry-run.js
  --require-staged-binaries`. Verify no provider API keys, local configs, npm
  tokens, private endpoints, dashboard exports, package artifacts, or
  unintended release files are included.
* Use the maintainer npm account `500tpig` for the first publish.
* Prefer Trusted Publishing/OIDC for future automated publishes over a
  long-lived `NPM_TOKEN`; use maintainer-approved npm authentication and 2FA.
  Never commit npm tokens or generated credential files.
* Remember that npm packaging does not sign or notarize macOS binaries. Keep
  Gatekeeper/quarantine caveats separate from npm availability.

After publishing, verify registry metadata and run an install smoke:

```bash
npm view sourcemux name version dist-tags bin optionalDependencies --json
npm view @500tpig/sourcemux-darwin-arm64 name version dist-tags os cpu --json
npm view @500tpig/sourcemux-darwin-x64 name version dist-tags os cpu --json
npm view @500tpig/sourcemux-linux-arm64 name version dist-tags os cpu --json
npm view @500tpig/sourcemux-linux-x64 name version dist-tags os cpu --json
npm view @500tpig/sourcemux-win32-x64 name version dist-tags os cpu --json
npm install --prefix "$(mktemp -d)" sourcemux@<version>
npm exec --package sourcemux@<version> -- sourcemux version
```

Check that the root package `optionalDependencies` all point at `<version>` and
that every platform package has the matching `version`, `os`, and `cpu`
metadata. If the install smoke runs on one of the supported platforms, confirm
that `sourcemux version` prints the same release version.

### 4. Install smoke

Run at least one binary install smoke path. For `go install`, isolate `HOME`
and `GOBIN` so the smoke does not write to the maintainer's normal Go bin or
agent config directories:

```bash
SMOKE_HOME="$(mktemp -d)"
HOME="$SMOKE_HOME" GOBIN="$SMOKE_HOME/bin" go install github.com/500tpig/sourcemux-go/cmd/sourcemux@latest
HOME="$SMOKE_HOME" PATH="$SMOKE_HOME/bin:$PATH" sourcemux version
HOME="$SMOKE_HOME" PATH="$SMOKE_HOME/bin:$PATH" sourcemux --config "$SMOKE_HOME/.config/sourcemux/sourcemux.json" setup --non-interactive \
  --api-url "https://your-grok-compatible-endpoint.example/v1" \
  --api-key "sk-your-key" \
  --model "grok-4.20-fast" \
  --json
HOME="$SMOKE_HOME" PATH="$SMOKE_HOME/bin:$PATH" sourcemux --config "$SMOKE_HOME/.config/sourcemux/sourcemux.json" doctor --json
```

Where available, also smoke package managers and then remove the package:

```bash
brew update
brew tap 500tpig/tap
brew install --cask sourcemux
sourcemux version
brew uninstall --cask sourcemux
```

```powershell
scoop bucket add 500tpig https://github.com/500tpig/scoop-bucket.git
scoop update
scoop install 500tpig/sourcemux
sourcemux version
scoop uninstall sourcemux
```

### 5. Generated skill lifecycle smoke

Run generated skill lifecycle checks only inside an isolated temporary `HOME`.
Do not point these commands at the maintainer's real `~/.codex`,
`~/.config/sourcemux`, or other agent config directories.

```bash
SMOKE_HOME="$(mktemp -d)"
HOME="$SMOKE_HOME" GOBIN="$SMOKE_HOME/bin" go install github.com/500tpig/sourcemux-go/cmd/sourcemux@latest
BIN="$SMOKE_HOME/bin/sourcemux"
CONFIG="$SMOKE_HOME/.config/sourcemux/sourcemux.json"

HOME="$SMOKE_HOME" "$BIN" --config "$CONFIG" setup --non-interactive \
  --api-url "https://your-grok-compatible-endpoint.example/v1" \
  --api-key "sk-your-key" \
  --model "grok-4.20-fast" \
  --json
HOME="$SMOKE_HOME" "$BIN" --config "$CONFIG" bootstrap codex --scope user --binary "$BIN" --dry-run --json
HOME="$SMOKE_HOME" "$BIN" --config "$CONFIG" bootstrap codex --scope user --binary "$BIN" --json
HOME="$SMOKE_HOME" "$BIN" --config "$CONFIG" bootstrap status codex --scope user --config-status --binary "$BIN" --json
HOME="$SMOKE_HOME" "$BIN" --config "$CONFIG" bootstrap update codex --scope user --binary "$BIN" --json
HOME="$SMOKE_HOME" "$BIN" --config "$CONFIG" uninstall codex --scope user --dry-run --json
HOME="$SMOKE_HOME" "$BIN" --config "$CONFIG" uninstall codex --scope user --json
```

Expected lifecycle results:

* `bootstrap codex --scope user` writes only under the temporary
  `~/.codex/skills/sourcemux-routing` and records `.sourcemux-install.json`.
  The generated manifest/config path should point at `$CONFIG` inside the
  temporary home.
* `bootstrap status codex --scope user --config-status` reports
  `installed=true`, `managed=true`, and `modified=false`.
* `bootstrap update codex --scope user` refreshes or reports unchanged generated
  skill content without touching real user agent configs.
* `uninstall codex --scope user` removes the generated skill/manifest only; it
  does not delete the SourceMux binary or provider config JSON.

If you need to smoke `--write-config`, do it in a fresh temporary `HOME` and
seed only disposable agent config files there. Confirm that uninstall with
`--write-config` removes only the `sourcemux` MCP entry and preserves unrelated
entries.

### 6. Rollback and known failures

If the GitHub Release is published but Homebrew or Scoop publishing fails:

* Do not update README, QUICKSTART, or handoff docs to claim that package
  manager channel is available for the new version.
* Record the exact failed channel and version in release notes or an issue.
* Fix the satellite repository manifest or rerun the release automation from a
  clean state, then repeat the raw cask/manifest verification.

If release assets are missing or checksums do not match:

* Treat the release as failed until the artifact/checksum set is coherent.
* Prefer publishing a fixed follow-up tag over silently replacing artifacts
  after users may have downloaded them.
* If a tag/release is intentionally removed before use, make sure package
  manager manifests do not still point at it.

If local `dist/*` snapshots disagree with the public release/tap/bucket state,
trust the public artifacts for closeout. Regenerate snapshots only for local
preflight; do not edit or commit ignored `dist/*` files as release evidence.

If npm publication partially fails:

* Do not republish an already published package version; npm versions are
  immutable.
* Identify exactly which packages exist with `npm view <package>@<version>
  version` and which packages are missing.
* Prefer GitHub Actions **Re-run failed jobs** for the failed `npm-publish` job
  after confirming the GitHub Release assets are still the intended source of
  truth. Do not create a new tag or stage ad-hoc local binaries for repair.
  The npm publish steps skip packages that already have `<version>` and
  continue with missing platform packages before attempting the root package.
* If a platform package was published with the wrong binary, publish a fixed
  follow-up version and deprecate the bad package version with a clear message;
  do not replace or mutate the published tarball.
* If the root package was published before a platform package is available,
  publish the missing platform package as soon as possible, then rerun the
  npm registry verification and install smoke. If the gap is user-visible,
  record it in release notes or an issue.
