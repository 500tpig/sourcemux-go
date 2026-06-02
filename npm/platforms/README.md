# SourceMux npm platform packages

Each subdirectory is a platform-specific optional dependency for the root
`sourcemux` npm package. These packages are source manifests only until release
prep stages a real Go binary into `bin/`.

Do not commit staged binaries. Use:

```bash
node npm/scripts/stage-platform-binary.js --target <target> --binary <path>
```

Targets:

* `darwin-x64`
* `darwin-arm64`
* `linux-x64`
* `linux-arm64`
* `win32-x64`
