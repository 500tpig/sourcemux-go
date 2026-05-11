# Handoff and operations

This document is for maintainers deploying the binary or connecting MCP clients. Do not put real API keys, private endpoints, or production config files in Git.

## Runtime modes

- CLI mode: `grok-search cli <command>`
- MCP server mode: `grok-search` over stdio

Both modes use the same engine code and the same single config file.

## Important paths

Local development:

```text
./grok-search
./grok-search.json
```

Server deployment example:

```text
/usr/local/bin/grok-search
/etc/grok-search/grok-search.json
```

Use an explicit `--config` path whenever the process working directory is uncertain.

## Build and verify

```bash
go test ./...
go vet ./...
go build -o grok-search .
```

## Config

The application reads one JSON file only:

- Default: `./grok-search.json`
- Explicit: `--config /path/to/grok-search.json`

It does not read environment-variable config chains, hidden home-directory config, or legacy `endpoints.json` files.

Start from one of the safe examples:

```bash
cp configs/grok-search.example.json grok-search.json
chmod 600 grok-search.json
```

For `smart_answer`, use:

```bash
cp configs/grok-search.reasoning.example.json grok-search.json
chmod 600 grok-search.json
```

Then replace placeholder endpoints and keys.

For native xAI Responses API endpoints, `grokEndpoints[].responseTools` can opt
into built-in tools such as `web_search` and `x_search` when
`sendSearchFlag` is true. Leave it empty for the backward-compatible
`web_search` default.

## Recommended production permissions

```bash
sudo chown root:root /etc/grok-search/grok-search.json
sudo chmod 600 /etc/grok-search/grok-search.json
sudo chmod 755 /usr/local/bin/grok-search
```

## MCP registration

Generic stdio server entry:

```json
{
  "type": "stdio",
  "command": "/absolute/path/to/grok-search",
  "args": ["--config", "/absolute/path/to/grok-search.json"]
}
```

Claude Code example:

```bash
claude mcp add-json grok-search '{
  "type": "stdio",
  "command": "/absolute/path/to/grok-search",
  "args": ["--config", "/absolute/path/to/grok-search.json"]
}'
```

## Acceptance checks

CLI:

```bash
./grok-search cli --config /path/to/grok-search.json config list --json
./grok-search cli --config /path/to/grok-search.json doctor --json
./grok-search cli --config /path/to/grok-search.json search "What is today's date?" --json
./grok-search cli --config /path/to/grok-search.json fetch "https://example.com" --json
```

MCP:

1. Call `get_config_info`.
2. Call `web_search` with a simple current-information query.
3. Call `get_sources` with the returned `session_id`.
4. Call `web_fetch` on one returned URL or `https://example.com`.
5. If Tavily is configured, call `web_map` and `web_crawl`.
6. Call `research_run` for a bounded research pack.
7. If `reasoningEndpoints[]` is configured, call `smart_answer`.

Expected behavior:

- Secrets are masked in config output.
- Search responses include an engine label.
- Fetch responses include a source label.
- Research output includes executed searches, source summary, fetched page summary, high-signal sources, confirmed facts, likely inferences, and open questions.
- `smart_answer` includes endpoint/model metadata and high-signal URLs.

## Troubleshooting

### `config file not found`

Check the active path:

```bash
./grok-search cli --config /path/to/grok-search.json config path
```

Create the file with `setup` or copy an example config.

### `no Grok endpoints configured`

The active config has no search-capable endpoint. Check:

```bash
./grok-search cli --config /path/to/grok-search.json config list --json
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

Set `grokPoolTimeoutSec`, for example:

```json
{
  "grokPoolTimeoutSec": 45
}
```

This bounds the total wall-clock time spent across the Grok endpoint pool.
