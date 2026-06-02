# SourceMux npm wrapper scaffold

This directory contains the planned npm wrapper for the SourceMux Go CLI.
It is source-only scaffolding until an npm publication is explicitly approved
and performed.

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
* `@500tpig/sourcemux` is only a fallback root package name if the unscoped
  `sourcemux` name cannot be published later.
* The checked-in package manifests are marked `private` to prevent accidental
  publishing during scaffold development.
* Do not document `npm install -g sourcemux` or `npx sourcemux` as public
  install paths until the first approved npm publication has been verified in
  the registry.

## Local tests

Run wrapper tests without installing any dependencies:

```bash
node --test npm/package/test
```

Or from the root package directory:

```bash
npm --prefix npm/package test
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
