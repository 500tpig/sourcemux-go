package app

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/500tpig/sourcemux-go/internal/cli"
	"github.com/500tpig/sourcemux-go/internal/config"
	"github.com/500tpig/sourcemux-go/internal/install"
	"github.com/500tpig/sourcemux-go/internal/server"
)

type VersionInfo struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
}

var buildInfo = VersionInfo{Version: "dev", Commit: "none", Date: "unknown"}

const usage = `Usage: sourcemux [--config <path>] <command>

Commands:
  setup               Create sourcemux.json without hand-editing JSON.
  doctor              Check config locally; use --probe for live provider probes.
  search <query>      Run a web search through configured fallbacks.
  fetch <url>         Fetch a URL as Markdown through configured fallbacks.
  docs-search <query> Search documentation/source pages.
  research <query>    Run a bounded research workflow.
  config <command>    Inspect the active config path or masked config.
  bootstrap <target>  Install or update generated agent skills/MCP config.
  cli <command>       Compatibility path for one-shot CLI commands.
  install <target>    Compatibility path for bootstrap.
  uninstall <target>  Remove generated SourceMux skills/MCP config.
  version             Print version information.

Without a command, sourcemux starts the stdio MCP server and reads the selected
config file. Use --config when the config is not ./sourcemux.json.
`

func SetVersionInfo(version, commit, date string) {
	buildInfo = VersionInfo{
		Version: stringOr(version, "dev"),
		Commit:  stringOr(commit, "none"),
		Date:    stringOr(date, "unknown"),
	}
}

// Run routes between MCP stdio server mode and the one-shot CLI mode.
func Run(args []string) int {
	configPath, args, err := SplitGlobalConfigArg(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "argument error: %v\n", err)
		return 2
	}

	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help") {
		fmt.Fprint(os.Stdout, usage)
		return 0
	}
	if len(args) > 0 && args[0] == "cli" {
		return cli.RunWithConfig(args[1:], configPath)
	}
	if len(args) > 0 && isTopLevelCLICommand(args[0]) {
		return cli.RunWithConfig(args, configPath)
	}
	if len(args) > 0 && args[0] == "bootstrap" {
		return install.RunInstall(args[1:], configPath)
	}
	if len(args) > 0 && args[0] == "install" {
		return install.RunInstall(args[1:], configPath)
	}
	if len(args) > 0 && args[0] == "uninstall" {
		return install.RunUninstall(args[1:], configPath)
	}
	if len(args) > 0 && args[0] == "version" {
		return printVersion(args[1:])
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

func isTopLevelCLICommand(cmd string) bool {
	switch cmd {
	case "search",
		"docs-search",
		"fetch",
		"exa-search",
		"exa-contents",
		"map",
		"crawl",
		"doctor",
		"probe",
		"config",
		"setup",
		"smoke",
		"plan",
		"research",
		"smart-answer",
		"tinyfish-bench":
		return true
	default:
		return false
	}
}

func printVersion(args []string) int {
	asJSON := false
	for _, arg := range args {
		switch arg {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintln(os.Stdout, "Usage: sourcemux version [--json]")
			return 0
		default:
			fmt.Fprintf(os.Stderr, "unknown version flag %q\n", arg)
			return 2
		}
	}
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(buildInfo)
		return 0
	}
	fmt.Fprintf(os.Stdout, "sourcemux %s (commit=%s date=%s)\n", buildInfo.Version, buildInfo.Commit, buildInfo.Date)
	return 0
}

func stringOr(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
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
