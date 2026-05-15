# Agent notes for sourcemux-go

This repository is a Go MCP server and CLI for search, fetch, research, and reasoning synthesis workflows.

## Project layout

```text
.
├── main.go
├── internal/
│   ├── cli/       # CLI command parsing and output formatting
│   ├── config/    # Single-file config loading and normalization
│   ├── engine/    # Provider clients, endpoint pools, retry helpers
│   ├── server/    # MCP server wiring
│   └── tools/     # MCP tools and composable workflows
├── configs/       # Safe example config files
├── docs/          # User and maintainer docs
└── scripts/       # Local helper scripts
```

## Development rules

- Prefer `rg` and `rg --files` for searching.
- Keep runtime config single-file based: default `./sourcemux.json`, or one explicit `--config` path.
- Do not add environment-variable config chains, hidden home config fallbacks, or legacy `endpoints.json` loading.
- Never commit real API keys, private endpoints, provider dashboard exports, or local credential files.
- Example configs must use placeholder secrets and safe example endpoints.
- Tests must not call live external APIs; use local test servers or fakes.
- When adding provider behavior, update both CLI and MCP documentation when the surface is user-visible.
- Keep synthesis-only models in `reasoningEndpoints[]`; do not put them in `grokEndpoints[]`.

## Required checks

Before finishing code changes, run:

```bash
gofmt -w <modified-go-files>
go test ./...
go vet ./...
go build ./...
```

## Useful commands

```bash
go build -o sourcemux ./cmd/sourcemux
./sourcemux cli --config ./sourcemux.json config list --json
./sourcemux cli --config ./sourcemux.json search "example query" --json
./sourcemux cli --config ./sourcemux.json research "example research task" --depth standard --json
```
