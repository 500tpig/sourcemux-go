# Quick start

This guide starts from a fresh clone and avoids any local-only paths.

## 1. Install or build

The currently shareable path is to build from a source checkout that includes
the SourceMux rename. Homebrew, Scoop, GitHub Releases, and `go install
...@latest` become stable install paths after the first SourceMux release is
published.

Build from source:

```bash
git clone https://github.com/500tpig/sourcemux-go.git
cd sourcemux-go
go build -o sourcemux .
```

Stable install paths after release:

```bash
brew tap 500tpig/tap
brew install --cask sourcemux
```

```powershell
scoop bucket add 500tpig https://github.com/500tpig/scoop-bucket.git
scoop install 500tpig/sourcemux
```

After release, `@latest` also resolves to the published SourceMux version:

```bash
go install github.com/500tpig/sourcemux-go/cmd/sourcemux@latest
```

If you are migrating from `grok-search`, use `sourcemux` for new commands and
rename `grok-search.json` to `sourcemux.json` or pass the old file explicitly
with `--config`.

## 2. Create config

The recommended path is the setup command:

```bash
./sourcemux cli setup --non-interactive \
  --api-url "https://your-grok-compatible-endpoint.example/v1" \
  --api-key "sk-your-key" \
  --model "grok-4.20-fast" \
  --context7-key "ctx7sk-your-key" \
  --json
```

For a native xAI Responses API endpoint with both web and X search tools:

```bash
./sourcemux cli setup --non-interactive \
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
cp configs/sourcemux.example.json sourcemux.json
chmod 600 sourcemux.json
```

Then edit placeholders. Never commit `sourcemux.json`.

## 3. Verify config

```bash
./sourcemux cli config path
./sourcemux cli config list --json
./sourcemux cli doctor --json
```

`config list` masks secrets and does not probe the network. `doctor` is local-only by default; use `doctor --probe` or `probe` only when you explicitly want live provider checks.

## 4. Run CLI commands

```bash
./sourcemux cli search "latest Go release notes" --json
./sourcemux cli docs-search "middleware auth" --library-id /vercel/next.js --json
./sourcemux cli context7-library next.js "middleware auth" --json
./sourcemux cli context7-docs /vercel/next.js "middleware auth" --json
./sourcemux cli fetch "https://example.com" --json
./sourcemux cli research "Evaluate the current status of Go modules" --depth standard --json
```

Context7 is optional and specialized for library/framework/API docs. It is used only when you pass an explicit Context7 `library-id` or `library-name`; general docs/web search remains Exa-oriented.

## 5. Install agent routing skill and MCP snippets

SourceMux includes a top-level installer that writes a concise
`sourcemux-routing` skill and prints copyable MCP stdio JSON where automatic
client config is not yet verified:

```bash
./sourcemux install list-agents
./sourcemux install codex claude-code --scope project --config ./sourcemux.json --dry-run
./sourcemux install codex --scope project --binary "$(pwd)/sourcemux" --config ./sourcemux.json
./sourcemux install status
```

Use `--json` for automation and `--force` to back up and replace an existing
generated skill. The installer does not write provider API keys into agent
config; it only passes the selected config file path to the SourceMux binary.
If you run the installer through `go run`, pass `--binary` so generated agent
commands do not point at Go's temporary build artifact.

Generated skills include a `.sourcemux-install.json` manifest with a content
hash. `sourcemux uninstall <target>` removes only files that still match that
manifest; if you edited the generated skill, uninstall refuses to delete it.

For first-tier targets, the dry-run/install plan also prints the official MCP
setup command or config snippet:

* Codex: `codex mcp add ...` and `.codex/config.toml` / `~/.codex/config.toml`
* Claude Code: `claude mcp add --transport stdio --scope ...`
* Gemini CLI: `gemini mcp add --scope ...` and `.gemini/settings.json`
* OpenCode: `opencode.json` / JSONC `mcp` snippet

## 6. Add MCP server manually

Use absolute paths so the MCP client's working directory does not matter:

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

After registration, call `get_config_info` from the MCP client to confirm the server sees the expected config.
