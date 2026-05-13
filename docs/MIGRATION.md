# Configuration migration

This document describes the v1 to v2 configuration transition.

## v1 compatibility

Existing `grok-search.json` files with top-level fields such as
`grokEndpoints`, `reasoningEndpoints`, `exa`, `jina`, `tavily`, and `tinyfish`
continue to load. They are treated as legacy v1 configs and are mapped into the
runtime provider view in memory.

Legacy v1 configs default to `minimum_profile: "off"` so existing search/fetch
workflows do not suddenly fail because optional providers are missing.

## v2 capabilities

v2 config files use an explicit `capabilities` envelope:

```json
{
  "version": 2,
  "minimum_profile": "standard",
  "capabilities": {
    "main_search": {
      "providers": [
        {
          "type": "grok-pool",
          "name": "grok-pool",
          "endpoints": [
            {
              "name": "primary",
              "baseURL": "https://your-endpoint.example/v1",
              "apiKey": "sk-your-key",
              "model": "grok-4.20-fast"
            }
          ]
        }
      ]
    },
    "docs_search": {
      "providers": [
        {
          "type": "exa",
          "name": "exa-main",
          "apiURL": "https://api.exa.ai",
          "apiKey": "exa-your-key"
        },
        {
          "type": "context7",
          "name": "context7-main",
          "apiURL": "https://context7.com",
          "apiKey": "ctx7sk-your-key",
          "library_scopes": ["/vercel/*", "/facebook/*"]
        }
      ]
    },
    "web_fetch": {
      "providers": [
        {
          "type": "jina",
          "name": "jina-reader",
          "apiURL": "https://r.jina.ai"
        }
      ]
    },
    "web_enhance": {
      "providers": []
    }
  }
}
```

Do not mix v1 provider fields with the v2 `capabilities` block in the same
file. Mixed configs fail loudly instead of silently reordering providers.

`Context7` is optional and does not satisfy `minimum_profile=standard` by
itself; keep Exa configured for the required `docs_search` provider.

## Explicit migration

Run:

```bash
grok-search cli config migrate --json
```

The command:

- writes a backup to `<config>.bak` by default;
- rewrites the active config as v2 JSON;
- keeps file permissions at `0600`;
- does not print full API keys in stdout/stderr;
- sets migrated configs to `minimum_profile: "off"` to preserve existing
  behavior.

Use `grok-search cli doctor --json` after migration for local-only structural
validation. `doctor` does not contact providers unless `--probe` is explicitly
passed.
