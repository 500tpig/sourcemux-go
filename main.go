package main

import (
	"fmt"
	"os"

	"github.com/bettas/grok-search-go/internal/cli"
	"github.com/bettas/grok-search-go/internal/config"
	"github.com/bettas/grok-search-go/internal/server"
)

// main routes between two execution modes:
//
//   - default: launch the MCP server on stdio (for Claude Code / Codex /
//     Cherry Studio MCP integrations).
//   - `grok-search cli ...`: one-shot CLI mode that reuses the same engine
//     layer but emits human-readable text or JSON, suitable for shelling out
//     from external automation (e.g. notion-local-ops-mcp's run_command).
func main() {
	if len(os.Args) > 1 && os.Args[1] == "cli" {
		os.Exit(cli.Run(os.Args[2:]))
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	if err := server.Run(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}
