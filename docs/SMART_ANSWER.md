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

Heavy multi-agent models such as `grok-4.20-multi-agent-xhigh` should not be in
the default search profile. Either put them in `reasoningEndpoints[]` for final
synthesis, or put them in `grokEndpoints[]` with `"profile": "heavy"` and select
them explicitly with `search --profile heavy`. For slow multi-agent search,
use `--fallback-after` to bound when SourceMux gives way to fallback providers,
or `--grok-pool-timeout 0 --no-fallback` to verify the Grok profile itself.

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
  --reasoning-endpoint deepseek-flash
```

Use Pro for more complex synthesis without changing the config:

```bash
./sourcemux smart-answer "Compare these architecture options" \
  --depth deep \
  --reasoning-model deepseek-v4-pro \
  --json
```

Explicit heavy search profile:

```bash
./sourcemux search "Investigate this complex current topic" \
  --profile heavy \
  --fallback-after 60s \
  --timeout 180s \
  --json

./sourcemux search "Investigate this complex current topic" \
  --profile xhigh \
  --grok-pool-timeout 0 \
  --no-fallback \
  --timeout 300s \
  --json
```

## MCP

Tool: `smart_answer`

Inputs:

| Parameter | Required | Notes |
| --- | :---: | --- |
| `query` | Yes | The question to answer. |
| `depth` | No | `quick`, `standard`, or `deep`. |
| `platform` | No | Optional search focus, such as `GitHub, Reddit`. |
| `domains` | No | Domain allow-list for research. |
| `max_fetches` | No | Maximum high-signal URLs fetched by research. |
| `reasoning_endpoint` | No | Name from `reasoningEndpoints[]`. |
| `reasoning_model` | No | One-shot model override. |

Output includes the final answer, endpoint/model metadata, research depth, source count, high-signal URLs, and the underlying research pack in JSON mode.

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
