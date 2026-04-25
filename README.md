# Grok Search Go

Go 实现的 MCP 服务器，为 Claude Code / Cherry Studio 等 LLM Client 提供实时网络搜索和网页抓取能力。

## 架构

```
LLM Client ──MCP──► Grok Search Server (Go)
                      ├─ web_search  ───► Grok API（AI 搜索）
                      ├─ web_fetch   ───► Jina Reader → Tavily Extract（降级抓取）
                      └─ web_map     ───► Tavily Map（站点映射）
```

## 特性

- 🚀 **单二进制部署** — 编译后无运行时依赖
- 🔍 **AI 驱动搜索** — Grok web 搜索，自动抽取信源
- 📥 **零成本抓取** — Jina Reader 免费、无需 key、Markdown 输出
- ⏰ **自动时间注入** — 检测时间相关查询，注入本地时间上下文
- 🔄 **抓取降级** — Jina Reader → Tavily Extract → 错误提示
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
# 可选：网页抓取兜底
export TAVILY_API_KEY="tvly-xxx"
# 可选：Jina 提速（不设也能用免费档）
export JINA_API_KEY="jina_xxx"
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
| `get_sources` | 获取上一次搜索的信源 URL |
| `web_fetch` | 网页内容抓取（Jina Reader 主，Tavily Extract 兜底） |
| `web_map` | 站点结构映射（Tavily Map） |
| `get_config_info` | 配置诊断 + Grok 连通性测试 |

## License

MIT
