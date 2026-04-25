# Grok Search Go

Go 实现的 MCP 服务器，为 Claude Code / Cherry Studio 等 LLM Client 提供实时网络搜索和网页抓取能力。**支持多 Grok 端点池 + 自动降级**。

## 架构

```
LLM Client ──MCP──► Grok Search Server (Go)
                      ├─ web_search  ───► Grok 端点池（按优先级降级）→ Tavily Search 兑底
                      ├─ web_fetch   ───► Jina Reader → Tavily Extract（降级抓取）
                      └─ web_map     ───► Tavily Map（站点映射）
```

## 特性

- 🚀 **单二进制部署** — 编译后无运行时依赖
- 🔍 **AI 驱动搜索** — Grok web 搜索 + 5 级 source URL 抽取（annotations / search_sources / citations / search_results / 正则）
- 🔄 **多端点池** — 一个 grok2api 挂了自动切到下一个，最后兜底 Tavily
- 📥 **零成本抓取** — Jina Reader 免费、无需 key、Markdown 输出
- ⏰ **自动时间注入** — 检测时间相关查询，注入本地时间上下文
- 🔑 **OpenAI 兼容** — 任意 Grok 镜像 / grok2api / xAI 原生

## 安装

```bash
go install github.com/bettas/grok-search-go@latest
# 或本地编译
git clone https://github.com/bettas/grok-search-go.git
cd grok-search-go
go build -o grok-search .
```

## 配置

### 多端点池（推荐）

```bash
export GROK_ENDPOINTS_JSON='[
  {"name":"wykon","baseURL":"https://grok2api.wykon.homes/v1","apiKey":"sk-...","model":"grok-4.20-fast","sendSearchFlag":false},
  {"name":"yyds","baseURL":"https://yyds.215.im/v1","apiKey":"sk-...","model":"grok-4.20-fast","sendSearchFlag":false}
]'
# 或写到文件
export GROK_ENDPOINTS_FILE=/path/to/endpoints.json
# 可选：网页抓取兑底
export TAVILY_API_KEY="tvly-xxx"
# 可选：Jina 提速
export JINA_API_KEY="jina_xxx"
```

### 单端点（向后兼容）

```bash
export GROK_API_URL="https://your-endpoint/v1"
export GROK_API_KEY="sk-..."
export GROK_MODEL="grok-4.20-fast"
export GROK_SEND_SEARCH_FLAG="false"   # grok2api 类代理通常应关掉
```

### 添加到 Claude Code

```bash
claude mcp add-json grok-search '{
  "type": "stdio",
  "command": "/path/to/grok-search",
  "env": {
    "GROK_ENDPOINTS_JSON": "[{\"name\":\"wykon\",\"baseURL\":\"https://grok2api.wykon.homes/v1\",\"apiKey\":\"sk-...\",\"model\":\"grok-4.20-fast\",\"sendSearchFlag\":false}]"
  }
}'
```

## MCP 工具

| 工具 | 描述 |
|------|------|
| `web_search` | AI 驱动的网络搜索（Grok 池 → Tavily Search 兑底）。返回 `engine: <name> (<model>)` + `session_id` + 答案 |
| `get_sources` | 获取上一次搜索（按 session_id）的信源 URL |
| `web_fetch` | 网页内容抓取（Jina Reader 主，Tavily Extract 兑底） |
| `web_map` | 站点结构映射（Tavily Map） |
| `get_config_info` | 列出每个 Grok 端点 + 探活（GET /models）+ Tavily/Jina 状态 |

## 端点字段

| 字段 | 必填 | 默认 | 说明 |
|------|------|------|------|
| `name` | 否 | `endpoint-N` | 显示名 |
| `baseURL` | ✅ | — | OpenAI 兼容根路径（含 `/v1`） |
| `apiKey` | ✅ | — | Bearer token |
| `model` | 否 | `grok-3-mini` | 模型 ID |
| `sendSearchFlag` | 否 | `false` | xAI 原生需 `true`；多数 grok2api 自动联网，置 `false` |

## License

MIT
