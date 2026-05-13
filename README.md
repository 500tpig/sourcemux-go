# Grok Search Go

Grok Search Go is a CLI-first search, fetch, and research tool that also exposes the same capabilities as an MCP server for clients such as Codex, Claude Code, and Cherry Studio.

The default routing is:

- `web_search` / `cli search`: Grok endpoint pool -> TinyFish Search -> Exa Search -> Tavily Search
- `web_fetch` / `cli fetch`: Jina Reader -> TinyFish Fetch -> Exa Contents -> Tavily Extract
- `research_run` / `cli research`: plan queries -> search -> collect sources -> rank URLs -> fetch top pages
- `smart_answer` / `cli smart-answer`: run bounded research, then synthesize the final answer with a configured OpenAI-compatible reasoning endpoint

## Features

- Single Go binary for CLI and stdio MCP server modes.
- MCP text responses stay compact; CLI text/JSON remain the canonical full-output surfaces.
- Single explicit JSON config file: `./grok-search.json` by default, or `--config /path/to/grok-search.json`.
- Grok/OpenAI-compatible endpoint pool with priority fallback.
- Optional TinyFish, Exa, Tavily, and Jina integrations.
- Source caching via `get_sources` for MCP workflows.
- Bounded research packs for reproducible downstream reasoning.
- Separate `reasoningEndpoints[]` for synthesis models such as DeepSeek Flash/Pro.

## Install

```bash
go install github.com/500tpig/grok-search-go@latest
```

Or build from source:

```bash
git clone https://github.com/500tpig/grok-search-go.git
cd grok-search-go
go build -o grok-search .
```

## Quick start

1. Create a local config. The generated file may contain secrets and is ignored by Git.

```bash
./grok-search cli setup --non-interactive \
  --api-url "https://your-grok-compatible-endpoint.example/v1" \
  --api-key "sk-your-key" \
  --model "grok-4.20-fast" \
  --json
```

2. Inspect the active config without printing secrets.

```bash
./grok-search cli config list --json
```

3. Run a search.

```bash
./grok-search cli search "What changed in the latest Go release?" --json
```

More detailed setup examples are in [`docs/QUICKSTART.md`](docs/QUICKSTART.md). Safe example config files are in [`configs/`](configs/).
AI agent integration guidance is in [`docs/AI_USAGE.md`](docs/AI_USAGE.md).

## Configuration

The runtime reads exactly one config file:

- Default: `./grok-search.json`
- Explicit: `grok-search --config /path/to/grok-search.json` or `grok-search cli --config /path/to/grok-search.json ...`

It does not read environment-variable config chains, `~/.config/grok-search/*`, or legacy `endpoints.json` files.

Minimal config:

```json
{
  "grokEndpoints": [
    {
      "name": "primary",
      "baseURL": "https://your-grok-compatible-endpoint.example/v1",
      "apiKey": "sk-your-key",
      "model": "grok-4.20-fast",
      "sendSearchFlag": false
    }
  ],
  "grokPoolTimeoutSec": 45,
  "logLevel": "INFO"
}
```

Config fields:

| Field | Required | Notes |
| --- | :---: | --- |
| `grokEndpoints[]` | No | Search-capable Grok/OpenAI-compatible endpoints tried in order. |
| `grokEndpoints[].baseURL` | Yes | OpenAI-compatible API root; `/v1` is appended if omitted. |
| `grokEndpoints[].apiKey` | Yes | Bearer token. Never commit real keys. |
| `grokEndpoints[].model` | No | Defaults to `grok-3-mini`. |
| `grokEndpoints[].sendSearchFlag` | No | Usually `true` for native xAI search; often `false` for grok2api proxies. |
| `grokEndpoints[].apiType` | No | `chat` or `responses`. |
| `grokEndpoints[].responseTools` | No | Responses API built-in tools to send when `sendSearchFlag` is true. Supported: `web_search`, `x_search`. Empty defaults to `web_search`. |
| `reasoningEndpoints[]` | No | Synthesis-only OpenAI-compatible Chat Completions endpoints. Used by `smart_answer`, not `web_search`. |
| `reasoningEndpoints[].baseURL` | Yes | OpenAI-compatible API root; `/v1` is appended if omitted. |
| `reasoningEndpoints[].apiKey` | Yes | Bearer token. |
| `reasoningEndpoints[].model` | No | Defaults to `deepseek-v4-flash`. |
| `tavily` | No | Tavily Search / Extract / Map / Crawl. |
| `exa` | No | Exa Search / Contents fallback and advanced Exa tools. |
| `jina` | No | Jina Reader fetch; works without a key. |
| `tinyfish` | No | TinyFish Search / Fetch fallback with multi-key rotation. |
| `grokPoolTimeoutSec` | No | Overall Grok pool wall-clock budget in seconds. |

See:

- [`configs/grok-search.example.json`](configs/grok-search.example.json)
- [`configs/grok-search.reasoning.example.json`](configs/grok-search.reasoning.example.json)

## CLI usage

```bash
./grok-search cli config path
./grok-search cli config files --json
./grok-search cli config list --json
./grok-search cli doctor --json

./grok-search cli search "latest Go release notes" --json
./grok-search cli fetch "https://example.com" --json
./grok-search cli plan "Evaluate a new open-source project" --depth deep
./grok-search cli research "Evaluate a new open-source project" \
  --depth deep --domain github.com --max-fetches 6 --json
./grok-search cli smart-answer "Should I use project X?" \
  --depth standard --reasoning-endpoint deepseek-flash --json
```

Main subcommands:

| Command | Purpose |
| --- | --- |
| `search <query>` | One search through the fallback route. |
| `fetch <url>` | Fetch one URL through the fallback route. |
| `exa-search <query>` | Direct advanced Exa Search call. |
| `exa-contents <url>` | Direct advanced Exa Contents call. |
| `map <url>` | Tavily URL discovery. |
| `crawl <url>` | Tavily site crawl with extracted content. |
| `plan <query>` | Offline search plan, no network calls. |
| `research <query>` | Bounded multi-step research pack. |
| `smart-answer <query>` | Research pack plus reasoning endpoint synthesis. |
| `config path/files/list` | Inspect the active single config file. |
| `setup` | Create a config without hand-writing JSON. |
| `doctor` / `probe` | Config overview and `/models` probe. |
| `tinyfish-bench` | Local TinyFish Search / Fetch / Agent benchmark. |

## MCP usage

Run the same binary in stdio mode. Pass `--config` unless the MCP client starts the process in the directory that contains `grok-search.json`.

Generic MCP server entry:

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

MCP tools:

| Tool | Purpose |
| --- | --- |
| `web_search` | Compact MCP search summary with source extraction and provider fallback. |
| `get_sources` | Return URLs from a previous `web_search` session. |
| `web_fetch` | Compact MCP fetch excerpt with provider fallback. |
| `exa_search_advanced` | Direct Exa Search advanced options. |
| `exa_contents_advanced` | Direct Exa Contents advanced options. |
| `web_map` | Discover site URLs with Tavily Map. |
| `web_crawl` | Crawl a site and extract page content with Tavily Crawl. |
| `search_planning` | Build a staged search plan before research. |
| `research_run` | Run the bounded research workflow and return a compact MCP pack. |
| `smart_answer` | Research first, then synthesize with `reasoningEndpoints`. |
| `get_config_info` | Diagnostic config output and Grok `/models` probing. |

## Smart answer and reasoning endpoints

`smart_answer` deliberately separates evidence collection from synthesis:

- `grokEndpoints[]` are the "eyes" used by `web_search` and `research_run`.
- `reasoningEndpoints[]` are the "brain" used only for final synthesis.

Do not place DeepSeek or another synthesis-only model in `grokEndpoints`; a successful non-search response would short-circuit the source-first search route.

For native xAI Responses API endpoints, enable X search by opting into response tools on the search endpoint:

```json
{
  "grokEndpoints": [
    {
      "name": "xai-search",
      "baseURL": "https://api.x.ai/v1",
      "apiKey": "sk-your-xai-key",
      "model": "grok-4.20-fast",
      "apiType": "responses",
      "sendSearchFlag": true,
      "responseTools": ["web_search", "x_search"]
    }
  ]
}
```

Leave `responseTools` empty to keep the backward-compatible `web_search` default. Set `sendSearchFlag` to `false` for proxies that auto-search or reject tool flags.

Example:

```json
{
  "grokEndpoints": [
    {
      "name": "grok-search",
      "baseURL": "https://your-grok-compatible-endpoint.example/v1",
      "apiKey": "sk-your-grok-key",
      "model": "grok-4.20-fast",
      "sendSearchFlag": false
    }
  ],
  "reasoningEndpoints": [
    {
      "name": "deepseek-flash",
      "baseURL": "https://api.deepseek.com/v1",
      "apiKey": "sk-your-deepseek-key",
      "model": "deepseek-v4-flash"
    },
    {
      "name": "deepseek-pro",
      "baseURL": "https://api.deepseek.com/v1",
      "apiKey": "sk-your-deepseek-key",
      "model": "deepseek-v4-pro"
    }
  ]
}
```

More details are in [`docs/SMART_ANSWER.md`](docs/SMART_ANSWER.md).

## Development

```bash
go test ./...
go vet ./...
go build ./...
```

The CI workflow runs the same baseline checks on pushes and pull requests to `main`.

## Security

Do not commit `grok-search.json`, API keys, provider dashboard exports, or local credential files. See [`SECURITY.md`](SECURITY.md) for vulnerability reporting and secret-handling guidance.

## License

MIT. See [`LICENSE`](LICENSE).
