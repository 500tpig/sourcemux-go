package main

import (
	"os"

	"github.com/500tpig/sourcemux-go/internal/app"
)

// main routes between two execution modes:
//
//   - default: launch the MCP server on stdio (for Claude Code / Codex /
//     Cherry Studio MCP integrations).
//   - `sourcemux cli ...`: one-shot CLI mode that reuses the same engine
//     layer but emits human-readable text or JSON, suitable for shelling out
//     from external automation (e.g. notion-local-ops-mcp's run_command).
func main() {
	os.Exit(app.Run(os.Args[1:]))
}
