# Quick start

This guide starts from a fresh clone and avoids any local-only paths.

## 1. Build

```bash
git clone https://github.com/500tpig/grok-search-go.git
cd grok-search-go
go build -o grok-search .
```

Or install the `grok-search` command directly:

```bash
go install github.com/500tpig/grok-search-go/cmd/grok-search@latest
```

## 2. Create config

The recommended path is the setup command:

```bash
./grok-search cli setup --non-interactive \
  --api-url "https://your-grok-compatible-endpoint.example/v1" \
  --api-key "sk-your-key" \
  --model "grok-4.20-fast" \
  --json
```

For a native xAI Responses API endpoint with both web and X search tools:

```bash
./grok-search cli setup --non-interactive \
  --api-url "https://api.x.ai/v1" \
  --api-key "sk-your-xai-key" \
  --model "grok-4.20-fast" \
  --api-type responses \
  --send-search-flag \
  --response-tools web_search,x_search \
  --json
```

Or start from an example:

```bash
cp configs/grok-search.example.json grok-search.json
chmod 600 grok-search.json
```

Then edit placeholders. Never commit `grok-search.json`.

## 3. Verify config

```bash
./grok-search cli config path
./grok-search cli config list --json
./grok-search cli doctor --json
```

`config list` masks secrets and does not probe the network. `doctor` probes configured Grok endpoints.

## 4. Run CLI commands

```bash
./grok-search cli search "latest Go release notes" --json
./grok-search cli fetch "https://example.com" --json
./grok-search cli research "Evaluate the current status of Go modules" --depth standard --json
```

## 5. Add MCP server

Use absolute paths so the MCP client's working directory does not matter:

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

After registration, call `get_config_info` from the MCP client to confirm the server sees the expected config.
