# Uninstall and migration

SourceMux separates agent integration cleanup from binary/config cleanup:

* `sourcemux uninstall ...` removes SourceMux-generated agent routing skills and,
  with `--write-config`, removes only the `sourcemux` MCP entry from supported
  client config files.
* Your package manager or shell removes the binary.
* You manually decide whether to keep or delete `sourcemux.json`, because it can
  contain API keys.

## 1. Inspect what SourceMux installed

User scope:

```bash
sourcemux bootstrap status --scope user --config-status --json
```

Project scope:

```bash
sourcemux bootstrap status --scope project --config-status --json
```

## 2. Remove generated agent skills and MCP config entries

Dry-run first:

```bash
sourcemux uninstall --all --scope user --write-config --dry-run --json
sourcemux uninstall --all --scope project --write-config --dry-run --json
```

Then remove:

```bash
sourcemux uninstall --all --scope user --write-config
sourcemux uninstall --all --scope project --write-config
```

If a previous install did not have a `.sourcemux-install.json` manifest, or if
you edited the generated skill, SourceMux refuses to delete it by default. Use
`--force` to create a timestamped backup and remove it:

```bash
sourcemux uninstall --all --scope user --write-config --force
sourcemux uninstall --all --scope project --write-config --force
```

`--write-config` removes only the `sourcemux` MCP entry. It does not delete the
whole Codex/Gemini/OpenCode config file and it preserves unrelated MCP entries.

## 3. Remove the binary

Homebrew cask:

```bash
brew uninstall --cask sourcemux
brew untap 500tpig/tap
```

Scoop:

```powershell
scoop uninstall sourcemux
scoop bucket rm 500tpig
```

`go install`:

```bash
rm -f "$(go env GOPATH)/bin/sourcemux" "$(go env GOPATH)/bin/grok-search"
```

Manual binary copy:

```bash
rm -f /usr/local/bin/sourcemux /usr/local/bin/grok-search
```

Adjust the path if you copied the binary somewhere else.

## 4. Decide what to do with config files

Do not delete config automatically if it may contain keys you still need. Common
locations are:

```bash
rm -f ./sourcemux.json
rm -f ~/.config/sourcemux/sourcemux.json
```

Only run those commands if you intentionally want to delete the stored provider
keys.

## 5. Reinstall with the public user flow

After installing the new binary:

```bash
sourcemux --config ~/.config/sourcemux/sourcemux.json setup
sourcemux --config ~/.config/sourcemux/sourcemux.json doctor --json
sourcemux --config ~/.config/sourcemux/sourcemux.json search "latest AI news" --json
sourcemux bootstrap codex --scope user
```

Use `--write-config` only if you want SourceMux to merge supported MCP client
config files:

```bash
sourcemux bootstrap codex --scope user --write-config
```

For source checkout development, use project scope instead:

```bash
./sourcemux bootstrap codex --scope project --binary "$(pwd)/sourcemux"
```
