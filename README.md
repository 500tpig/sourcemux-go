# Grok Search Go

Go 实现的 MCP 服务器，为 Claude Code / Cherry Studio 等 LLM Client 提供实时网络搜索和网页抓取能力。**支持多 Grok 端点池 + 自动降级**。

## 架构

```
LLM Client ──MCP──► Grok Search Server (Go)
                      ├─ web_search  ───► Grok 端点池（按优先级降级）→ TinyFish Search → Exa Search → Tavily Search 兑底
                      ├─ web_fetch   ───► Jina Reader → TinyFish Fetch → Exa Contents → Tavily Extract（降级抓取）
                      ├─ exa_search_advanced   ─► Exa Search 高级模式（deep / deep-reasoning / output schema）
                      ├─ exa_contents_advanced ─► Exa Contents 高级模式（subpages / maxAgeHours）
                      ├─ web_map     ───► Tavily Map（只发现 URL）
                      ├─ web_crawl   ───► Tavily Crawl（站点遍历 + 内容抽取）
                      └─ research_run ─► 组合式 research workflow（计划→搜索→取源→抓取→打包）
```

## 特性

- 🚀 **单二进制部署** — 编译后无运行时依赖
- 🔍 **AI 驱动搜索** — Grok web 搜索 + 5 级 source URL 抽取（annotations / search_sources / citations / search_results / 正则）
- 🔄 **多端点池** — 一个 grok2api 挂了自动切到下一个，最后兜底 Tavily
- 🧭 **Exa 兜底** — 可选接入 Exa Search / Contents，适合本地 agent 的 source-first 检索与抓取
- 🧠 **高级 Exa 模式** — 显式暴露 Exa deep / deep-reasoning、output schema、subpages、maxAgeHours；不改变默认搜索/抓取路由
- 🐟 **TinyFish 多 key 兜底** — 可选接入 TinyFish Search / Fetch，多 key 轮询并在 429/错误时自动换 key
- 📥 **零成本抓取** — Jina Reader 免费、无需 key、Markdown 输出
- 🕸️ **站点级 Crawl** — Tavily Crawl 可遍历站点并返回抽取内容；`web_map` 只发现 URL，`web_crawl` 返回内容
- 🧩 **组合式 Research Run** — `research_run` / `cli research` 执行有界内存工作流，串联 `search_planning`、`web_search`、`get_sources`、`web_fetch`，必要时使用 `web_map` / `web_crawl`
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

## 交接与部署

完整交接、服务器部署、Codex / Claude Code 接入、验收与排障步骤见：

- [`docs/HANDOFF.md`](docs/HANDOFF.md)

## 配置

### 统一配置文件（推荐）

默认会读取 **`~/.config/grok-search/config.json`**（遵循 XDG，可用 `XDG_CONFIG_HOME` 覆盖）。本地使用时建议把 Grok / Tavily / Jina 都放这里，MCP 客户端只需要配置 `command`。

```bash
mkdir -p ~/.config/grok-search
cat > ~/.config/grok-search/config.json <<'JSON'
{
  "grokEndpoints": [
    {"name":"wykon","baseURL":"https://grok2api.wykon.homes/v1","apiKey":"sk-...","model":"grok-4.20-fast","sendSearchFlag":false},
    {"name":"yyds","baseURL":"https://yyds.215.im/v1","apiKey":"sk-...","model":"grok-4.20-fast","sendSearchFlag":false}
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
  "tinyfish": {
    "enabled": true,
    "searchURL": "https://api.search.tinyfish.ai",
    "fetchURL": "https://api.fetch.tinyfish.ai",
    "keys": [
      {"name": "acct-a", "apiKey": "tf_..."},
      {"name": "acct-b", "apiKey": "tf_..."}
    ]
  },
  "grokPoolTimeoutSec": 45,
  "logLevel": "INFO"
}
JSON
chmod 600 ~/.config/grok-search/config.json
```

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
# 可选：source-first 搜索 / 抓取兑底
export EXA_API_KEY="exa-xxx"
# 可选：Jina 提速
export JINA_API_KEY="jina_xxx"
# 可选：TinyFish Search / Fetch 生产兜底，多 key 逗号分隔
export TINYFISH_API_KEYS="tf_key_1,tf_key_2"
export TINYFISH_KEY_NAMES="acct-a,acct-b"
export TINYFISH_ENABLED="true"
```

### 单端点（向后兼容）

```bash
export GROK_API_URL="https://your-endpoint/v1"
export GROK_API_KEY="sk-..."
export GROK_MODEL="grok-4.20-fast"
export GROK_SEND_SEARCH_FLAG="false"   # grok2api 类代理通常应关掉
```

### 旧版默认端点文件（无需 env）

如果上面这些 env 和 `config.json` 都没设置 Grok endpoint，会继续尝试读 **`~/.config/grok-search/endpoints.json`**。这是旧版兼容路径，只能配置 Grok endpoint；Tavily / Jina 建议写到 `config.json` 或 env。

```bash
mkdir -p ~/.config/grok-search
cat > ~/.config/grok-search/endpoints.json <<'JSON'
[
  {"name":"wykon","baseURL":"https://grok2api.wykon.homes/v1","apiKey":"sk-...","model":"grok-4.20-fast","sendSearchFlag":false},
  {"name":"yyds","baseURL":"https://yyds.215.im/v1","apiKey":"sk-...","model":"grok-4.20-fast","sendSearchFlag":false}
]
JSON
chmod 600 ~/.config/grok-search/endpoints.json   # 包含 API key，建议收紧权限
```

优先级：

```text
环境变量 > GROK_ENDPOINTS_JSON / GROK_ENDPOINTS_FILE > GROK_API_URL + GROK_API_KEY > ~/.config/grok-search/config.json > ~/.config/grok-search/endpoints.json
```

最小可用配置只需要 Grok endpoint；建议先从 endpoint 的 `/models` 返回列表里选模型：

- 默认推荐：`grok-4.20-fast`（搜索、摘要、日常 research）
- 高质量兜底：`grok-4.20-reasoning`（复杂归纳、长链路分析）
- 兼容兜底：`grok-3-mini`（旧端点默认值）

如果使用 xAI 原生端点通常需要 `sendSearchFlag: true`；多数 grok2api / OpenAI-compatible 代理建议 `false`。

这样 Claude Code / Cherry Studio 那边的 MCP 注册就只需要一行 `command`，不用再传 `env`：

```bash
claude mcp add grok-search /path/to/grok-search
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

### 添加到 Codex

Codex 推荐走 MCP。先编译二进制：

```bash
go build -o grok-search .
```

然后在 Codex 配置中添加 stdio MCP server，或使用 Codex CLI 添加：

```bash
codex mcp add grok-search /Users/johnsmith/Project/Study/grok-search-go/grok-search
```

本仓库同时提供项目级 skill：`.codex/skills/grok-search-mcp/SKILL.md`。它只负责告诉 Codex 什么时候、怎样调用 MCP；真正的搜索/抓取仍由 MCP server 执行。

配置完成后，在 Codex / Claude 中调用 `get_config_info`，确认 endpoint 探活成功。

## CLI 模式（无需 MCP）

同一个二进制还提供 `grok-search cli <subcommand>` 子命令，方便脚本、CI、或者其它没法接 stdio MCP 的 agent 直接调用。和 MCP 模式共用一份 engine 代码 + 一份配置链。

```bash
./grok-search cli probe --json                                            # 端点探活 + 配置概览
./grok-search cli search "X 上 grok 4.20 的最新评价" --platform Twitter --json
./grok-search cli search "被 CF 墙后的某产品评测" --model grok-4.20-fast
./grok-search cli fetch  "https://example.com/article" --json             # Jina, Tavily 兑底
./grok-search cli exa-search "最新 AI 芯片发布" \
  --type deep --output-schema-json '{"type":"object","properties":{"answer":{"type":"string"}}}' --json
./grok-search cli exa-contents "https://example.com/docs" \
  --subpages 3 --subpage-target api --max-age-hours 0 --json
./grok-search cli map    "https://example.com" --limit 50 --json          # 需 TAVILY_API_KEY
./grok-search cli crawl  "https://example.com/docs" \
  --instructions "Find API reference pages" --limit 10 --json             # 需 TAVILY_API_KEY
./grok-search cli plan   "调研主题" --depth deep                          # 离线计划，不打网络
./grok-search cli research "调研主题" \
  --depth deep --domain example.com --max-fetches 6 --json                # 执行搜索/抓取并输出 research pack
./grok-search cli tinyfish-bench --json                                   # 本地 TinyFish 评测
./grok-search cli --help                                                  # 完整 usage
```

子命令一览：

| 命令 | 引擎 | 主要 flags |
|------|------|-----------|
| `search <query>` | Grok 池 → TinyFish → Exa → Tavily 兑底 | `--platform` `--model` `--timeout` `--json` |
| `fetch <url>` | Jina Reader → TinyFish Fetch → Exa Contents → Tavily Extract | `--timeout` `--json` |
| `exa-search <query>` | Exa Search 高级模式 | `--type` `--num-results` `--text` `--text-max-characters` `--highlights` `--highlights-query` `--system-prompt` `--output-schema-json` `--timeout` `--json` |
| `exa-contents <url>` | Exa Contents 高级模式 | `--text` `--text-max-characters` `--highlights` `--highlights-query` `--subpages` `--subpage-target` `--max-age-hours` `--timeout` `--json` |
| `map <url>` | Tavily Map | `--max-depth` `--max-breadth` `--limit` `--timeout` `--json` |
| `crawl <url>` | Tavily Crawl | `--instructions` `--max-depth` `--max-breadth` `--limit` `--extract-depth` `--format` `--include-images` `--timeout` `--json` |
| `probe` | 配置 + `/models` 探活 | `--list-timeout` `--preview` `--json` |
| `plan <query>` | 纯逻辑（不调网络）| `--depth` `--platform` |
| `research <query>` | 组合式执行：规划查询 → 多轮搜索 → 取 sources → 去重排序 → 抓取 top-N → 输出 research pack | `--depth` `--platform` `--domain` `--max-fetches` `--timeout` `--json` |
| `tinyfish-bench` | TinyFish Search / Fetch / Agent 本地评测 | `--cases` `--keys-file` `--surfaces` `--timeout` `--json` |

`map` 和 `crawl` 的区别：

- `map` 只做站点 URL 发现，输出 URL 列表，适合先判断站点结构。
- `crawl` 会遍历站点并抽取每个页面的 `raw_content`，适合深度研究、文档打包、下游 LLM 读取。

`research` 和相邻命令的区别：

- `search` 只跑一次搜索并返回答案/信源；`research` 会先规划多个查询，再汇总多个搜索 session 的来源。
- `fetch` 只抓取一个 URL；`research` 会对候选 URL 去重、启发式排序后抓取 top-N。
- `crawl` 是站点遍历能力；`research` 只在指定 `--domain` 且有用时复用现有 Tavily Map/Crawl，不重新实现 crawl provider。
- `max_fetches` / `--max-fetches` 是执行上限，v1 会限制在最多 12 个 URL，避免 research pack 过大。

### 高级 Exa 模式

默认 `search` / `fetch` 路由不变：

- `search` 仍然是 Grok 池 → TinyFish → Exa → Tavily
- `fetch` 仍然是 Jina → TinyFish → Exa → Tavily

如果你想直接使用 Exa 的更强能力，再显式调用：

- MCP: `exa_search_advanced`
- MCP: `exa_contents_advanced`
- CLI: `exa-search`
- CLI: `exa-contents`

适用场景：

- 需要 `deep` / `deep-reasoning`
- 需要 `outputSchema`
- 需要 `systemPrompt`
- 需要 `subpages`
- 需要 `maxAgeHours`

不适用场景：

- 只是想保持默认、低心智负担的搜索/抓取体验
- 希望继续依赖当前多 provider 自动降级路由

设计要点：

- flag 支持任意位置：`cli search "q" --platform X` 与 `cli search --platform X "q"` 等价。
- `--json` 全部子命令都支持；不传时输出人读格式。
- 配置链与 MCP 一致：env > `~/.config/grok-search/config.json` > `~/.config/grok-search/endpoints.json`。
- Codex 用户可直接 `claude mcp add grok-search ...`（MCP）+ `./grok-search cli ...`（CLI）双管齐下；CLI 模式对应 skill 见 `.codex/skills/grok-search-cli/SKILL.md`。

### TinyFish 本地 benchmark

`tinyfish-bench` 是隔离评测工具，只调用 TinyFish REST Search / Fetch / sync Agent API；生产 MCP `web_search` / `web_fetch` 是否使用 TinyFish 由上面的 `tinyfish.enabled` / `TINYFISH_ENABLED` 配置决定。

```bash
export TINYFISH_API_KEYS='tf_key_1,tf_key_2'
export TINYFISH_KEY_NAMES='free-a,free-b' # 可选，仅用于输出标识

./grok-search cli tinyfish-bench \
  --cases docs/tinyfish-benchmark-cases.sample.json \
  --surfaces search,fetch,agent \
  --json
```

也可以用本地密钥文件（不要提交）：

```json
{
  "keys": [
    {"name": "acct-a", "apiKey": "tf_..."},
    {"name": "acct-b", "apiKey": "tf_..."}
  ]
}
```

```bash
./grok-search cli tinyfish-bench --keys-file /path/to/tinyfish-keys.json --json
```

输出只包含 masked key status（如 `tf_1...abcd`），不会打印完整 API key。样例 cases 文件在 `docs/tinyfish-benchmark-cases.sample.json`，不包含任何密钥。

### 健康检查脚本（`scripts/test_grok_models.sh`）

用来一键体检整个端点池 + 验证 fast / auto / expert 等模型档位是否真的能跑通：

```bash
./scripts/test_grok_models.sh                     # 默认测 fast/auto/expert
./scripts/test_grok_models.sh -q "今天日期？"     # 自定义 query
./scripts/test_grok_models.sh -m grok-4.20-fast \
                              -m grok-4.20-0309-reasoning  # 只测指定模型
./scripts/test_grok_models.sh -t 90s              # 单次搜索超时
./scripts/test_grok_models.sh --bin /abs/grok-search       # 指定二进制
```

它会做三件事：

1. 调 `cli probe`，列出每个端点 `ok` 状态、默认模型、并标记是否声明支持 `fast/auto/expert`。
2. 对每个待测模型跑一次真实 `cli search`，记录：实际命中的 `engine/endpoint_name`、返回的 `model`、`sources_count`、耗时、回答片段。
3. 最后输出 PASS/FAIL 汇总，**退出码 0=全部成功、1=任一失败**，方便接 CI / cron。

典型用途：

- 怀疑 `primary` 端点 auto/expert 余额掉了 → 跑一次看是否被路由到了别的 endpoint。
- 新加 endpoint 后做冒烟测试。
- 想确认搜索是否真的带回了 sources（`grok-4.20-fast` 一般 `sources_count > 0`，纯回答模型常为 0）。

脚本只用 bash + 可选 `python3`/`jq` 做 JSON 解析，无额外依赖。

## MCP 工具

| 工具 | 描述 |
|------|------|
| `web_search` | AI 驱动的网络搜索（Grok 池 → TinyFish Search → Exa Search → Tavily Search 兑底）。支持按次传 `model` 覆盖默认模型；返回 `engine` + `session_id` + 答案/信源摘要 |
| `get_sources` | 获取上一次搜索（按 session_id）的信源 URL |
| `web_fetch` | 网页内容抓取（Jina Reader → TinyFish Fetch → Exa Contents → Tavily Extract） |
| `exa_search_advanced` | 直接调用 Exa Search 高级模式；显式使用 `deep` / `deep-reasoning` / `outputSchema` / `systemPrompt`，不改变默认 `web_search` 路由 |
| `exa_contents_advanced` | 直接调用 Exa Contents 高级模式；显式使用 `subpages` / `subpageTarget` / `maxAgeHours`，不改变默认 `web_fetch` 路由 |
| `web_map` | 站点结构映射（Tavily Map，只返回 URL） |
| `web_crawl` | 站点级遍历 + 内容抽取（Tavily Crawl，返回每页 `raw_content`） |
| `get_config_info` | 列出每个 Grok 端点 + 探活（GET /models）+ Tavily/Jina 状态 |
| `search_planning` | 复杂研究前生成分阶段搜索计划，指导后续 `web_search` / `get_sources` / `web_fetch` / `web_crawl` |
| `research_run` | 执行组合式研究工作流：调用规划逻辑、跑多轮 `web_search`、读取 `get_sources`、去重/排序 URL、抓取 top-N，并输出 compact research pack |

`web_search` 参数：

| 参数 | 必填 | 说明 |
|------|------|------|
| `query` | ✅ | 搜索查询 |
| `platform` | 否 | 聚焦平台，如 `GitHub, Reddit` |
| `model` | 否 | 按次覆盖 Grok 模型，不修改配置文件，如 `grok-4.20-fast` |

`exa_search_advanced` 参数：

| 参数 | 必填 | 说明 |
|------|------|------|
| `query` | ✅ | 搜索查询 |
| `search_type` | 否 | `instant` / `fast` / `auto` / `neural` / `deep-lite` / `deep` / `deep-reasoning` |
| `num_results` | 否 | 返回结果数量，默认 5 |
| `text` | 否 | 返回全文内容 |
| `text_max_characters` | 否 | 全文最大字符数 |
| `highlights` | 否 | 返回 highlights；当未选择其它内容模式时会默认启用 |
| `highlights_query` | 否 | 引导 Exa 抽取 highlights 的 query |
| `system_prompt` | 否 | Exa `systemPrompt` |
| `output_schema_json` | 否 | Exa `outputSchema` 的 JSON object 字符串 |

`exa_contents_advanced` 参数：

| 参数 | 必填 | 说明 |
|------|------|------|
| `url` | ✅ | 目标 URL |
| `text` | 否 | 返回全文内容；当未选择其它内容模式时会默认启用 |
| `text_max_characters` | 否 | 全文最大字符数 |
| `highlights` | 否 | 返回 highlights |
| `highlights_query` | 否 | 引导 Exa 抽取 highlights 的 query |
| `subpages` | 否 | 需要发现/抓取的子页面数量 |
| `subpage_target` | 否 | 子页面筛选词数组，对应 Exa `subpageTarget` |
| `max_age_hours` | 否 | Exa `maxAgeHours`；`0` 表示强制新抓，`-1` 表示只读缓存 |

`search_planning` 参数：

| 参数 | 必填 | 说明 |
|------|------|------|
| `query` | ✅ | 研究问题 |
| `depth` | 否 | `quick` / `standard` / `deep`，默认 `standard` |
| `platform` | 否 | 聚焦平台 |

`research_run` 参数：

| 参数 | 必填 | 说明 |
|------|------|------|
| `query` | ✅ | 研究问题 |
| `depth` | 否 | `quick` / `standard` / `deep`，默认 `standard` |
| `platform` | 否 | 聚焦平台 |
| `domains` | 否 | 域名/站点 allow-list；用于过滤/优先排序，也可触发 `web_map` / `web_crawl` 站点扩展 |
| `max_fetches` | 否 | 最多抓取的高信号 URL 数；默认随 depth 调整，v1 上限为 12 |

`web_crawl` 参数：

| 参数 | 必填 | 说明 |
|------|------|------|
| `url` | ✅ | 起始站点 URL |
| `instructions` | 否 | 自然语言 crawl 指令，如只找 API 文档页 |
| `max_depth` | 否 | 最大深度，默认 `1` |
| `max_breadth` | 否 | 每层/每页最大链接数，默认 `20` |
| `limit` | 否 | 总页面数限制，默认 `10` |
| `extract_depth` | 否 | `basic` / `advanced`，默认 `basic` |
| `format` | 否 | `markdown` / `text`，默认 `markdown` |
| `include_images` | 否 | 是否包含图片 URL，默认 `false` |

## 端点字段

| 字段 | 必填 | 默认 | 说明 |
|------|------|------|------|
| `name` | 否 | `endpoint-N` | 显示名 |
| `baseURL` | ✅ | — | OpenAI 兼容根路径（含 `/v1`） |
| `apiKey` | ✅ | — | Bearer token |
| `model` | 否 | `grok-3-mini` | 模型 ID |
| `sendSearchFlag` | 否 | `false` | xAI 原生需 `true`；多数 grok2api 自动联网，置 `false` |

## 超时与限额

| env | 默认 | 说明 |
|------|------|------|
| `GROK_POOL_TIMEOUT_SEC` | 未设 / `0` | GrokPool 全局 wall-clock 预算（秒）。`>0` 时所有端点 + 重试合计不会超过该预算，避免多个端点均慢时堆叠到 `端点数 × MaxAttempts × MaxDelay`。推荐 `30` ～ `90`。 |
| `EXA_API_KEY` | 未设 | 可选。开启 Exa Search / Contents 作为 Grok 后、Tavily 前的 source-first fallback。 |
| `EXA_API_URL` | `https://api.exa.ai` | 可选。Exa API 根路径。 |
| `EXA_ENABLED` | `true` | 可选。设为 `false` 可临时关闭 Exa。 |

## 本地诊断

检查每个 endpoint 的 `/models` 和实际 chat/search 可用性：

```bash
go run ./cmd/grok-diagnose
```

默认会测试当前配置模型，以及模型名包含 `grok` 或 `search` 的候选模型。可选：

```bash
go run ./cmd/grok-diagnose -mode current     # 只测当前配置模型
go run ./cmd/grok-diagnose -mode all         # 测 /models 返回的全部模型
go run ./cmd/grok-diagnose -timeout 40s      # 调整单模型超时
```

## License

MIT
