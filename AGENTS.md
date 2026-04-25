# grok-search-go

Go 版 Grok Search MCP Server — 双引擎架构（Grok AI 搜索 + Tavily/Firecrawl 网页抓取）。

## 项目结构

```
.
├── main.go                          # 入口
├── go.mod
├── internal/
│   ├── config/config.go             # 环境变量配置
│   ├── engine/
│   │   ├── grok.go                  # Grok API client (OpenAI 兼容)
│   │   ├── tavily.go                # Tavily Extract + Map client
│   │   └── firecrawl.go             # Firecrawl Scrape fallback
│   ├── server/server.go             # MCP server 初始化 + 运行
│   └── tools/
│       ├── search.go                # web_search 工具
│       ├── fetch.go                 # web_fetch 工具 (Tavily→Firecrawl 降级)
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
- `GUDA_API_KEY` — 一键配置所有服务
- `GROK_API_URL` + `GROK_API_KEY` — 自定义 Grok 端点

可选:
- `TAVILY_API_KEY` / `TAVILY_API_URL` — 网页抓取
- `FIRECRAWL_API_KEY` / `FIRECRAWL_API_URL` — 抓取降级
- `GROK_MODEL` — 默认模型 (grok-3-mini)

## TODO

- [x] 安装 Go 运行时
- [x] `go mod tidy` 拉取依赖
- [x] 修复 server.go 中 stdio transport 的 placeholder
- [x] 补充 Grok 响应中 sources 的解析逻辑（citations / search_results / 文本兜底，含单测）
- [ ] 添加 switch_model 工具
- [ ] 添加 search_planning 工具
- [ ] 添加智能重试 (指数退避 + Retry-After)
- [ ] 写测试
- [ ] 添加 Claude Code 集成配置命令
- [ ] README 中文 + 英文
