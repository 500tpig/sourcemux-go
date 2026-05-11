# Grok Search Go 交接说明

本文面向后续维护、服务器部署和 MCP 客户端接入。不要把真实 API key 写入本文或 Git。

## 当前状态

- MCP server：stdio transport
- 主入口：`main.go`
- 配置入口：`internal/config/config.go`
- MCP 注册：`internal/server/server.go`
- 工具：
  - `web_search`：Grok endpoint pool -> TinyFish Search -> Exa Search -> Tavily Search 兜底
  - `get_sources`：按 `session_id` 取上次搜索来源
  - `web_fetch`：Jina Reader -> TinyFish Fetch -> Exa Contents -> Tavily Extract 兜底
  - `web_map`：Tavily Map，只发现 URL
  - `web_crawl`：Tavily Crawl，站点遍历 + 内容抽取
  - `search_planning`：复杂研究前生成分阶段搜索计划
  - `research_run`：组合式 research workflow，串联计划、搜索、来源读取、抓取和打包
  - `get_config_info`：配置诊断和 endpoint `/models` 探活
- 已支持：
  - 多 Grok-compatible endpoint
  - 单文件配置 `./grok-search.json` 或显式 `--config /path/to/grok-search.json`
  - baseURL 自动补 `/v1`
  - `text/event-stream` / SSE chat completion 响应解析
  - 429 / 5xx / 网络错误重试
  - 可选 Grok pool 总超时
  - 项目级 Codex skill：`.codex/skills/grok-search-mcp/SKILL.md`

## 本地快速验证

```bash
cd /Users/johnsmith/Project/Study/grok-search-go
go test ./...
go vet ./...
go build -o grok-search .
```

本地推荐配置文件：

```text
./grok-search.json
```

建议权限：

```bash
chmod 600 grok-search.json
```

推荐配置：

```json
{
  "grokEndpoints": [
    {
      "name": "primary",
      "baseURL": "https://your-endpoint.example/v1",
      "apiKey": "sk-...",
      "model": "grok-4.20-fast",
      "sendSearchFlag": false
    }
  ],
  "tavily": {
    "apiKey": "tvly-...",
    "apiURL": "https://api.tavily.com",
    "enabled": true
  },
  "exa": {
    "apiKey": "exa-...",
    "apiURL": "https://api.exa.ai",
    "enabled": true
  },
  "jina": {
    "apiKey": "jina_...",
    "apiURL": "https://r.jina.ai"
  },
  "grokPoolTimeoutSec": 45,
  "logLevel": "INFO"
}
```

## 推荐配置

### 本地开发

```json
{
  "grokEndpoints": [
    {
      "name": "primary",
      "baseURL": "https://your-primary-endpoint.example/v1",
      "apiKey": "sk-...",
      "model": "grok-4.20-fast",
      "sendSearchFlag": false
    },
    {
      "name": "backup",
      "baseURL": "https://your-backup-endpoint.example/v1",
      "apiKey": "sk-...",
      "model": "grok-4.20-fast",
      "sendSearchFlag": false
    }
  ],
  "tavily": {
    "apiKey": "tvly-...",
    "enabled": true
  },
  "exa": {
    "apiKey": "exa-...",
    "enabled": true
  },
  "jina": {
    "apiKey": "jina_..."
  },
  "grokPoolTimeoutSec": 45
}
```

运行时不读取环境变量配置链；Tavily / Exa / Jina / TinyFish key 都放在同一个 `grok-search.json`。

### 服务器部署

推荐目录：

```text
/usr/local/bin/grok-search
/etc/grok-search/grok-search.json
```

`/etc/grok-search/grok-search.json`：

```json
{
  "grokEndpoints": [
    {
      "name": "primary",
      "baseURL": "https://your-primary-endpoint.example/v1",
      "apiKey": "sk-...",
      "model": "grok-4.20-fast",
      "sendSearchFlag": false
    },
    {
      "name": "backup",
      "baseURL": "https://your-backup-endpoint.example/v1",
      "apiKey": "sk-...",
      "model": "grok-4.20-fast",
      "sendSearchFlag": false
    }
  ],
  "tavily": {
    "apiKey": "tvly-...",
    "enabled": true
  },
  "exa": {
    "apiKey": "exa-...",
    "enabled": true
  },
  "jina": {
    "apiKey": "jina_..."
  },
  "grokPoolTimeoutSec": 45,
  "logLevel": "INFO"
}
```

权限：

```bash
sudo chown root:root /etc/grok-search/grok-search.json
sudo chmod 600 /etc/grok-search/grok-search.json
sudo chmod 755 /usr/local/bin/grok-search
```

如果使用 `/etc/grok-search/grok-search.json`，启动 MCP 进程时显式传 `--config /etc/grok-search/grok-search.json`。

## Codex 接入

构建二进制：

```bash
go build -o grok-search .
```

注册 MCP：

```bash
codex mcp add-json grok-search '{
  "type": "stdio",
  "command": "/Users/johnsmith/Project/Study/grok-search-go/grok-search",
  "args": ["--config", "/Users/johnsmith/Project/Study/grok-search-go/grok-search.json"]
}'
```

如果部署在服务器，需要把 Codex/客户端里的 command 改成服务器上的二进制路径。

项目级 skill 位于：

```text
.codex/skills/grok-search-mcp/SKILL.md
```

它只定义 Codex 什么时候使用此 MCP；真正的联网搜索/抓取仍由 MCP server 提供。

## Claude Code / Cherry Studio 接入

显式传配置文件路径：

```bash
claude mcp add-json grok-search '{
  "type": "stdio",
  "command": "/path/to/grok-search",
  "args": ["--config", "/etc/grok-search/grok-search.json"]
}'
```

## 上线验收

在 MCP 客户端里依次调用：

1. `get_config_info`
   - 期望：endpoint `Probe: OK`
   - key 只应显示 mask
2. `web_search`
   - 示例 query：`今天是几号？请用一句中文回答。`
   - 期望：返回 `engine: <name> (<model>)` 和答案
   - 可选：传 `model` 临时覆盖本次 Grok 模型，不会修改配置文件
3. `web_fetch`
   - 示例 URL：`https://example.com`
   - 期望：返回 `Source: Jina Reader`
4. `web_map`
   - 仅在配置 `tavily.apiKey` 后测试
5. `web_crawl`
   - 仅在配置 `tavily.apiKey` 后测试
   - 示例 URL：`https://example.com`
   - 期望：返回 `Source: Tavily Crawl`、`base_url` 和抓取页面内容摘要
6. `search_planning`
   - 示例 query：`评估某个开源项目的最新状态`
   - 期望：返回分阶段搜索计划
7. `research_run`
   - 示例 query：`评估某个开源项目的最新状态`
   - 期望：返回 compact research pack，包含 executed searches、source summary、fetched pages summary、confirmed facts、likely inferences 和 open questions

## 常见问题

### `no Grok endpoints configured`

没有配置任何 endpoint。检查当前启动命令是否指向正确的单文件配置：

```bash
./grok-search cli --config /path/to/grok-search.json config list --json
```

### `/models` 返回 HTML

通常是 `baseURL` 配成了站点首页而不是 OpenAI-compatible API 根路径。当前代码会自动补 `/v1`，但如果该服务路径不是 `/v1`，需要在配置里写真实 API base URL。

### `/chat/completions` 返回 `text/event-stream`

已支持 SSE 解析。如果遇到 decode 错误，检查返回是否是非标准 SSE 或错误页。

### `web_map` / `web_crawl` 不可用

需要在 `grok-search.json` 的 `tavily.apiKey` 中配置 Tavily key。未配置 Tavily 时，`web_search` 仍可用 Grok/Exa/TinyFish，`web_fetch` 仍可用 Jina。

### Exa 不生效

检查 `grok-search.json` 的 `exa.apiKey`。`exa.enabled:false` 会关闭 Exa。Exa 当前只作为 Grok 后、Tavily 前的 fallback，不会主动并行打多家搜索。

### 速度慢或多个 endpoint 叠加等待

在 `grok-search.json` 设置 `grokPoolTimeoutSec: 45`。该值限制整个 Grok endpoint pool 的总 wall-clock 预算。

## 维护建议

- 默认模型：`grok-4.20-fast`
- 复杂分析可用：`grok-4.20-reasoning`
- 多 endpoint 按稳定性排序，把最稳定的放第一位
- 不要提交真实 key
- 每次改动后至少运行：

```bash
go test ./...
go vet ./...
go build -o /tmp/grok-search-check .
```
