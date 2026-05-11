# Security Policy

## Supported versions

Security fixes target the `main` branch until versioned releases are published.

## Reporting a vulnerability

Please report suspected vulnerabilities privately by opening a GitHub security advisory if the repository has advisories enabled. If advisories are not available, contact the maintainer through the repository owner profile.

Do not include live API keys, private provider endpoints, or production config files in public issues.

## Secret handling

- Runtime credentials belong in `grok-search.json` or another explicitly selected local config file.
- `grok-search.json`, `.env`, and `config.local.json` are ignored by Git.
- Example config files must use placeholder keys and safe example endpoints only.
- Diagnostic commands should print masked key status only.
- Upstream error bodies that may echo configured secrets should be redacted before being surfaced.

## Network behavior

This project calls configured third-party search, fetch, crawl, and reasoning APIs. Review the configured providers before using the tool with sensitive queries or internal URLs.
