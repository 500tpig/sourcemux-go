# Smart answer and reasoning endpoints

`smart_answer` is a two-phase workflow:

1. Gather evidence with the existing `research_run` workflow.
2. Send the compact research pack to a configured OpenAI-compatible reasoning endpoint for final synthesis.

This keeps current-source collection separate from final reasoning.

## Why `reasoningEndpoints` are separate

`grokEndpoints[]` are search-capable endpoints used by `web_search` and `research_run`. A synthesis-only model such as DeepSeek Flash/Pro should not go there, because a successful response would short-circuit the source-first search fallback route.

Use `reasoningEndpoints[]` for final-answer models:

```json
{
  "grokEndpoints": [
    {
      "name": "grok-search",
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
    }
  ]
}
```

## CLI

```bash
./grok-search cli smart-answer "Should I adopt this library?" \
  --depth standard \
  --reasoning-endpoint deepseek-flash
```

Use Pro for more complex synthesis without changing the config:

```bash
./grok-search cli smart-answer "Compare these architecture options" \
  --depth deep \
  --reasoning-model deepseek-v4-pro \
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

add at least one `reasoningEndpoints[]` entry to the active `grok-search.json`, then verify:

```bash
./grok-search cli config list --json
```

Do not fix this by adding DeepSeek to `grokEndpoints[]`; that changes search routing semantics.
