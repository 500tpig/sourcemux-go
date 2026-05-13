# Contributing

Thanks for improving Grok Search Go.

## Development setup

```bash
git clone https://github.com/500tpig/grok-search-go.git
cd grok-search-go
go mod download
go test ./...
go vet ./...
go build ./...
```

## Local config

Do not commit real credentials. Use a local ignored config file:

```bash
cp configs/grok-search.example.json grok-search.json
chmod 600 grok-search.json
```

Then replace placeholder values with your own provider endpoints and keys.

## Pull requests

- Keep changes focused and reproducible.
- Add or update tests for behavior changes.
- Update README or docs for user-visible behavior.
- Run `gofmt` on modified Go files.
- Run `go test ./...`, `go vet ./...`, and `go build ./...` before submitting.

## Provider integrations

Tests must not call live external APIs. Use local test servers or fakes for request construction, response parsing, retries, and error handling.
