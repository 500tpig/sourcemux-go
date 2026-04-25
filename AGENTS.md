# grok-search-go

Go 版 Grok Search MCP Server — Grok AI 搜索 + Jina Reader 抓取（Tavily 兜底）。

## 项目结构

```
.
├── main.go                          # 入口
├── go.mod
├── internal/
│   ├── config/config.go             # 环境变量配置
│   ├── engine/
│   │   ├── grok.go                  # Grok API client (OpenAI 兼容)
│   │   ├── tavily.go                # Tavily Search + Extract + Map client
│   │   └── jina.go                  # Jina Reader client (web_fetch 主力)
│   ├── server/server.go             # MCP server 初始化 + 运行
│   └── tools/
│       ├── search.go                # web_search 工具 (Grok → Tavily Search 降级)
│       ├── fetch.go                 # web_fetch 工具 (Jina → Tavily Extract 降级)
│       ├── map.go                   # web_map 工具
│       ├── sources.go               # get_sources 工具
│       └── config_tool.go           # get_config_info 诊断工具
```

## 开发前提

- Go 1.22+
- 依赖: github.com/mark3labs/mcp-go, github.com/google/uuid

## 快速启动

```bash
# 安装 Go (macOS)
brew install go
# 或从 https://go.dev/dl/ 下载

# 进入项目
cd /Users/johnsmith/Project/Study/grok-search-go

# 下载依赖
go mod tidy

# 编译运行
go build -o grok-search . && ./grok-search
```

## 环境变量

必填（二选一）:
- `GUDA_API_KEY` — 一键配置 Grok / Tavily（共用同一聚合 key）
- `GROK_API_URL` + `GROK_API_KEY` — 自定义 Grok 端点

可选:
- `TAVILY_API_KEY` / `TAVILY_API_URL` — 网页抓取兜底 + web_search 兑底引擎
- `JINA_API_URL` — 默认 `https://r.jina.ai`
- `JINA_API_KEY` — 可选，仅用于提升 Jina 速率上限
- `GROK_MODEL` — 默认模型 (grok-3-mini)

## TODO

- [x] 安装 Go 运行时
- [x] `go mod tidy` 拉取依赖
- [x] 修复 server.go 中 stdio transport 的 placeholder
- [x] 补充 Grok 响应中 sources 的解析逻辑（citations / search_results / 文本兜底，含单测）
- [x] 集成 Jina Reader 替代 Firecrawl（web_fetch: Jina → Tavily Extract 兜底）
- [x] web_search 接 Tavily Search 兑底（Grok 失败/空响应时降级）
- [ ] 添加 switch_model 工具
- [ ] 添加 search_planning 工具
- [ ] 添加智能重试 (指数退避 + Retry-After)
- [ ] 添加 Claude Code 集成配置命令
- [ ] README 中文 + 英文
