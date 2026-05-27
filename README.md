# SourceMux

[![CI](https://github.com/500tpig/sourcemux-go/actions/workflows/ci.yml/badge.svg)](https://github.com/500tpig/sourcemux-go/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/github/go-mod/go-version/500tpig/sourcemux-go)](go.mod)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

[中文](#中文) | [English](#english)

> **SourceMux is a single-binary CLI + stdio MCP agent research router.**
> It gives agents fast/default search for one-hop work, `profile=auto` heavy
> Grok research when configured, and Jina-first URL fetch with provider
> fallbacks.

## 中文

SourceMux 是一个面向 AI Agent、MCP 客户端和命令行自动化的单二进制 CLI + stdio MCP 研究路由器。它把 Grok / OpenAI-compatible endpoint pool、TinyFish、Exa、Tavily、Jina 等能力接到统一 fallback route：普通查询走快速默认搜索，`research` / `smart-answer` 默认用 `profile=auto` 在适合时切到已配置的 heavy / multi-agent Grok 搜索，URL 抓取先走轻量零 key 的 Jina Reader，再按需 fallback 到 TinyFish / Exa / Tavily。

仓库默认只保存安全示例配置。真实 API key 只应该放在本地 `sourcemux.json`，或用 `--config /path/to/sourcemux.json` 显式指定的本地配置文件里。不要提交真实密钥、私有 provider endpoint 或 provider dashboard 导出文件。

### 适合什么时候用

- 你希望 AI 助手联网搜索当前信息，但要保留可复现的 CLI 命令和 JSON 输出。
- 你想抓取一个 URL 正文，或把网页内容交给后续 Agent / 脚本处理。
- 你想查官方文档、API、SDK、框架用法，并通过 Exa 做文档 / web 搜索。
- 你想生成一个可审计的轻量 research pack：先规划搜索，再收集来源，最后抓取关键页面。
- 你想把同一套搜索能力接入 Codex、Claude Code、Cherry Studio 等 MCP 客户端。

### 默认路由

| 能力 / 命令 | 默认路线 |
| --- | --- |
| `web_search` / `sourcemux search` | Grok endpoint pool -> TinyFish Search -> Exa Search -> Tavily Search |
| `web_fetch` / `sourcemux fetch` | Jina Reader -> TinyFish Fetch -> Exa Contents -> Tavily Extract |
| `docs_search` / `sourcemux docs-search` | Exa docs/web search fallback |
| `research_run` / `sourcemux research` | 规划 query -> 搜索 -> 收集来源 -> 排序 URL -> 抓取高价值页面（默认 `--profile auto`） |
| `smart_answer` / `sourcemux smart-answer` | 先跑 bounded research（默认 `--profile auto`），再交给配置好的 reasoning endpoint 综合回答 |

### 为什么不是只用 Jina 或普通搜索

- Jina Reader 是轻量、零 key、fetch-first 的 URL 正文提取入口；它不是搜索、文档检索、heavy Grok 或最终综合能力的上限。
- 普通搜索适合一次性找结果；SourceMux 额外提供 agent 友好的 route、fallback、`get_sources`、fetch 验证、bounded research pack 和可复现 JSON。
- 对复杂、当前、对比或高风险问题，`research` / `smart-answer` 默认 `profile=auto`，可以在配置了 heavy Grok profile 时自动用更强搜索，同时仍保留 fallback。

### 安装

当前可直接给别人用的方式是从包含 SourceMux 改名后的源码构建。Homebrew / Scoop / `go install ...@latest` 需要等第一次 SourceMux release 发布后再作为稳定安装方式使用。

源码构建：

```bash
git clone https://github.com/500tpig/sourcemux-go.git
cd sourcemux-go
go build -o sourcemux .
```

发布 tag、GitHub Release 和包管理器 manifest 都实际存在后，预计可用的发布通道命令：

```bash
brew tap 500tpig/tap
brew install --cask sourcemux
```

不要直接用 `brew install sourcemux`，除非 SourceMux 之后也进入了 Homebrew
core；本项目发布路径是上面的 tap/cask。

```powershell
scoop bucket add 500tpig https://github.com/500tpig/scoop-bucket.git
scoop install 500tpig/sourcemux
```

```bash
go install github.com/500tpig/sourcemux-go/cmd/sourcemux@latest
```

确认安装：

```bash
sourcemux version
sourcemux config path
```

### 快速开始

生成本地配置：

```bash
sourcemux setup --non-interactive \
  --api-url "https://your-grok-compatible-endpoint.example/v1" \
  --api-key "sk-your-key" \
  --model "grok-4.20-fast" \
  --json
```

检查配置，输出会遮蔽 key：

```bash
sourcemux config list --json
sourcemux doctor --json
```

跑一次搜索：

```bash
sourcemux search "今天 Go 生态有哪些重要更新？" --json
```

显式跑较慢的 heavy Grok 搜索时，用户面向的研究应保留 fallback；只有诊断 profile 本身是否可返回时才禁用 fallback：

```bash
sourcemux search "复杂搜索问题" --profile heavy --fallback-after 60s --timeout 180s --json
sourcemux search "ping" --profile heavy --grok-pool-timeout 0 --no-fallback --timeout 120s --json
```

抓取网页正文：

```bash
sourcemux fetch "https://example.com" --json
```

查库 / 框架 / SDK 文档：

```bash
sourcemux docs-search "next.js middleware auth" --json
```

生成研究包：

```bash
sourcemux research "Evaluate the current status of Go modules" --depth deep --profile auto --json
```

### 配置文件

SourceMux 只读取一个显式 JSON 配置文件：

- 默认：`./sourcemux.json`
- 显式：`sourcemux --config /path/to/sourcemux.json <command>`
- 兼容旧写法：`sourcemux cli --config /path/to/sourcemux.json <command>`

它不会读取环境变量配置链、`~/.config/sourcemux/*` 或旧的 `endpoints.json`。如果你已有旧版 `grok-search.json`，可以改名：

```bash
mv grok-search.json sourcemux.json
```

也可以继续显式指定旧文件：

```bash
sourcemux --config ./grok-search.json config list --json
```

最小配置示例：

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

安全示例文件：

- [`configs/sourcemux.example.json`](configs/sourcemux.example.json)
- [`configs/sourcemux.reasoning.example.json`](configs/sourcemux.reasoning.example.json)

### 常用命令

| 命令 | 用途 |
| --- | --- |
| `sourcemux search <query>` | 按 fallback route 做一次网页搜索 |
| `sourcemux docs-search <query>` | 文档搜索；使用 Exa docs/web search fallback |
| `sourcemux fetch <url>` | 抓取一个 URL 的正文 |
| `sourcemux map <url>` | 用 Tavily 发现站点 URL |
| `sourcemux crawl <url>` | 用 Tavily 抓取站点内容 |
| `sourcemux research <query>` | 生成 bounded research pack |
| `sourcemux smart-answer <query>` | research 后交给 reasoning endpoint 综合 |
| `sourcemux config path/files/list` | 查看当前配置路径和遮蔽后的有效配置 |
| `sourcemux setup` | 生成本地配置，不必手写 JSON |
| `sourcemux doctor` / `probe` | 本地配置检查 / 显式 live probe |
| `sourcemux bootstrap list-agents/status` | 安装或检查 AI Agent 路由 skill 与 MCP 配置片段 |

### MCP 接入

通用 stdio MCP server 配置：

```json
{
  "type": "stdio",
  "command": "/absolute/path/to/sourcemux",
  "args": ["--config", "/absolute/path/to/sourcemux.json"]
}
```

Claude Code 示例：

```bash
claude mcp add-json sourcemux '{
  "type": "stdio",
  "command": "/absolute/path/to/sourcemux",
  "args": ["--config", "/absolute/path/to/sourcemux.json"]
}'
```

也可以先用内置安装器生成 CLI-first 的 `sourcemux-routing` skill；只有显式传
`--write-config` 或选择 `mcp-json` / `stdio` 目标时才输出 MCP 配置指导：

```bash
sourcemux bootstrap list-agents
sourcemux bootstrap codex claude-code --scope project --config ./sourcemux.json --dry-run
sourcemux bootstrap codex --scope project --binary "$(pwd)/sourcemux" --config ./sourcemux.json
sourcemux bootstrap codex --write-config --scope project --binary "$(pwd)/sourcemux" --config ./sourcemux.json
sourcemux bootstrap update codex --write-config --scope project --binary "$(pwd)/sourcemux" --config ./sourcemux.json
sourcemux bootstrap status --config-status
```

未传 `--write-config` 时，生成的 skill 会要求使用 CLI，并在每个 CLI 示例中带上
安装时配置的 `--config` 路径。传 `--write-config` 后，支持安全写入的目标会生成
MCP-aware skill，并输出更具体的官方 MCP 接入方式：Codex 的
`codex mcp add` / `config.toml`、Claude Code 的
`claude mcp add --transport stdio`、Gemini CLI 的 `gemini mcp add` /
`settings.json`，以及 OpenCode 的 `opencode.json` 配置片段。
`--write-config` 会为 Codex（`.codex/config.toml` / `~/.codex/config.toml`）、
Gemini（`.gemini/settings.json` / `~/.gemini/settings.json`）和 OpenCode
（`opencode.json` / `~/.config/opencode/opencode.json`）安全合并
`sourcemux` MCP 条目，不调用外部 agent CLI，也不会写入 provider API key。
修改已有配置前会创建带时间戳的备份；`--dry-run --json`
会显示将创建备份的原因和路径意图，但不会写文件。当前写入器会保留配置语义、
无关 key 和无关 MCP 条目，但可能重新序列化 Codex TOML、Gemini JSON 和
OpenCode JSONC；注释和原始排版不保证保留，备份文件是回滚路径。
`sourcemux uninstall <target> --write-config` 只删除 `sourcemux` 条目，
不删除整个配置文件。
生成的 skill 目录会带 `.sourcemux-install.json` manifest；`bootstrap update`
会自动刷新未被用户修改的旧生成 skill。`uninstall` 默认只删除 manifest hash
仍匹配的生成文件；如果用户改过生成 skill，或旧安装没有 manifest，可传
`--force` 先备份再移除。

MCP 侧常用工具：

| 工具 | 用途 |
| --- | --- |
| `web_search` | 紧凑搜索摘要，带 provider fallback 和来源提取 |
| `docs_search` | 文档搜索；使用 Exa docs/web search fallback |
| `get_sources` | 返回上一次 `web_search` 的 URL 列表 |
| `web_fetch` | 抓取网页正文摘要 |
| `web_map` / `web_crawl` | 站点 URL 发现 / 站点抓取 |
| `research_run` | 返回紧凑 research pack |
| `smart_answer` | research 后调用 reasoning endpoint 综合 |
| `get_config_info` | 配置诊断和 Grok `/models` probe |

### 从 grok-search 迁移

项目已改名为 SourceMux：

- GitHub 仓库目标名：`500tpig/sourcemux-go`
- 主命令：`sourcemux`
- 默认配置：`sourcemux.json`

旧的 `cmd/grok-search` 仍保留一个迁移窗口，GoReleaser 也会把 `grok-search` 兼容 binary 一起打包。新文档和新安装请使用 `sourcemux`。

已有本地 clone 在 GitHub 仓库改名后运行：

```bash
git remote set-url origin https://github.com/500tpig/sourcemux-go.git
```

### 更多文档

- [`docs/QUICKSTART.md`](docs/QUICKSTART.md) — 更完整的快速开始。
- [`docs/AI_USAGE.md`](docs/AI_USAGE.md) — AI Agent / MCP / CLI 使用建议。
- [`docs/MIGRATION.md`](docs/MIGRATION.md) — 改名与配置迁移。
- [`docs/RELEASE.md`](docs/RELEASE.md) — 发布、Homebrew、Scoop 和 GoReleaser。

## English

SourceMux is a single-binary CLI + stdio MCP agent research router for search,
fetch, docs lookup, bounded research, and reasoning synthesis. It gives agents
fast/default search for one-hop work, `profile=auto` heavy Grok research when
configured, and Jina-first URL fetch with provider fallbacks.

The default routing is:

- `web_search` / `sourcemux search`: Grok endpoint pool -> TinyFish Search -> Exa Search -> Tavily Search
- `web_fetch` / `sourcemux fetch`: Jina Reader -> TinyFish Fetch -> Exa Contents -> Tavily Extract
- `docs_search` / `sourcemux docs-search`: Exa docs/web search fallback
- `research_run` / `sourcemux research`: plan queries -> search -> collect sources -> rank URLs -> fetch top pages (defaults to `--profile auto`)
- `smart_answer` / `sourcemux smart-answer`: run bounded research (defaults to `--profile auto`), then synthesize the final answer with a configured OpenAI-compatible reasoning endpoint

Why not just Jina or simple search?

- Jina Reader is a lightweight, zero-key, fetch-first URL extraction provider.
  It is the first fetch attempt, not the ceiling for search, docs discovery,
  heavy Grok search, or synthesis.
- Simple web search returns candidate results. SourceMux adds agent-oriented
  routing, fallback, `get_sources`, fetch verification, bounded research packs,
  and reproducible JSON.
- For complex, current, comparative, or high-risk work, `research` and
  `smart-answer` default to `profile=auto`, so configured heavy Grok profiles
  can be used while fallback remains available.

## Features

- Single Go binary for CLI and stdio MCP server modes.
- MCP text responses stay compact; CLI text/JSON remain the canonical full-output surfaces.
- Single explicit JSON config file: `./sourcemux.json` by default, or `--config /path/to/sourcemux.json`.
- Grok/OpenAI-compatible endpoint pool with priority fallback.
- Optional TinyFish, Exa, Tavily, and Jina integrations.
- Source caching via `get_sources` for MCP workflows.
- Bounded research packs for reproducible downstream reasoning.
- Separate `reasoningEndpoints[]` for synthesis models such as DeepSeek Flash/Pro.

## Install

The currently shareable install path is to build from a source checkout that includes the SourceMux rename. Homebrew, Scoop, and `go install ...@latest` become stable install paths only after a tagged SourceMux release and matching package manifests are published.

Build from source:

```bash
git clone https://github.com/500tpig/sourcemux-go.git
cd sourcemux-go
go build -o sourcemux .
```

Expected release-channel commands after the tag, GitHub Release, and package
manifests exist:

```bash
brew tap 500tpig/tap
brew install --cask sourcemux
```

Do not use plain `brew install sourcemux` unless SourceMux is later accepted
into Homebrew core; this project publishes through the tap/cask path above.

```powershell
scoop bucket add 500tpig https://github.com/500tpig/scoop-bucket.git
scoop install 500tpig/sourcemux
```

```bash
go install github.com/500tpig/sourcemux-go/cmd/sourcemux@latest
```

Compatibility note: the repository still keeps `cmd/grok-search` as a legacy
command entrypoint for one migration window. New installs and docs should use
`sourcemux`.

Make sure Go's bin directory is in your `PATH`:

```bash
export PATH="$PATH:$(go env GOPATH)/bin"
```

Verify the command:

```bash
sourcemux version
sourcemux config path
```

Or build from source:

```bash
git clone https://github.com/500tpig/sourcemux-go.git
cd sourcemux-go
go build -o sourcemux .
```

## Quick start

The examples below assume `sourcemux` is installed on your `PATH`. If you built from source, use `./sourcemux` instead.

1. Create a local config. The generated file may contain secrets and is ignored by Git.

```bash
sourcemux setup --non-interactive \
  --api-url "https://your-grok-compatible-endpoint.example/v1" \
  --api-key "sk-your-key" \
  --model "grok-4.20-fast" \
  --json
```

2. Inspect the active config without printing secrets.

```bash
sourcemux config list --json
```

3. Run a search.

```bash
sourcemux search "What changed in the latest Go release?" --json
```

More detailed setup examples are in [`docs/QUICKSTART.md`](docs/QUICKSTART.md). Safe example config files are in [`configs/`](configs/).
AI agent integration guidance is in [`docs/AI_USAGE.md`](docs/AI_USAGE.md).
Uninstall and migration guidance is in [`docs/UNINSTALL.md`](docs/UNINSTALL.md).
Release automation notes are in [`docs/RELEASE.md`](docs/RELEASE.md).

## Configuration

The runtime reads exactly one config file:

- Default: `./sourcemux.json`
- Explicit: `sourcemux --config /path/to/sourcemux.json <command>`
- Compatibility form: `sourcemux cli --config /path/to/sourcemux.json <command>`

It does not read environment-variable config chains, `~/.config/sourcemux/*`, or legacy `endpoints.json` files.
If you already have `grok-search.json`, rename it to `sourcemux.json` or pass it explicitly with `--config`.

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
| `grokEndpoints[].enabled` | No | Defaults to `true`. Set `false` to keep an endpoint in config without using it. |
| `grokEndpoints[].profile` | No | Defaults to `default`. Plain `search` uses `default`; `research`/`smart-answer` default to `auto`, which uses `heavy` for research/deep/current/comparison/high-risk flows when configured. |
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
| `grokPoolTimeoutSec` | No | Default overall Grok pool wall-clock budget in seconds. Override per search with `--grok-pool-timeout`; `0` disables the pool cap. |

Heavy Grok search models such as `grok-4.20-multi-agent-xhigh` should not be
first in the default search pool. If they are used for search, put them in
`grokEndpoints[]` with `"profile": "heavy"`; `reasoningEndpoints[]` alone is
only for final synthesis and is not used by `web_search` / `research`.

```bash
sourcemux research "complex current topic" --depth deep --profile auto --json
sourcemux search "complex current topic" --profile heavy --fallback-after 60s --timeout 180s --json
sourcemux search "ping" --profile heavy --grok-pool-timeout 0 --no-fallback --timeout 120s --json
```

Use `--no-fallback` only when you need to diagnose whether the selected Grok
profile itself can return. Do not use it for user-facing research/search; it
disables TinyFish/Exa/Tavily fallback results.

See:

- [`configs/sourcemux.example.json`](configs/sourcemux.example.json)
- [`configs/sourcemux.reasoning.example.json`](configs/sourcemux.reasoning.example.json)

## CLI usage

```bash
./sourcemux config path
./sourcemux config files --json
./sourcemux config list --json
./sourcemux doctor --json

./sourcemux search "latest Go release notes" --json
./sourcemux fetch "https://example.com" --json
./sourcemux plan "Evaluate a new open-source project" --depth deep
./sourcemux research "Evaluate a new open-source project" \
  --depth deep --profile auto --domain github.com --max-fetches 6 --json
./sourcemux smart-answer "Should I use project X?" \
  --depth standard --profile auto --reasoning-endpoint deepseek-flash --json
```

Main subcommands:

| Command | Purpose |
| --- | --- |
| `search <query>` | One search through the fallback route. |
| `docs-search <query>` | Documentation search through Exa docs/web search fallback. |
| `fetch <url>` | Fetch one URL through the fallback route. |
| `exa-search <query>` | Direct advanced Exa Search call. |
| `exa-contents <url>` | Direct advanced Exa Contents call. |
| `map <url>` | Tavily URL discovery. |
| `crawl <url>` | Tavily site crawl with extracted content. |
| `plan <query>` | Offline search plan, no network calls. |
| `research <query>` | Bounded multi-step research pack (defaults to `--profile auto`). |
| `smart-answer <query>` | Research pack plus reasoning endpoint synthesis (passes `--profile` into research; default `auto`). |
| `config path/files/list` | Inspect the active single config file. |
| `setup` | Create a config without hand-writing JSON. |
| `doctor` / `probe` | Local config overview; opt-in live provider probes. |
| `bootstrap list-agents/status` | Install or inspect AI agent routing skills and MCP snippets. |
| `tinyfish-bench` | Local TinyFish Search / Fetch / Agent benchmark. |

Search-specific controls:

- `--profile auto|default|heavy|xhigh` selects or resolves a Grok endpoint profile. Plain `search` defaults to `default`; `research` and `smart-answer` default to `auto`.
- `--grok-pool-timeout <dur>` overrides `grokPoolTimeoutSec` for that call; `0` disables the Grok pool cap and leaves cancellation to `--timeout` or the caller.
- `--fallback-after <dur>` is an alias for `--grok-pool-timeout` when you want the selected Grok pool to give way to fallback providers after a bounded wait.
- `--no-fallback` disables TinyFish/Exa/Tavily fallback so failures from the selected Grok pool are visible; use it for diagnostics only, not user-facing research/search.

## MCP usage

Run the same binary in stdio mode. Pass `--config` unless the MCP client starts the process in the directory that contains `sourcemux.json`.

Generic MCP server entry:

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

The installer generates a CLI-first `sourcemux-routing` skill by default. It
prints MCP setup guidance only when you pass `--write-config` or explicitly
select the `mcp-json` / `stdio` targets:

```bash
sourcemux bootstrap list-agents
sourcemux bootstrap codex claude-code --scope project --config ./sourcemux.json --dry-run
sourcemux bootstrap codex --scope project --binary "$(pwd)/sourcemux" --config ./sourcemux.json
sourcemux bootstrap codex --write-config --scope project --binary "$(pwd)/sourcemux" --config ./sourcemux.json
sourcemux bootstrap update codex --write-config --scope project --binary "$(pwd)/sourcemux" --config ./sourcemux.json
sourcemux bootstrap status --config-status
```

Without `--write-config`, the generated skill tells agents to use the CLI and
every CLI example includes the installed `--config` path. With `--write-config`,
safe-writer targets get an MCP-aware skill and emit more specific official MCP
setup guidance: Codex
`codex mcp add` / `config.toml`, Claude Code
`claude mcp add --transport stdio`, Gemini CLI `gemini mcp add` /
`settings.json`, and OpenCode `opencode.json` snippets.
Pass `--write-config` to safely merge the `sourcemux` MCP entry for Codex
(`.codex/config.toml` or `~/.codex/config.toml`), Gemini
(`.gemini/settings.json` or `~/.gemini/settings.json`), and OpenCode
(`opencode.json` or `~/.config/opencode/opencode.json`). The installer does
not invoke external agent CLIs and does not write provider API keys. Before it
modifies an existing client config, it creates a timestamped backup so the
previous file can be restored; `--dry-run --json` reports the backup intent
without creating files. Current writers preserve config semantics, unrelated
keys, and unrelated MCP entries, but may reserialize/reformat Codex TOML,
Gemini JSON, and OpenCode JSONC; comments and original formatting are not
guaranteed to be preserved, so backups are the rollback path. `sourcemux uninstall <target> --write-config`
removes only the `sourcemux` entry and never deletes the whole config file.
Generated skill directories include a `.sourcemux-install.json` manifest;
`bootstrap update` refreshes old generated skills that still match their manifest.
`uninstall` removes only generated files whose content still matches the
manifest hash by default; pass `--force` to back up and remove a modified or
pre-manifest generated skill.

MCP tools:

| Tool | Purpose |
| --- | --- |
| `web_search` | Compact MCP search summary with source extraction and provider fallback. |
| `docs_search` | Documentation search through Exa docs/web search fallback. |
| `get_sources` | Return URLs from a previous `web_search` session. |
| `web_fetch` | Compact MCP fetch excerpt with provider fallback. |
| `exa_search_advanced` | Direct Exa Search advanced options. |
| `exa_contents_advanced` | Direct Exa Contents advanced options. |
| `web_map` | Discover site URLs with Tavily Map. |
| `web_crawl` | Crawl a site and extract page content with Tavily Crawl. |
| `search_planning` | Build a staged search plan before research. |
| `research_run` | Run the bounded research workflow and return a compact MCP pack (`profile` optional; omitted means `auto`). |
| `smart_answer` | Research first, then synthesize with `reasoningEndpoints` (`profile` controls the research phase; omitted means `auto`). |
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
      "profile": "default",
      "apiType": "responses",
      "sendSearchFlag": true,
      "responseTools": ["web_search", "x_search"]
    }
  ]
}
```

Leave `responseTools` empty to keep the backward-compatible `web_search` default. Set `sendSearchFlag` to `false` for proxies that auto-search or reject tool flags.

For heavy multi-agent search, keep the endpoint out of the default pool and
configure it in `grokEndpoints[]` with `profile: "heavy"`. Agent/research
flows can then use `--profile auto`; explicit direct heavy searches can force
`--profile heavy`:

```json
{
  "grokEndpoints": [
    {
      "name": "grok-multi-agent-xhigh",
      "baseURL": "https://your-grok-compatible-endpoint.example/v1",
      "apiKey": "sk-your-grok-key",
      "model": "grok-4.20-multi-agent-xhigh",
      "profile": "heavy",
      "sendSearchFlag": false
    }
  ]
}
```

```bash
sourcemux research "complex current topic" --depth deep --profile auto --json
sourcemux search "complex current topic" --profile heavy --fallback-after 60s --timeout 180s --json
sourcemux search "ping" --profile heavy --grok-pool-timeout 0 --no-fallback --timeout 120s --json
```

Example:

```json
{
  "grokEndpoints": [
    {
      "name": "sourcemux",
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

Do not commit `sourcemux.json`, API keys, provider dashboard exports, or local credential files. See [`SECURITY.md`](SECURITY.md) for vulnerability reporting and secret-handling guidance.

中文提醒：发布前请确认 `git status --ignored --short sourcemux.json` 显示为 ignored，且 `git ls-files --error-unmatch sourcemux.json` 没有输出。`config list` 会遮蔽密钥；`doctor` 默认只做本地结构检查，`doctor --probe` / `probe` 才会访问配置的 provider，请只在可信配置下运行。

## License

MIT. See [`LICENSE`](LICENSE).
