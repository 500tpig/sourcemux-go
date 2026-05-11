package main

import (
	"fmt"
	"os"
	"strings"

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
	configPath, args, err := splitGlobalConfigArg(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "argument error: %v\n", err)
		os.Exit(2)
	}

	if len(args) > 0 && args[0] == "cli" {
		os.Exit(cli.RunWithConfig(args[1:], configPath))
	}

	cfg, err := config.LoadFile(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	if err := server.Run(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}

func splitGlobalConfigArg(args []string) (string, []string, error) {
	var out []string
	configPath := config.DefaultConfigPath()
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--config" || arg == "-c":
			if i+1 >= len(args) {
				return "", nil, fmt.Errorf("%s requires a path", arg)
			}
			if strings.TrimSpace(args[i+1]) == "" {
				return "", nil, fmt.Errorf("%s requires a non-empty path", arg)
			}
			configPath = args[i+1]
			i++
		case strings.HasPrefix(arg, "--config="):
			configPath = strings.TrimPrefix(arg, "--config=")
			if strings.TrimSpace(configPath) == "" {
				return "", nil, fmt.Errorf("--config requires a non-empty path")
			}
		default:
			out = append(out, arg)
		}
	}
	return configPath, out, nil
}
