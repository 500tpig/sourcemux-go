# Contributing

Thanks for improving SourceMux.

## Development setup

```bash
git clone https://github.com/500tpig/sourcemux-go.git
cd sourcemux-go
go mod download
go test ./...
go vet ./...
go build ./...
```

## Local config

Do not commit real credentials. Use a local ignored config file:

```bash
cp configs/sourcemux.example.json sourcemux.json
chmod 600 sourcemux.json
```

Then replace placeholder values with your own provider endpoints and keys.

If you are migrating from the older `grok-search` name, you can keep an
existing `grok-search.json` by passing it explicitly with `--config`, but new
setup and docs should use `sourcemux.json`.

## Pull requests

- Keep changes focused and reproducible.
- Add or update tests for behavior changes.
- Update README or docs for user-visible behavior.
- Run `gofmt` on modified Go files.
- Run `go test ./...`, `go vet ./...`, and `go build ./...` before submitting.

## Repository hygiene

- Keep local AI workflow state out of public commits: `.trellis/`, `.agents/`,
  `.codex/`, and `.claude/` should stay developer-local.
- Do not commit local task archives, journals, OS metadata such as `.DS_Store`,
  or generated release artifacts.
- Keep docs and example configs free of personal absolute paths, private
  endpoints, and real credentials.

## Provider integrations

Tests must not call live external APIs. Use local test servers or fakes for request construction, response parsing, retries, and error handling.
