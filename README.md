# Grok Search Go

Go 实现的 MCP 服务器，为 Claude Code / Cherry Studio 等 LLM Client 提供实时网络搜索和网页抓取能力。

## 架构

```
LLM Client ──MCP──► Grok Search Server (Go)
                      ├─ web_search  ───► Grok API（AI 搜索）
                      ├─ web_fetch   ───► Tavily Extract → Firecrawl Scrape（降级抓取）
                      └─ web_map     ───► Tavily Map（站点映射）
```

## 特性

- 🚀 **单二进制部署** — 编译后无运行时依赖
- 🔍 **双引擎搜索** — Grok AI 搜索 + Tavily/Firecrawl 内容抓取
- ⏰ **自动时间注入** — 检测时间相关查询，注入本地时间上下文
- 🔄 **三级降级** — Tavily Extract → Firecrawl Scrape → 错误提示
- 🔑 **OpenAI 兼容** — 支持任意 Grok 镜像站 / grok2api

## 安装

```bash
# 需要 Go 1.22+
go install github.com/bettas/grok-search-go@latest

# 或本地编译
git clone https://github.com/bettas/grok-search-go.git
cd grok-search-go
go build -o grok-search .
```

## 配置

### 自定义 API（使用自己的 grok2api）

```bash
export GROK_API_URL="https://your-grok2api-endpoint/v1"
export GROK_API_KEY="your-key"
# 可选
export TAVILY_API_KEY="tvly-xxx"
```

### 添加到 Claude Code

```bash
claude mcp add-json grok-search '{  
  "type": "stdio",
  "command": "/path/to/grok-search",
  "env": {
    "GROK_API_URL": "https://your-endpoint/v1",
    "GROK_API_KEY": "your-key"
  }
}'
```

## MCP 工具

| 工具 | 描述 |
|------|------|
| `web_search` | AI 驱动的网络搜索（via Grok） |
| `get_sources` | 获取搜索信源 |
| `web_fetch` | 网页内容抓取（Tavily + Firecrawl 降级） |
| `web_map` | 站点结构映射 |
| `get_config_info` | 配置诊断 |

## License

MIT
