# Release

This project uses GoReleaser for tagged releases.

## One-time setup

Create the satellite repositories used by GoReleaser:

* `500tpig/homebrew-tap`
* `500tpig/scoop-bucket`

Then add repository secrets to `500tpig/grok-search-go`:

* `HOMEBREW_TAP_TOKEN` with contents write access to `500tpig/homebrew-tap`
* `SCOOP_BUCKET_TOKEN` with contents write access to `500tpig/scoop-bucket`

The default `GITHUB_TOKEN` publishes the GitHub Release in this repository, but
it cannot push to the separate tap/bucket repositories.

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

It also publishes:

* GitHub Release archives and `checksums.txt`
* Homebrew cask in `500tpig/homebrew-tap`
* Scoop manifest in `500tpig/scoop-bucket`

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
