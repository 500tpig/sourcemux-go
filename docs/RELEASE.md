# Release

This project uses GoReleaser for tagged releases.

## Current public channel state

The current verified public baseline is `v0.2.1` (checked 2026-06-02):

* GitHub Release: `https://github.com/500tpig/sourcemux-go/releases/tag/v0.2.1`
* Homebrew cask: `500tpig/homebrew-tap`, `Casks/sourcemux.rb`, version `0.2.1`
* Scoop manifest: `500tpig/scoop-bucket`, `sourcemux.json`, version `0.2.1`

For future releases, do not update public docs to claim Homebrew or Scoop
availability until the corresponding GitHub Release assets, raw tap cask, and
raw bucket manifest have been verified.

## One-time setup

Create the satellite repositories used by GoReleaser:

* `500tpig/homebrew-tap`
* `500tpig/scoop-bucket`

Then add repository secrets to `500tpig/sourcemux-go`:

* `HOMEBREW_TAP_TOKEN` with contents write access to `500tpig/homebrew-tap`
* `SCOOP_BUCKET_TOKEN` with contents write access to `500tpig/scoop-bucket`

The default `GITHUB_TOKEN` publishes the GitHub Release in this repository, but
it cannot push to the separate tap/bucket repositories.

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

### 3. Install smoke

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

### 4. Generated skill lifecycle smoke

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

### 5. Rollback and known failures

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
