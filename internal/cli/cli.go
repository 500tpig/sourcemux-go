// Package cli exposes the sourcemux engine layer (Grok pool, Jina Reader,
// Tavily) as a non-MCP one-shot CLI. It is invoked via `sourcemux cli
// <subcommand> [flags]` and mirrors the MCP tool surface: search / fetch /
// map / crawl / doctor / probe / config / setup / plan / research /
// smart-answer.
//
// Design notes:
//
//   - The CLI calls engine.* directly; it deliberately does NOT go through
//     internal/tools because those are tightly bound to mark3labs/mcp-go's
//     CallToolRequest. Sharing engine.* keeps both surfaces honest about a
//     single source of behavior.
//   - Every subcommand supports --json so callers can parse output reliably.
//   - Run never panics; it returns a Unix-style exit code (0=ok, 1=runtime
//     failure, 2=usage error, 3=config gate) and lets main.go translate to os.Exit.
package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/500tpig/sourcemux-go/internal/config"
)

const usage = `Usage: sourcemux cli <command> [flags]

Commands:
  search <query>      Run a web search through Grok/TinyFish/Exa/Tavily fallbacks.
  docs-search <query> Search docs through Exa docs/web search fallback.
  fetch  <url>        Fetch a URL as Markdown (Jina/TinyFish/Exa/Tavily fallbacks).
  exa-search <query>  Run Exa Search directly with advanced Exa-only options.
  exa-contents <url>  Run Exa Contents directly with advanced Exa-only options.
  map    <url>        Discover URLs on a site (Tavily Map; needs tavily.apiKey).
  crawl  <url>        Crawl a site and extract content (Tavily Crawl; needs tavily.apiKey).
  doctor              Check config locally; use --probe for live provider probes.
  probe               Show config and probe each Grok endpoint (/models).
  config <command>    Inspect the single active config file or masked effective config.
  setup               Write sourcemux.json without hand-editing JSON.
  plan   <query>      Print a deterministic multi-step search plan.
  research <query>    Run a composable in-memory research workflow (use --profile for the search pool).
  smart-answer <query>
                      Research sources, then synthesize with a reasoning endpoint.
  tinyfish-bench      Benchmark TinyFish Search, Fetch, and Agent locally.

Common flags (subcommand-dependent):
  --config <path>     Use one explicit config file (default: ./sourcemux.json).
  --json              Emit machine-readable JSON instead of human text.
  --platform <name>   Focus a platform, e.g. 'Twitter' or 'GitHub, Reddit'.
                      Useful for content blocked by CF or hosted on X.
  --model <name>      One-shot Grok model override, e.g. 'grok-4.20-fast'.
  --profile <name>    Use a named Grok endpoint profile, e.g. 'heavy'.
  --timeout <dur>     Per-call timeout, e.g. '60s', '2m'.
  --grok-pool-timeout <dur>
                      Override configured Grok pool cap; 0 disables it.
  --fallback-after <dur>
                      Alias for --grok-pool-timeout on search.
  --no-fallback       Search only the selected Grok pool; use for diagnostics, not normal research.
  --help, -h          Show this usage.

Examples:
  sourcemux cli search "X 上 grok 4 的最新评价" --platform Twitter --json
  sourcemux cli search "复杂搜索问题" --profile heavy --fallback-after 60s --timeout 180s --json
  sourcemux cli search "ping" --profile heavy --grok-pool-timeout 0 --no-fallback --timeout 120s --json
  sourcemux cli docs-search "middleware auth" --json
  sourcemux cli fetch  "https://example.com/article" --json
  sourcemux cli exa-search "latest AI chip launches" --type deep --output-schema-json '{"type":"object"}' --json
  sourcemux cli exa-contents "https://example.com/docs" --subpages 3 --subpage-target api --json
  sourcemux cli crawl  "https://example.com/docs" --instructions "Find API pages" --limit 10 --json
  sourcemux cli doctor --json
  sourcemux cli config path
  sourcemux cli config files --json
  sourcemux cli config list --json
  sourcemux cli --config ./prod.sourcemux.json config list --json
  sourcemux cli setup --non-interactive --api-url https://your-endpoint/v1 --api-key sk-... --json
  sourcemux cli probe  --json
  sourcemux cli plan   "Notion AI agents" --depth deep
  sourcemux cli research "Notion AI agents" --depth deep --profile heavy --domain example.com --max-fetches 6 --json
  sourcemux cli smart-answer "Should I use SuperGrok or DeepSeek?" --reasoning-model deepseek-v4-flash --json
  sourcemux cli tinyfish-bench --cases docs/tinyfish-benchmark-cases.sample.json --json
`

// Run dispatches the cli subcommand tree. args is everything after the
// leading "cli" token (so args[0] is the subcommand name).
func Run(args []string) int {
	return RunWithConfig(args, config.DefaultConfigPath())
}

func RunWithConfig(args []string, configPath string) int {
	var err error
	configPath, args, err = splitCLIConfigArg(args, configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "argument error: %v\n", err)
		return 2
	}
	previous := activeConfigPath
	activeConfigPath = configPath
	defer func() { activeConfigPath = previous }()

	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprint(os.Stdout, usage)
		return 0
	}

	cmd, rest := args[0], args[1:]
	switch cmd {
	case "search":
		return runSearch(rest)
	case "docs-search":
		return runDocsSearch(rest)
	case "fetch":
		return runFetch(rest)
	case "exa-search":
		return runExaSearch(rest)
	case "exa-contents":
		return runExaContents(rest)
	case "map":
		return runMap(rest)
	case "crawl":
		return runCrawl(rest)
	case "doctor":
		return runDoctor(rest)
	case "probe":
		return runProbe(rest)
	case "config":
		return runConfig(rest)
	case "setup":
		return runSetup(rest)
	case "smoke":
		return runSmoke(rest)
	case "plan":
		return runPlan(rest)
	case "research":
		return runResearch(rest)
	case "smart-answer":
		return runSmartAnswer(rest)
	case "tinyfish-bench":
		return runTinyFishBench(rest)
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q\n\n%s", cmd, usage)
		return 2
	}
}

var activeConfigPath = config.DefaultConfigPath()

func loadConfig() (*config.Config, error) {
	return config.LoadFile(activeConfigPath)
}

func currentConfigPath() string {
	if strings.TrimSpace(activeConfigPath) == "" {
		return config.DefaultConfigPath()
	}
	return activeConfigPath
}

func splitCLIConfigArg(args []string, fallback string) (string, []string, error) {
	if strings.TrimSpace(fallback) == "" {
		fallback = config.DefaultConfigPath()
	}
	var out []string
	configPath := fallback
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
