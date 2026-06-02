# sourcemux npm wrapper

This package is the npm root wrapper source for the SourceMux Go CLI. It
exposes:

```json
{
  "bin": {
    "sourcemux": "bin/sourcemux.js"
  }
}
```

At runtime the JS launcher selects a platform optional dependency package,
resolves its native `sourcemux` binary, and spawns it with inherited stdio.

The published package installs the matching platform optional dependency when
npm supports the current OS/architecture. The installed command is `sourcemux`.
