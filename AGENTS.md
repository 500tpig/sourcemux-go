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
│   │   ├── tavily.go                # Tavily Search + Extract + Map client
│   │   └── jina.go                  # Jina Reader client（web_fetch 主力）
│   ├── server/server.go             # MCP server 初始化 + 运行
│   └── tools/
│       ├── search.go                # web_search 工具（Grok 池 → Exa → Tavily Search 降级）
│       ├── fetch.go                 # web_fetch 工具（Jina → Exa Contents → Tavily Extract 降级）
│       ├── map.go                   # web_map 工具
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
| `search <query>` | Grok 池搜索（Exa / Tavily 兑底）。flags: `--platform`, `--model`, `--timeout`, `--json` |
| `fetch <url>` | Jina Reader 抓取（Exa Contents / Tavily Extract 兑底）。flags: `--timeout`, `--json` |
| `map <url>` | Tavily Map 站点映射，需要 `TAVILY_API_KEY`。flags: `--max-depth`, `--max-breadth`, `--limit`, `--timeout`, `--json` |
| `probe` | 列出每个 Grok 端点 + `/models` 探活 + Tavily/Jina 状态。flags: `--list-timeout`, `--preview`, `--json` |
| `plan <query>` | 离线生成多步搜索计划（不调网络）。flags: `--depth`(quick/standard/deep), `--platform` |

示例：

```bash
./grok-search cli probe --json
./grok-search cli search "X 上 grok 4.20 的最新评价" --platform Twitter --json
./grok-search cli fetch "https://example.com/article" --json
./grok-search cli plan "调研主题" --depth deep
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

- `TAVILY_API_KEY` / `TAVILY_API_URL` — web_fetch / web_map / web_search 兑底
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
- [ ] 添加智能重试（指数退避 + Retry-After）
- [ ] 添加 switch_model 工具
- [ ] 添加 search_planning 工具
- [ ] 添加 Claude Code 集成配置命令
- [ ] README 中文 + 英文
