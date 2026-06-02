# SourceMux npm wrapper

This directory contains the npm wrapper source for the SourceMux Go CLI. The
root package `sourcemux` and the platform packages are published for `0.2.1`.

The MVP uses a root package plus platform-specific optional dependencies:

```text
npm/
  package/                 # root package: sourcemux
  platforms/
    darwin-x64/            # @500tpig/sourcemux-darwin-x64
    darwin-arm64/          # @500tpig/sourcemux-darwin-arm64
    linux-x64/             # @500tpig/sourcemux-linux-x64
    linux-arm64/           # @500tpig/sourcemux-linux-arm64
    win32-x64/             # @500tpig/sourcemux-win32-x64
  scripts/
    stage-platform-binary.js
```

The core implementation remains the Go binary from `cmd/sourcemux`. The root
package exposes only the `sourcemux` npm bin and delegates execution to the
native binary supplied by the matching optional dependency package.

## Status and naming

* Root npm package name: `sourcemux`.
* Platform package names: `@500tpig/sourcemux-<platform>-<arch>`.
* `@500tpig/sourcemux` was kept as the fallback root package name before the
  first publish. The public root package is currently `sourcemux`.
* First publication was performed with the maintainer npm account `500tpig`.
* Prefer npm Trusted Publishing/OIDC for future automated publishes instead of
  a long-lived `NPM_TOKEN`.

### Registry precheck

Live npm registry check recorded on `2026-06-02T15:41:36Z`:

* `https://registry.npmjs.org/sourcemux` returned 404 via SourceMux fetch.
* `npm view sourcemux name version dist-tags --json` returned `E404`.

Fallback root package check recorded on `2026-06-02T15:41:37Z`:

* `https://registry.npmjs.org/@500tpig%2fsourcemux` returned 404 via SourceMux
  fetch.
* `npm view @500tpig/sourcemux name version dist-tags --json` returned `E404`.

Maintainer account check recorded on `2026-06-02T15:41:37Z`:

* `npm whoami` returned `500tpig`.

Publication check recorded on `2026-06-02T16:07:07Z`:

* `sourcemux@0.2.1` returned `latest: 0.2.1`.
* All five `@500tpig/sourcemux-*` platform packages returned `latest: 0.2.1`.
* `npm install sourcemux@0.2.1` installed successfully and `sourcemux version`
  returned `sourcemux 0.2.1`.

Repeat registry checks immediately before publishing future versions.

## Local tests

Run wrapper tests without installing any dependencies:

```bash
node --test npm/package/test
```

Or from the root package directory:

```bash
npm --prefix npm/package test
```

## Dry-run package verification

Verify the root package and every platform package file list without publishing:

```bash
npm --prefix npm/package run pack:dry-run
```

This runs `npm pack --dry-run --json` for `npm/package` and every
`npm/platforms/*` package, then rejects local configs, npm tokens, package
artifacts, unexpected release files, and any path outside the expected wrapper
or platform binary file set.

During release packaging, after each native binary has been staged into the
matching platform package, require all staged binary paths to be present:

```bash
node npm/scripts/verify-pack-dry-run.js --require-staged-binaries
```

## Local platform package staging

Do not commit real native binaries. To smoke local packaging, build or extract a
SourceMux binary, then stage it into the current platform package:

```bash
go build -o /tmp/sourcemux-local ./cmd/sourcemux
node npm/scripts/stage-platform-binary.js --target darwin-arm64 --binary /tmp/sourcemux-local
```

Use the target matching the binary:

* `darwin-x64`
* `darwin-arm64`
* `linux-x64`
* `linux-arm64`
* `win32-x64`

The staging script copies the binary to `npm/platforms/<target>/bin/sourcemux`
or `sourcemux.exe`. Those paths are git-ignored.

## macOS security caveat

The npm wrapper is an install channel, not a macOS codesign or notarization
solution. Unsigned/unnotarized macOS binaries can still be blocked depending on
Gatekeeper, quarantine attributes, architecture, and local policy.
