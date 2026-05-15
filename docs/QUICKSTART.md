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

Do not use plain `brew install sourcemux` unless SourceMux has also been
accepted into Homebrew core; the project release path is the tap/cask above.

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

If you previously installed SourceMux agent skills or MCP config entries, see
[`UNINSTALL.md`](UNINSTALL.md) for the cleanup and reinstall flow.

## 2. Create config

The recommended path is the setup command:

```bash
./sourcemux setup --non-interactive \
  --api-url "https://your-grok-compatible-endpoint.example/v1" \
  --api-key "sk-your-key" \
  --model "grok-4.20-fast" \
  --json
```

For a native xAI Responses API endpoint with both web and X search tools:

```bash
./sourcemux setup --non-interactive \
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
./sourcemux config path
./sourcemux config list --json
./sourcemux doctor --json
```

`config list` masks secrets and does not probe the network. `doctor` is local-only by default; use `doctor --probe` or `probe` only when you explicitly want live provider checks.

## 4. Run CLI commands

```bash
./sourcemux search "latest Go release notes" --json
./sourcemux search "latest community feedback on GPT-5.4 Codex" --platform Twitter --json
./sourcemux docs-search "next.js middleware auth" --json
./sourcemux exa-search "OpenAI Responses API reference" --type deep --json
./sourcemux exa-contents "https://example.com/docs" --subpages 3 --subpage-target api --json
./sourcemux fetch "https://example.com" --json
./sourcemux plan "Evaluate current Go module proxy behavior" --depth standard
./sourcemux research "Evaluate the current status of Go modules" --depth standard --json
```

Use `search --platform Twitter` for freshness/community discovery, `docs-search`
or direct `exa-search` for source-first docs/API discovery, and `fetch` to
verify key URLs before source-critical claims.

## 5. Install agent routing skill and MCP snippets

SourceMux includes a top-level installer that writes a concise CLI-first
`sourcemux-routing` skill with capability routing and evidence rules. It prints
MCP setup guidance only when you pass `--write-config` or explicitly select
`mcp-json` / `stdio`:

```bash
./sourcemux bootstrap list-agents
./sourcemux bootstrap codex claude-code --scope project --config ./sourcemux.json --dry-run
./sourcemux bootstrap codex --scope project --binary "$(pwd)/sourcemux" --config ./sourcemux.json
./sourcemux bootstrap codex --write-config --scope project --binary "$(pwd)/sourcemux" --config ./sourcemux.json
./sourcemux bootstrap update codex --write-config --scope project --binary "$(pwd)/sourcemux" --config ./sourcemux.json
./sourcemux bootstrap status --config-status
```

Without `--write-config`, generated CLI examples include the configured
`--config` path and do not tell agents to call MCP tools.

Use `--json` for automation and `--force` to back up and replace an existing
generated skill. The installer does not write provider API keys into agent
config; it only passes the selected config file path to the SourceMux binary.
If you run the installer through `go run`, pass `--binary` so generated agent
commands do not point at Go's temporary build artifact.

Use `--write-config` when you want SourceMux to safely merge supported local
MCP client config files instead of only printing snippets. The first safe
writers are Codex (`.codex/config.toml` or `~/.codex/config.toml`), Gemini
(`.gemini/settings.json` or `~/.gemini/settings.json`), and OpenCode
(`opencode.json` or `~/.config/opencode/opencode.json`). Existing unrelated
keys and unrelated MCP entries are preserved. Before modifying an existing
file, SourceMux creates a timestamped backup so you can restore the previous
client config; dry-runs show the backup intent but create no files. The current
writers preserve config semantics, not comments or original formatting: Codex
TOML, Gemini JSON, and OpenCode JSONC may be reserialized/reformatted, so
backups are the rollback path. `sourcemux uninstall <target> --write-config`
removes only the `sourcemux` MCP entry and never deletes the whole client
config file.

Generated skills include a `.sourcemux-install.json` manifest with a content
hash. `sourcemux bootstrap update <target>` refreshes unmodified generated skills.
`sourcemux uninstall <target>` removes only files that still match that
manifest; if you edited the generated skill, uninstall refuses to delete it
unless you pass `--force`, which backs up the modified or pre-manifest skill
first.

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
