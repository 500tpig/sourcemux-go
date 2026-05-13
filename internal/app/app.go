package app

import (
	"fmt"
	"os"
	"strings"

	"github.com/500tpig/grok-search-go/internal/cli"
	"github.com/500tpig/grok-search-go/internal/config"
	"github.com/500tpig/grok-search-go/internal/server"
)

// Run routes between MCP stdio server mode and the one-shot CLI mode.
func Run(args []string) int {
	configPath, args, err := SplitGlobalConfigArg(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "argument error: %v\n", err)
		return 2
	}

	if len(args) > 0 && args[0] == "cli" {
		return cli.RunWithConfig(args[1:], configPath)
	}

	cfg, err := config.LoadFile(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		return 1
	}

	if err := server.Run(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		return 1
	}
	return 0
}

func SplitGlobalConfigArg(args []string) (string, []string, error) {
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
