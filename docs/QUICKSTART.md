# Quick start

This guide puts the public user flow first. Source checkout examples are in the
development section at the end.

## Public user flow

Assumption: `sourcemux` is already installed on your `PATH` from a release
asset, package manager, or:

```bash
go install github.com/500tpig/sourcemux-go/cmd/sourcemux@latest
```

Use one explicit user config file:

```text
~/.config/sourcemux/sourcemux.json
```

SourceMux does not auto-scan `~/.config/sourcemux`; every runtime command uses
that file because it is passed with `--config`.

## 1. Create user config

```bash
sourcemux --config ~/.config/sourcemux/sourcemux.json setup --non-interactive \
  --api-url "https://your-grok-compatible-endpoint.example/v1" \
  --api-key "sk-your-key" \
  --model "grok-4.20-fast" \
  --json
```

For a native xAI Responses API endpoint with both web and X search tools:

```bash
sourcemux --config ~/.config/sourcemux/sourcemux.json setup --non-interactive \
  --api-url "https://api.x.ai/v1" \
  --api-key "sk-your-xai-key" \
  --model "grok-4.20-fast" \
  --api-type responses \
  --send-search-flag \
  --response-tools web_search,x_search \
  --json
```

Never commit a real `sourcemux.json`.

## 2. Verify config

```bash
sourcemux --config ~/.config/sourcemux/sourcemux.json config path
sourcemux --config ~/.config/sourcemux/sourcemux.json config list --json
sourcemux --config ~/.config/sourcemux/sourcemux.json doctor --json
```

`config list` masks secrets and does not probe the network. `doctor` is
local-only by default; use `doctor --probe` or `probe` only when you explicitly
want live provider checks.

## 3. Run CLI commands

```bash
sourcemux --config ~/.config/sourcemux/sourcemux.json search "latest Go release notes" --json
sourcemux --config ~/.config/sourcemux/sourcemux.json search "latest community feedback on GPT-5.4 Codex" --platform Twitter --json
sourcemux --config ~/.config/sourcemux/sourcemux.json docs-search "next.js middleware auth" --json
sourcemux --config ~/.config/sourcemux/sourcemux.json fetch "https://example.com" --json
sourcemux --config ~/.config/sourcemux/sourcemux.json plan "Evaluate current Go module proxy behavior" --depth standard
sourcemux --config ~/.config/sourcemux/sourcemux.json research "Evaluate the current status of Go modules" --depth standard --profile auto --json
```

Use `search --platform Twitter` for freshness/community discovery, `docs-search`
or direct `exa-search` for source-first docs/API discovery, and `fetch` to
verify key URLs before source-critical claims. `plan` is offline and
deterministic. `research` defaults to `profile=auto`, so configured heavy search
is used for research/deep/current/comparison/high-risk flows while fallback
providers remain available.

## 4. Install agent routing skill

User-scope bootstrap defaults the generated skill's config path to
`~/.config/sourcemux/sourcemux.json`. Explicit `--config` still wins.

```bash
sourcemux bootstrap list-agents
sourcemux bootstrap codex claude-code --scope user --dry-run
sourcemux bootstrap codex --scope user
sourcemux bootstrap update codex --scope user
sourcemux bootstrap status --scope user --config-status
```

Without `--write-config`, generated skills are CLI-first and do not tell agents
to call SourceMux MCP tools. Use `--write-config` only when you want SourceMux
to safely merge supported Codex/Gemini/OpenCode MCP client config files:

```bash
sourcemux bootstrap codex --scope user --write-config --dry-run --json
```

The installer never writes provider API keys into agent config; it only passes
the selected config file path to the SourceMux binary. If status reports a
missing/stale binary or config path, reinstall or update the skill instead of
guessing a replacement path.

## 5. Add MCP server manually

Use absolute paths so the MCP client's working directory does not matter:

```json
{
  "type": "stdio",
  "command": "/absolute/path/to/sourcemux",
  "args": ["--config", "/home/you/.config/sourcemux/sourcemux.json"]
}
```

After registration, call `get_config_info` from the MCP client to confirm the
server sees the expected config.

## Development from source

Use this section when you are developing SourceMux from a checkout. Project
scope defaults generated skills to `./sourcemux.json`; it is intentionally
separate from the public user flow.

```bash
git clone https://github.com/500tpig/sourcemux-go.git
cd sourcemux-go
go build -o sourcemux .
./sourcemux --config ./sourcemux.json setup --non-interactive \
  --api-url "https://your-grok-compatible-endpoint.example/v1" \
  --api-key "sk-your-key" \
  --model "grok-4.20-fast" \
  --json
./sourcemux --config ./sourcemux.json doctor --json
./sourcemux bootstrap codex --scope project --binary "$(pwd)/sourcemux"
./sourcemux bootstrap status --scope project --config-status
```

If you are migrating from `grok-search`, use `sourcemux` for new commands and
rename `grok-search.json` to `sourcemux.json` or pass the old file explicitly
with `--config`.
