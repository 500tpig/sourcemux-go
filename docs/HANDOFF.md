# Handoff and operations

This document is for maintainers deploying the binary or connecting MCP clients. Do not put real API keys, private endpoints, or production config files in Git.

## Runtime modes

- CLI mode: `sourcemux <command>`; `sourcemux cli <command>` remains a compatibility path.
- MCP server mode: `sourcemux` over stdio

Both modes use the same engine code and the same single config file.

## Important paths

Local development:

```text
./sourcemux
./sourcemux.json
```

Server deployment example:

```text
/usr/local/bin/sourcemux
/etc/sourcemux/sourcemux.json
```

Use an explicit `--config` path whenever the process working directory is uncertain.

## Build and verify

```bash
go test ./...
go vet ./...
go build -o sourcemux .
```

## Config

The application reads one JSON file only:

- Default: `./sourcemux.json`
- Explicit: `--config /path/to/sourcemux.json`

It does not read environment-variable config chains, hidden home-directory config, or legacy `endpoints.json` files.

Start from one of the safe examples:

```bash
cp configs/sourcemux.example.json sourcemux.json
chmod 600 sourcemux.json
```

For `smart_answer`, use:

```bash
cp configs/sourcemux.reasoning.example.json sourcemux.json
chmod 600 sourcemux.json
```

Then replace placeholder endpoints and keys.

For native xAI Responses API endpoints, `grokEndpoints[].responseTools` can opt
into built-in tools such as `web_search` and `x_search` when
`sendSearchFlag` is true. Leave it empty for the backward-compatible
`web_search` default.

## Recommended production permissions

```bash
sudo chown root:root /etc/sourcemux/sourcemux.json
sudo chmod 600 /etc/sourcemux/sourcemux.json
sudo chmod 755 /usr/local/bin/sourcemux
```

## MCP registration

Generic stdio server entry:

```json
{
  "type": "stdio",
  "command": "/absolute/path/to/sourcemux",
  "args": ["--config", "/absolute/path/to/sourcemux.json"]
}
```

Claude Code example:

```bash
claude mcp add-json sourcemux '{
  "type": "stdio",
  "command": "/absolute/path/to/sourcemux",
  "args": ["--config", "/absolute/path/to/sourcemux.json"]
}'
```

## Acceptance checks

CLI:

```bash
./sourcemux --config /path/to/sourcemux.json config list --json
./sourcemux --config /path/to/sourcemux.json doctor --json
./sourcemux --config /path/to/sourcemux.json search "What is today's date?" --json
./sourcemux --config /path/to/sourcemux.json fetch "https://example.com" --json
```

MCP:

1. Call `get_config_info`.
2. Call `web_search` with a simple current-information query.
3. Call `get_sources` with the returned `session_id`.
4. Call `web_fetch` on one returned URL or `https://example.com`.
5. If Tavily is configured, call `web_map` and `web_crawl`.
6. Call `research_run` for a bounded research pack; it defaults to `profile=auto` so configured heavy search is used when appropriate.
7. If `reasoningEndpoints[]` is configured, call `smart_answer`; pass `profile` if you need to force `default` or `heavy` for its research phase.

Expected behavior:

- Secrets are masked in config output.
- MCP search responses include an engine label plus a compact summary; use CLI `search --json` for full output.
- MCP fetch responses include a source label plus a compact excerpt; use CLI `fetch --json` for full output. Fetch is Jina-first because Jina Reader is lightweight and can work without a key, then falls back to TinyFish Fetch, Exa Contents, and Tavily Extract when configured. Treat Jina as the first URL extraction provider, not the whole SourceMux capability ceiling.
- MCP research output stays compact while still surfacing executed searches, source summary, fetched page summary, high-signal sources, confirmed facts, likely inferences, and open questions; use CLI `research --json` for the full pack.
- `smart_answer` includes endpoint/model metadata and high-signal URLs.

## Troubleshooting

### `config file not found`

Check the active path:

```bash
./sourcemux --config /path/to/sourcemux.json config path
```

Create the file with `setup` or copy an example config.

### `no Grok endpoints configured`

The active config has no search-capable endpoint. Check:

```bash
./sourcemux --config /path/to/sourcemux.json config list --json
```

Provider-only configs can still support some direct provider commands, but `web_search` needs a configured search route or fallback provider keys.

### `no reasoningEndpoints configured`

`smart_answer` needs `reasoningEndpoints[]`. Add a reasoning endpoint to the active config. Do not put synthesis-only models in `grokEndpoints[]`.

### `/models` returns HTML

The configured `baseURL` likely points to a web page instead of an OpenAI-compatible API root. Use the provider's API base URL. The loader appends `/v1` if omitted.

### `web_map` or `web_crawl` unavailable

Set `tavily.apiKey` and keep `tavily.enabled` true.

### Exa is not used

Set `exa.apiKey` and keep `exa.enabled` true. Exa is a fallback behind Grok and TinyFish in the default search route.

### Slow endpoint pool

Set `grokPoolTimeoutSec`, and keep heavy models out of the default profile. For
example:

```json
{
  "grokEndpoints": [
    {
      "name": "fast",
      "baseURL": "https://your-endpoint.example/v1",
      "apiKey": "sk-your-key",
      "model": "grok-4.20-fast",
      "profile": "default"
    },
    {
      "name": "xhigh",
      "baseURL": "https://your-endpoint.example/v1",
      "apiKey": "sk-your-key",
      "model": "grok-4.20-multi-agent-xhigh",
      "profile": "heavy"
    }
  ],
  "grokPoolTimeoutSec": 300
}
```

This bounds the total wall-clock time spent across the selected Grok endpoint
profile. Plain `search` stays on `default`; `research` and `smart-answer`
default to `profile=auto`, which resolves to `heavy` for research/deep/current/
comparison/high-risk flows when a heavy Grok profile exists. Multi-agent search
models (e.g. `grok-4.20-multi-agent-xhigh`) must be in `grokEndpoints[]` with
`profile: "heavy"`; placing them only in `reasoningEndpoints[]` makes them
available for final synthesis, not search.

For heavy multi-agent searches, always pass `--timeout` above the pool cap:

```bash
./sourcemux --config /path/to/sourcemux.json search "complex current topic" \
  --profile heavy --fallback-after 60s --timeout 180s --json

./sourcemux --config /path/to/sourcemux.json research "complex current topic" \
  --depth deep --profile auto --json

./sourcemux --config /path/to/sourcemux.json search "ping" \
  --profile heavy --grok-pool-timeout 0 --no-fallback --timeout 120s --json
```

`--fallback-after` and `--grok-pool-timeout` both override
`grokPoolTimeoutSec` for that search. Use `--no-fallback` only when explicitly
diagnosing whether the selected Grok profile itself can return; do not use it
for user-facing research/search because it disables TinyFish/Exa/Tavily
fallback results.
