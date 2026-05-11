# Grok Search Go

CLI-first 的搜索 / 抓取 / research 工具，同时提供 MCP 接入给 Codex、Claude Code、Cherry Studio 等 LLM Client。**支持多 Grok 端点池 + 自动降级**。

推荐心智模型：

- **CLI 是主执行面**：适合本地 agent 高频调用、脚本、CI、可复现命令、排障和开源分发。
- **MCP 是客户端接入层**：适合 Codex / Claude 等客户端内直接调用工具；如果长会话里 MCP 连接变慢或不可用，直接切到同一个二进制的 CLI 模式。
- **单文件配置**：MCP 和 CLI 默认只读取当前目录的 `./grok-search.json`；需要放到别处时显式传 `--config`。

## 架构

```
Agent / Shell ──CLI──► grok-search cli <command>
LLM Client ────MCP──► Grok Search Server (same binary)
                       ├─ search / web_search  ─► Grok 端点池 → TinyFish Search → Exa Search → Tavily Search 兜底
                       ├─ fetch / web_fetch    ─► Jina Reader → TinyFish Fetch → Exa Contents → Tavily Extract
                       ├─ exa-search / exa_search_advanced
                       ├─ exa-contents / exa_contents_advanced
                       ├─ map / web_map
                       ├─ crawl / web_crawl
                       ├─ plan / search_planning
                       ├─ research / research_run（计划→搜索→取源→抓取→打包）
                       └─ smart-answer / smart_answer（research 取证→DeepSeek/推理端点综合）
```

## 特性

- 🚀 **单二进制部署** — 编译后无运行时依赖
- 🧰 **CLI-first** — `search` / `fetch` / `research` / `doctor` / `config` 都能作为普通命令运行，便于 agent 复现和人类排障
- 🧩 **MCP 集成** — 同一个二进制继续提供 `web_search` / `web_fetch` / `research_run` 等 MCP tools
- 🔍 **AI 驱动搜索** — Grok web 搜索 + 5 级 source URL 抽取（annotations / search_sources / citations / search_results / 正则）
- 🔄 **多端点池** — 一个 grok2api 挂了自动切到下一个，最后兜底 Tavily
- 🧭 **Exa 兜底** — 可选接入 Exa Search / Contents，适合本地 agent 的 source-first 检索与抓取
- 🧠 **高级 Exa 模式** — 显式暴露 Exa deep / deep-reasoning、output schema、subpages、maxAgeHours；不改变默认搜索/抓取路由
- 🐟 **TinyFish 多 key 兜底** — 可选接入 TinyFish Search / Fetch，多 key 轮询并在 429/错误时自动换 key
- 📥 **零成本抓取** — Jina Reader 免费、无需 key、Markdown 输出
- 🕸️ **站点级 Crawl** — Tavily Crawl 可遍历站点并返回抽取内容；`web_map` 只发现 URL，`web_crawl` 返回内容
- 🧩 **组合式 Research Run** — `research_run` / `cli research` 执行有界内存工作流，串联 `search_planning`、`web_search`、`get_sources`、`web_fetch`，必要时使用 `web_map` / `web_crawl`
- 🧠 **低成本智能综合** — `smart_answer` / `cli smart-answer` 保留现有搜索取证链路，再用 DeepSeek V4 Flash/Pro 等 OpenAI-compatible 推理端点输出最终答案
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

grok-search 现在只读一个 JSON 文件：

- 默认：当前工作目录的 `./grok-search.json`
- 显式指定：`grok-search --config /path/to/grok-search.json ...` 或 `grok-search cli --config /path/to/grok-search.json ...`
- 不读取环境变量配置链、不读取 `~/.config/grok-search/*`、不读取旧版 `endpoints.json`

真实配置里有 API key，`grok-search.json` 已加入 `.gitignore`，不要提交。

### setup：不手写 JSON

```bash
grok-search cli setup --non-interactive \
  --api-url "https://your-endpoint/v1" \
  --api-key "sk-..." \
  --model "grok-4.20-fast" \
  --json
```

默认写入 `./grok-search.json`，不会覆盖已有文件；需要重写时显式加 `--force`。想把配置放到别处：

```bash
grok-search cli --config /secure/path/grok-search.json setup --non-interactive \
  --api-url "https://your-endpoint/v1" \
  --api-key "sk-..." \
  --json
```

### 手写单文件

```bash
cat > grok-search.json <<'JSON'
{
  "grokEndpoints": [
    {"name":"wykon","baseURL":"https://grok2api.wykon.homes/v1","apiKey":"sk-...","model":"grok-4.20-fast","sendSearchFlag":false},
    {"name":"yyds","baseURL":"https://yyds.215.im/v1","apiKey":"sk-...","model":"grok-4.20-fast","sendSearchFlag":false}
  ],
  "reasoningEndpoints": [
    {"name":"deepseek-flash","baseURL":"https://api.deepseek.com/v1","apiKey":"sk-...","model":"deepseek-v4-flash"},
    {"name":"deepseek-pro","baseURL":"https://api.deepseek.com/v1","apiKey":"sk-...","model":"deepseek-v4-pro"}
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
chmod 600 grok-search.json
```

### 检查配置

```bash
grok-search cli config path
grok-search cli config files --json
grok-search cli config list --json
grok-search cli doctor --json
```

`config list` 只打印 masked key status，不打印完整 API key。`config files` 只说明当前唯一配置文件；历史 `~/.config/grok-search/config.json`、`~/.config/grok-search/endpoints.json`、备份和日志都不会被读取。

### 字段说明

| 字段 | 必填 | 说明 |
|------|:---:|------|
| `grokEndpoints[]` | 否 | Grok/OpenAI-compatible 端点池，按顺序尝试；如果只用 Exa/Tavily/Jina，也可以为空 |
| `grokEndpoints[].baseURL` | ✅ | OpenAI-compatible 根路径；缺 `/v1` 时会自动补齐 |
| `grokEndpoints[].apiKey` | ✅ | Bearer token |
| `grokEndpoints[].model` | 否 | 默认 `grok-3-mini` |
| `grokEndpoints[].sendSearchFlag` | 否 | xAI 原生通常为 `true`；多数 grok2api 代理建议 `false` |
| `grokEndpoints[].apiType` | 否 | `chat` 或 `responses` |
| `reasoningEndpoints[]` | 否 | 最终综合/推理端点池，OpenAI-compatible Chat Completions；不参与 `web_search` 路由 |
| `reasoningEndpoints[].baseURL` | ✅ | OpenAI-compatible 根路径；缺 `/v1` 时会自动补齐 |
| `reasoningEndpoints[].apiKey` | ✅ | Bearer token |
| `reasoningEndpoints[].model` | 否 | 默认 `deepseek-v4-flash` |
| `tavily` | 否 | Tavily Search / Extract / Map / Crawl 配置 |
| `exa` | 否 | Exa Search / Contents 配置 |
| `jina` | 否 | Jina Reader 配置；无 key 也可用 |
| `tinyfish` | 否 | TinyFish Search / Fetch 生产兜底配置 |
| `grokPoolTimeoutSec` | 否 | Grok endpoint pool 总超时；`0` 或省略表示不额外限制 |
| `logLevel` | 否 | 默认 `INFO` |

### 旧配置怎么办？

如果你以前有 `/Users/.../.config/grok-search/config.json` 或 `endpoints.json`，新版本不会读取它们。建议手动把需要的 endpoint/provider 字段复制到项目的 `./grok-search.json`，确认 `grok-search cli config list --json` 正常后，旧目录可以留着备份或自行删除。

最小可用配置只需要 Grok endpoint；建议先从 endpoint 的 `/models` 返回列表里选模型：

- 默认推荐：`grok-4.20-fast`（搜索、摘要、日常 research）
- 高质量兜底：`grok-4.20-reasoning`（复杂归纳、长链路分析）
- 兼容兜底：`grok-3-mini`（旧端点默认值）

如果使用 xAI 原生端点通常需要 `sendSearchFlag: true`；多数 grok2api / OpenAI-compatible 代理建议 `false`。

### DeepSeek 智能综合配置

如果你想用免费/低价 Grok 账号负责搜索，再用 DeepSeek 做最终推理，添加独立的 `reasoningEndpoints`，不要把 DeepSeek 放进 `grokEndpoints`：

```json
{
  "grokEndpoints": [
    {"name":"grok2api-fast","baseURL":"https://your-grok2api/v1","apiKey":"sk-...","model":"grok-4.20-fast","sendSearchFlag":false}
  ],
  "reasoningEndpoints": [
    {"name":"deepseek-flash","baseURL":"https://api.deepseek.com/v1","apiKey":"sk-...","model":"deepseek-v4-flash"},
    {"name":"deepseek-pro","baseURL":"https://api.deepseek.com/v1","apiKey":"sk-...","model":"deepseek-v4-pro"}
  ]
}
```

使用方式：

```bash
./grok-search cli smart-answer "我应该买 SuperGrok 还是接 DeepSeek？" \
  --depth standard --reasoning-endpoint deepseek-flash

./grok-search cli smart-answer "复杂技术决策" \
  --depth deep --reasoning-model deepseek-v4-pro --json
```

心智模型：`grokEndpoints` 是“眼睛”（搜索/找来源），`reasoningEndpoints` 是“大脑”（基于 research pack 综合推理）。

这样 Claude Code / Cherry Studio 那边的 MCP 注册不需要传 env；如果 MCP 进程工作目录不是配置所在目录，就显式传 `--config`：

```bash
claude mcp add-json grok-search '{
  "type": "stdio",
  "command": "/path/to/grok-search",
  "args": ["--config", "/path/to/grok-search.json"]
}'
```

### 添加到 Codex

Codex 推荐走 MCP。先编译二进制：

```bash
go build -o grok-search .
```

然后在 Codex 配置中添加 stdio MCP server。建议显式传配置文件绝对路径，避免 MCP 进程工作目录不确定：

```bash
codex mcp add-json grok-search '{
  "type": "stdio",
  "command": "/Users/johnsmith/Project/Study/grok-search-go/grok-search",
  "args": ["--config", "/Users/johnsmith/Project/Study/grok-search-go/grok-search.json"]
}'
```

本仓库同时提供项目级 skill：`.codex/skills/grok-search-mcp/SKILL.md`。它只负责告诉 Codex 什么时候、怎样调用 MCP；真正的搜索/抓取仍由 MCP server 执行。

配置完成后，在 Codex / Claude 中调用 `get_config_info`，确认 endpoint 探活成功。

## CLI 模式（无需 MCP）

同一个二进制还提供 `grok-search cli <subcommand>` 子命令，方便脚本、CI、或者其它没法接 stdio MCP 的 agent 直接调用。和 MCP 模式共用一份 engine 代码 + 同一个 `grok-search.json`。

```bash
./grok-search cli config path                                             # 查看配置路径
./grok-search cli --config /path/to/grok-search.json config path --json   # 显式指定配置文件
./grok-search cli config files --json                                     # 查看唯一配置文件/加载说明
./grok-search cli config list --json                                      # 查看脱敏后的生效配置
./grok-search cli setup --non-interactive \
  --api-url "https://your-endpoint/v1" --api-key "sk-..." --json          # 写入 ./grok-search.json
./grok-search cli doctor --json                                           # 端点探活 + 配置概览
./grok-search cli search "X 上 grok 4.20 的最新评价" --platform Twitter --json
./grok-search cli search "被 CF 墙后的某产品评测" --model grok-4.20-fast
./grok-search cli fetch  "https://example.com/article" --json             # Jina, Tavily 兑底
./grok-search cli exa-search "最新 AI 芯片发布" \
  --type deep --output-schema-json '{"type":"object","properties":{"answer":{"type":"string"}}}' --json
./grok-search cli exa-contents "https://example.com/docs" \
  --subpages 3 --subpage-target api --max-age-hours 0 --json
./grok-search cli map    "https://example.com" --limit 50 --json          # 需 tavily.apiKey
./grok-search cli crawl  "https://example.com/docs" \
  --instructions "Find API reference pages" --limit 10 --json             # 需 tavily.apiKey
./grok-search cli plan   "调研主题" --depth deep                          # 离线计划，不打网络
./grok-search cli research "调研主题" \
  --depth deep --domain example.com --max-fetches 6 --json                # 执行搜索/抓取并输出 research pack
./grok-search cli smart-answer "调研主题" \
  --depth standard --reasoning-endpoint deepseek-flash --json             # research 取证后交给推理端点综合
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
| `doctor` | 配置 + `/models` 探活，新手优先用这个 | `--list-timeout` `--preview` `--json` |
| `probe` | `doctor` 的兼容旧名 | `--list-timeout` `--preview` `--json` |
| `config path` | 显示当前唯一配置文件路径、绝对路径和存在状态 | `--json` |
| `config files` | 显示当前唯一配置文件和加载说明，不读取历史隐藏配置 | `--json` |
| `config list` | 显示脱敏后的生效配置，不探活、不打网络 | `--json` |
| `setup` | 写入当前 `grok-search.json`，避免手写 JSON；默认不覆盖已有文件 | `--non-interactive` `--api-url` `--api-key` `--model` `--api-type` `--send-search-flag` `--tavily-key` `--exa-key` `--jina-key` `--tinyfish-keys` `--force` `--json` |
| `plan <query>` | 纯逻辑（不调网络）| `--depth` `--platform` |
| `research <query>` | 组合式执行：规划查询 → 多轮搜索 → 取 sources → 去重排序 → 抓取 top-N → 输出 research pack | `--depth` `--platform` `--domain` `--max-fetches` `--timeout` `--json` |
| `smart-answer <query>` | 组合式 research 取证 → DeepSeek/推理端点综合输出最终答案 | `--depth` `--platform` `--domain` `--max-fetches` `--reasoning-endpoint` `--reasoning-model` `--timeout` `--json` |
| `tinyfish-bench` | TinyFish Search / Fetch / Agent 本地评测 | `--cases` `--keys-file` `--surfaces` `--timeout` `--json` |

`map` 和 `crawl` 的区别：

- `map` 只做站点 URL 发现，输出 URL 列表，适合先判断站点结构。
- `crawl` 会遍历站点并抽取每个页面的 `raw_content`，适合深度研究、文档打包、下游 LLM 读取。

`research` 和相邻命令的区别：

- `search` 只跑一次搜索并返回答案/信源；`research` 会先规划多个查询，再汇总多个搜索 session 的来源。
- `fetch` 只抓取一个 URL；`research` 会对候选 URL 去重、启发式排序后抓取 top-N。
- `crawl` 是站点遍历能力；`research` 只在指定 `--domain` 且有用时复用现有 Tavily Map/Crawl，不重新实现 crawl provider。
- `max_fetches` / `--max-fetches` 是执行上限，v1 会限制在最多 12 个 URL，避免 research pack 过大。
- `smart-answer` 在 `research` 之后多做一步：把 compact research pack 交给 `reasoningEndpoints`，适合低成本提升最终推理质量。

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
- 配置与 MCP 一致：默认读取 `./grok-search.json`，或用全局 `--config` 显式指定单个 JSON 文件。
- Codex 用户可直接 `claude mcp add grok-search ...`（MCP）+ `./grok-search cli ...`（CLI）双管齐下；CLI 模式对应 skill 见 `.codex/skills/grok-search-cli/SKILL.md`。

### TinyFish 本地 benchmark

`tinyfish-bench` 是隔离评测工具，只调用 TinyFish REST Search / Fetch / sync Agent API；生产 MCP `web_search` / `web_fetch` 是否使用 TinyFish 由上面的 `tinyfish.enabled` 配置决定。评测密钥建议放到单独的本地临时文件（不要提交）：

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
| `smart_answer` | 先执行组合式 research 取证，再用配置的 DeepSeek/推理端点综合最终答案 |

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

`smart_answer` 参数：

| 参数 | 必填 | 说明 |
|------|------|------|
| `query` | ✅ | 要回答的问题 |
| `depth` | 否 | research 深度：`quick` / `standard` / `deep`，默认 `standard` |
| `platform` | 否 | 聚焦平台 |
| `domains` | 否 | 域名/站点 allow-list，传给 research 阶段 |
| `max_fetches` | 否 | research 阶段最多抓取的高信号 URL 数 |
| `reasoning_endpoint` | 否 | 指定 `reasoningEndpoints[].name`，如 `deepseek-flash` |
| `reasoning_model` | 否 | 按次覆盖推理模型，如 `deepseek-v4-pro` |

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
| `apiType` | 否 | `chat` | `chat` 使用 `/v1/chat/completions`；`responses` 使用 `/v1/responses` |

## 推理端点字段

| 字段 | 必填 | 默认 | 说明 |
|------|------|------|------|
| `name` | 否 | `reasoning-N` | 显示名，可被 `smart_answer.reasoning_endpoint` / `--reasoning-endpoint` 选择 |
| `baseURL` | ✅ | — | OpenAI-compatible 根路径；缺 `/v1` 时会自动补齐 |
| `apiKey` | ✅ | — | Bearer token |
| `model` | 否 | `deepseek-v4-flash` | 默认综合模型 |

## 超时与限额

| 配置字段 | 默认 | 说明 |
|------|------|------|
| `grokPoolTimeoutSec` | 未设 / `0` | GrokPool 全局 wall-clock 预算（秒）。`>0` 时所有端点 + 重试合计不会超过该预算，避免多个端点均慢时堆叠到 `端点数 × MaxAttempts × MaxDelay`。推荐 `30` ～ `90`。 |
| `exa.apiKey` | 未设 | 可选。开启 Exa Search / Contents 作为 Grok 后、Tavily 前的 source-first fallback。 |
| `exa.apiURL` | `https://api.exa.ai` | 可选。Exa API 根路径。 |
| `exa.enabled` | `true` | 可选。设为 `false` 可临时关闭 Exa。 |

## 本地诊断

检查每个 endpoint 的 `/models` 和实际 chat/search 可用性：

```bash
go run ./cmd/grok-diagnose -config ./grok-search.json
```

默认会测试当前配置模型，以及模型名包含 `grok` 或 `search` 的候选模型。可选：

```bash
go run ./cmd/grok-diagnose -config ./grok-search.json -mode current     # 只测当前配置模型
go run ./cmd/grok-diagnose -config ./grok-search.json -mode all         # 测 /models 返回的全部模型
go run ./cmd/grok-diagnose -config ./grok-search.json -timeout 40s      # 调整单模型超时
```

## License

MIT
