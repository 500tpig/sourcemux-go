# Release

This project uses GoReleaser for tagged releases.

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

Until this release exists and the tap is updated, users should build from
source. `brew install sourcemux` only works if SourceMux is later accepted into
Homebrew core; the release tap path is:

```bash
brew tap 500tpig/tap
brew install --cask sourcemux
```

## Local validation

If GoReleaser is installed:

```bash
goreleaser check
goreleaser release --snapshot --clean
```

If it is not installed, the regular quality gate is still required:

```bash
go test ./...
go vet ./...
go build ./...
```
