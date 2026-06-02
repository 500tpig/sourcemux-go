# sourcemux npm wrapper

This package is the planned npm root wrapper for the SourceMux Go CLI. It
exposes:

```json
{
  "bin": {
    "sourcemux": "./bin/sourcemux.js"
  }
}
```

At runtime the JS launcher selects a platform optional dependency package,
resolves its native `sourcemux` binary, and spawns it with inherited stdio.

This package is not published yet. The checked-in manifest is `private` to
avoid accidental npm publication before an approved release process exists.
