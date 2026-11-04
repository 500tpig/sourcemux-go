# Smart answer and reasoning endpoints

`smart_answer` is a two-phase workflow:

1. Gather evidence with the existing `research_run` workflow.
2. Send the compact research pack to a configured OpenAI-compatible reasoning endpoint for final synthesis.

This keeps current-source collection separate from final reasoning.

For native xAI Responses API endpoints, the evidence phase can opt into both
web and X search:

```json
{
  "grokEndpoints": [
    {
      "name": "xai-search",
      "baseURL": "https://api.x.ai/v1",
      "apiKey": "sk-your-xai-key",
      "model": "grok-4.20-fast",
      "apiType": "responses",
      "sendSearchFlag": true,
      "responseTools": ["web_search", "x_search"]
    }
  ]
}
```

`responseTools` is only for the search endpoint. The final synthesis model still
belongs in `reasoningEndpoints[]`.

Heavy multi-agent search models such as `grok-4.20-multi-agent-xhigh` should
not be in the default search profile. Put search-capable multi-agent models in
`grokEndpoints[]` with `"profile": "heavy"`; `smart-answer` and `research`
default to `profile=auto`, which follows `searchPolicy.autoPreference`
(`intent-based`, `heavy-first`, or `default-first`) and safely falls back to
default when heavy is unavailable. `reasoningEndpoints[]` alone is only for
final synthesis and is not used by `web_search` or `research`.

## Why `reasoningEndpoints` are separate

`grokEndpoints[]` are search-capable endpoints used by `web_search` and `research_run`. A synthesis-only model such as DeepSeek Flash/Pro should not go there, because a successful response would short-circuit the source-first search fallback route.

Use `reasoningEndpoints[]` for final-answer models:

```json
{
  "grokEndpoints": [
    {
      "name": "sourcemux",
      "baseURL": "https://your-grok-compatible-endpoint.example/v1",
      "apiKey": "sk-your-grok-key",
      "model": "grok-4.20-fast",
      "sendSearchFlag": false
    },
    {
      "name": "grok-multi-agent-search",
      "baseURL": "https://your-grok-compatible-endpoint.example/v1",
      "apiKey": "sk-your-grok-key",
      "model": "grok-4.20-multi-agent-xhigh",
      "profile": "heavy",
      "sendSearchFlag": false
    }
  ],
  "reasoningEndpoints": [
    {
      "name": "deepseek-flash",
      "baseURL": "https://api.deepseek.com/v1",
      "apiKey": "sk-your-deepseek-key",
      "model": "deepseek-v4-flash"
    },
    {
      "name": "deepseek-pro",
      "baseURL": "https://api.deepseek.com/v1",
      "apiKey": "sk-your-deepseek-key",
      "model": "deepseek-v4-pro"
    },
    {
      "name": "grok-multi-agent-xhigh",
      "baseURL": "https://your-grok-compatible-endpoint.example/v1",
      "apiKey": "sk-your-grok-key",
      "model": "grok-4.20-multi-agent-xhigh"
    }
  ]
}
```

## CLI

```bash
./sourcemux smart-answer "Should I adopt this library?" \
  --depth standard \
  --profile auto \
  --reasoning-endpoint deepseek-flash
```

Use Pro for more complex synthesis without changing the config:

```bash
./sourcemux smart-answer "Compare these architecture options" \
  --depth deep \
  --profile auto \
  --reasoning-model deepseek-v4-pro \
  --json
```

Explicit heavy search profile:

```bash
./sourcemux search "Investigate this complex current topic" \
  --profile heavy \
  --fallback-after 180s \
  --timeout 300s \
  --json

./sourcemux search "ping" \
  --profile heavy \
  --grok-pool-timeout 0 \
  --no-fallback \
  --timeout 120s \
  --json
```

## MCP

Tool: `smart_answer`

Inputs:

| Parameter | Required | Notes |
| --- | :---: | --- |
| `query` | Yes | The question to answer. |
| `depth` | No | `quick`, `standard`, or `deep`. |
| `mode` | No | `fixed`（默认）或 `jev`；JEV 需要顶层 `jev` 连接配置。 |
| `profile` | No | Research search profile: `auto` (default), `default`, `heavy`, or another configured profile. |
| `platform` | No | Optional search focus, such as `GitHub, Reddit`. |
| `domains` | No | Domain allow-list for research. |
| `max_fetches` | No | Maximum high-signal URLs fetched by research. |
| `reasoning_endpoint` | No | Name from `reasoningEndpoints[]`. |
| `reasoning_model` | No | One-shot model override. |

Output includes the final answer, endpoint/model metadata, research depth, source count, high-signal URLs, and the underlying research pack in JSON mode.

JEV 模式下，研究和综合共享总截止时间；综合只允许引用 `research.jev.citations` 中已读过的来源。`unknown` 或 `insufficient` 状态会进入综合提示。综合失败时，CLI JSON 和 MCP 错误结果仍保留研究证据。详细配置见 [JEV 使用说明](JEV.md)。

JEV 研究按约 5,000 字符分页扫描正文，证据不足时继续读取后文；每来源累计保留最多 2,000 字符相关原文。最终综合使用与 JEV 相同的摘录和位置记录，不混入发现链接、搜索摘要及启发式事实。原文未读完或片段被省略时会说明限制。

## Error: no `reasoningEndpoints` configured

If you see:

```text
no reasoningEndpoints configured
```

add at least one `reasoningEndpoints[]` entry to the active `sourcemux.json`, then verify:

```bash
./sourcemux config list --json
```

Do not fix this by adding DeepSeek to `grokEndpoints[]`; that changes search routing semantics.
