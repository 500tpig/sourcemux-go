# grok-search-go

Go 版 Grok Search MCP Server — Grok AI 搜索（多端点池 + 自动降级）+ Jina Reader 抓取（Exa / Tavily 兜底）。

## 项目结构

```
.
├── main.go                          # 入口
├── go.mod
├── internal/
│   ├── config/config.go             # 环境变量配置 + 端点池解析
│   ├── engine/
│   │   ├── grok.go                  # Grok API client（OpenAI 兼容；含 annotations / search_sources / citations / search_results / 正则 5 级源解析）
│   │   ├── grok_pool.go             # GrokPool: 多端点优先级降级
│   │   ├── exa.go                   # Exa Search + Contents client
│   │   ├── tavily.go                # Tavily Search + Extract + Map + Crawl client
│   │   └── jina.go                  # Jina Reader client（web_fetch 主力）
│   ├── server/server.go             # MCP server 初始化 + 运行
│   └── tools/
│       ├── search.go                # web_search 工具（Grok 池 → TinyFish → Exa → Tavily Search 降级）
│       ├── fetch.go                 # web_fetch 工具（Jina → TinyFish → Exa Contents → Tavily Extract 降级）
│       ├── map.go                   # web_map 工具
│       ├── crawl.go                 # web_crawl 工具
│       ├── research.go              # research_run MCP 工具 + 组合式 research executor
│       ├── sources.go               # get_sources 工具
│       └── config_tool.go           # get_config_info 诊断工具（列出每个端点 + 探活）
```

## 开发前提

- Go 1.22+
- 依赖: github.com/mark3labs/mcp-go, github.com/google/uuid

## 快速启动

```bash
brew install go   # 或从 https://go.dev/dl/ 下载
cd /Users/johnsmith/Project/Study/grok-search-go
go mod tidy
go build -o grok-search . && ./grok-search
```

## 运行模式

一个二进制，两种模式：

1. **stdio MCP server**（默认）— `./grok-search`，给 Claude Code / Cherry Studio / Codex 等 MCP 客户端用。
2. **CLI 模式** — `./grok-search cli <subcommand>`，给脚本、其它 agent、或想直接调一下的人类用。和 MCP 模式共用同一份 engine 代码（`internal/engine/*`），不走 MCP 协议。

CLI 子命令：

| 命令 | 说明 |
|------|------|
| `search <query>` | Grok 池搜索（TinyFish / Exa / Tavily 兑底）。flags: `--platform`, `--model`, `--timeout`, `--json` |
| `fetch <url>` | Jina Reader 抓取（TinyFish Fetch / Exa Contents / Tavily Extract 兑底）。flags: `--timeout`, `--json` |
| `map <url>` | Tavily Map 站点映射，需要 `TAVILY_API_KEY`。flags: `--max-depth`, `--max-breadth`, `--limit`, `--timeout`, `--json` |
| `crawl <url>` | Tavily Crawl 站点遍历 + 内容抽取，需要 `TAVILY_API_KEY`。flags: `--instructions`, `--max-depth`, `--max-breadth`, `--limit`, `--extract-depth`, `--format`, `--include-images`, `--timeout`, `--json` |
| `probe` | 列出每个 Grok 端点 + `/models` 探活 + Tavily/Jina 状态。flags: `--list-timeout`, `--preview`, `--json` |
| `plan <query>` | 离线生成多步搜索计划（不调网络）。flags: `--depth`(quick/standard/deep), `--platform` |
| `research <query>` | 执行组合式 research workflow（规划→搜索→取源→抓取→打包）。flags: `--depth`(quick/standard/deep), `--platform`, `--domain`, `--max-fetches`, `--json` |

示例：

```bash
./grok-search cli probe --json
./grok-search cli search "X 上 grok 4.20 的最新评价" --platform Twitter --json
./grok-search cli fetch "https://example.com/article" --json
./grok-search cli plan "调研主题" --depth deep
./grok-search cli crawl "https://example.com/docs" --instructions "Find API pages" --limit 10 --json
./grok-search cli research "调研主题" --depth deep --domain example.com --max-fetches 6 --json
```

CLI 用同样的配置链（env > `~/.config/grok-search/config.json` > 旧版 `endpoints.json`），所以 MCP 模式调好了 CLI 也能直接用。flag 支持任意位置（`cli search "q" --platform X` 和 `cli search --platform X "q"` 都行）。

## 环境变量

### Grok 端点（三选一，按优先级生效）

1. **`GROK_ENDPOINTS_JSON`** — 推荐。多端点池的 inline JSON 数组：

   ```bash
   export GROK_ENDPOINTS_JSON='[
     {"name":"wykon","baseURL":"https://grok2api.wykon.homes/v1","apiKey":"sk-...","model":"grok-4.20-fast","sendSearchFlag":false},
     {"name":"yyds","baseURL":"https://yyds.215.im/v1","apiKey":"sk-...","model":"grok-4.20-fast","sendSearchFlag":false}
   ]'
   ```

2. **`GROK_ENDPOINTS_FILE`** — JSON 文件路径，结构同上。

3. **`GROK_API_URL` + `GROK_API_KEY`**（向后兼容的单端点）：
   - `GROK_MODEL`（默认 `grok-3-mini`）
   - `GROK_NAME`（默认 `default`）
   - `GROK_SEND_SEARCH_FLAG`（默认 `true`）

字段说明：

| 字段 | 必填 | 说明 |
|------|------|------|
| `name` | 否 | 标识名（出现在 `engine: <name> (<model>)` 行和 get_config_info 输出里），缺省 `endpoint-N` |
| `baseURL` | ✅ | OpenAI 兼容根路径，需以 `/v1` 结尾 |
| `apiKey` | ✅ | Bearer token |
| `model` | 否 | 默认 `grok-3-mini` |
| `sendSearchFlag` | 否 | 是否在请求体加 `"search":true`。xAI 原生需要 true；多数 grok2api（自动联网）建议 false |

### 其他可选

- `TAVILY_API_KEY` / `TAVILY_API_URL` — web_fetch / web_map / web_crawl / web_search 兑底
- `TAVILY_ENABLED` — 默认 `true`，置 `false` 完全关掉 Tavily 路径
- `EXA_API_KEY` / `EXA_API_URL` — web_search / web_fetch 中 Grok/Jina 后、Tavily 前的 Exa 兜底
- `EXA_ENABLED` — 默认 `true`，置 `false` 完全关掉 Exa 路径
- `JINA_API_URL` — 默认 `https://r.jina.ai`
- `JINA_API_KEY` — 可选，仅用于提升 Jina 速率上限
- `GROK_DEBUG` / `GROK_LOG_LEVEL`

## 源解析优先级（grok.go 内部）

1. `choices[0].message.annotations[].url_citation.url`（OpenAI tools-spec / 多数 grok2api）
2. 顶层 `search_sources[].url`（grok2api wykon/yyds 风味）
3. 顶层 `citations[]`（xAI 原生）
4. 顶层 `search_results[].url`（旧版 grok2api）
5. 正文里的明文 URL 正则兜底

## TODO

- [x] 安装 Go 运行时
- [x] `go mod tidy` 拉取依赖
- [x] 修复 server.go 中 stdio transport 的 placeholder
- [x] 补充 Grok 响应中 sources 的解析逻辑（citations / search_results / 文本兜底，含单测）
- [x] 集成 Jina Reader 替代 Firecrawl（web_fetch: Jina → Tavily Extract 兜底）
- [x] web_search 接 Tavily Search 兑底（Grok 失败/空响应时降级）
- [x] Grok 多端点池 + 自动降级（grok2api annotations / search_sources 解析）
- [x] 接入 Exa Search / Contents 作为 source-first 兜底
- [x] 添加 Tavily Crawl（web_crawl + CLI crawl）
- [x] 添加组合式 research workflow（research_run + CLI research）
- [ ] 添加智能重试（指数退避 + Retry-After）
- [ ] 添加 switch_model 工具
- [x] 添加 search_planning 工具
- [ ] 添加 Claude Code 集成配置命令
- [ ] README 中文 + 英文
<!-- TRELLIS:START -->
# Trellis Instructions

These instructions are for AI assistants working in this project.

This project is managed by Trellis. The working knowledge you need lives under `.trellis/`:

- `.trellis/workflow.md` — development phases, when to create tasks, skill routing
- `.trellis/spec/` — package- and layer-scoped coding guidelines (read before writing code in a given layer)
- `.trellis/workspace/` — per-developer journals and session traces
- `.trellis/tasks/` — active and archived tasks (PRDs, research, jsonl context)

If a Trellis command is available on your platform (e.g. `/trellis:finish-work`, `/trellis:continue`), prefer it over manual steps. Not every platform exposes every command.

If you're using Codex or another agent-capable tool, additional project-scoped helpers may live in:
- `.agents/skills/` — reusable Trellis skills
- `.codex/agents/` — optional custom subagents

## Subagents

- ALWAYS wait for all subagents to complete before yielding.
- Spawn subagents automatically when:
  - Parallelizable work (e.g., install + verify, npm test + typecheck, multiple tasks from plan)
  - Long-running or blocking tasks where a worker can run independently.
  - Isolation for risky changes or checks

Managed by Trellis. Edits outside this block are preserved; edits inside may be overwritten by a future `trellis update`.

<!-- TRELLIS:END -->
