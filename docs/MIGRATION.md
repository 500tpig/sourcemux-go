# Migration

## Product rename: grok-search -> SourceMux

SourceMux is the new public project name. The public GitHub repository is:

```text
https://github.com/500tpig/sourcemux-go
```

After renaming the GitHub repository, update existing local clones:

```bash
git remote set-url origin https://github.com/500tpig/sourcemux-go.git
```

If you need to develop or test from a checkout, build from source:

```bash
git clone https://github.com/500tpig/sourcemux-go.git
cd sourcemux-go
go build -o sourcemux .
```

Tagged SourceMux releases now exist; the current verified public baseline
(checked 2026-06-02) is `v0.2.1`. New `go install` installs should use:

```bash
go install github.com/500tpig/sourcemux-go/cmd/sourcemux@latest
```

The old `cmd/grok-search` entrypoint remains in the repository for one
migration window, but docs and release packaging treat `sourcemux` as the
primary command.

Default config is now `./sourcemux.json`. Existing local configs can be renamed:

```bash
mv grok-search.json sourcemux.json
```

Or kept in place by passing it explicitly:

```bash
sourcemux --config ./grok-search.json
sourcemux --config ./grok-search.json config list --json
```

The runtime still reads one explicit config file only; it does not auto-scan
legacy names or hidden config directories.

## Configuration migration

This document describes the v1 to v2 configuration transition.

## v1 compatibility

Existing `sourcemux.json` files with top-level fields such as
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

Keep Exa configured for the required `docs_search` provider when
`minimum_profile=standard` is enabled.

### Web fetch provider order

For v2 configs, `capabilities.web_fetch.providers` can provide an explicit
`sourcemux fetch --profile auto` order. When no explicit v2 order is present,
SourceMux uses policy-first routing: GitHub URLs use repository-aware
enrichment first, ordinary pages prefer Firecrawl when configured, and
`--profile cheap` is the Jina-first low-cost route. To pin a local auto order,
write it explicitly:

```json
{
  "version": 2,
  "minimum_profile": "off",
  "capabilities": {
    "main_search": {"providers": []},
    "docs_search": {"providers": []},
    "web_fetch": {
      "providers": [
        {
          "type": "firecrawl",
          "apiURL": "https://api.firecrawl.dev/v2",
          "keys": [
            {"name": "acct-a", "apiKey": "fc-your-key-a"},
            {"name": "acct-b", "apiKey": "fc-your-key-b"}
          ],
          "enabled": true
        },
        {"type": "jina", "apiURL": "https://r.jina.ai"}
      ]
    },
    "web_enhance": {"providers": []}
  }
}
```

Use `apiKey` for a single Firecrawl key, or `keys[]` for multiple named keys.
SourceMux rotates the starting Firecrawl key and tries the remaining keys when
a key fails or returns empty content. The top-level `firecrawl` block enables
direct `firecrawl-scrape` / `firecrawl-map` commands and policy-first ordinary
fetch when Firecrawl keys are configured.

## Explicit migration

Run:

```bash
sourcemux config migrate --json
```

The command:

- writes a backup to `<config>.bak` by default;
- rewrites the active config as v2 JSON;
- keeps file permissions at `0600`;
- does not print full API keys in stdout/stderr;
- sets migrated configs to `minimum_profile: "off"` to preserve existing
  behavior.

Use `sourcemux doctor --json` after migration for local-only structural
validation. `doctor` does not contact providers unless `--probe` is explicitly
passed.
