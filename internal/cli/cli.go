// Package cli exposes the grok-search engine layer (Grok pool, Jina Reader,
// Tavily) as a non-MCP one-shot CLI. It is invoked via `grok-search cli
// <subcommand> [flags]` and mirrors the MCP tool surface: search / fetch /
// map / crawl / probe / plan / research.
//
// Design notes:
//
//   - The CLI calls engine.* directly; it deliberately does NOT go through
//     internal/tools because those are tightly bound to mark3labs/mcp-go's
//     CallToolRequest. Sharing engine.* keeps both surfaces honest about a
//     single source of behavior.
//   - Every subcommand supports --json so callers can parse output reliably.
//   - Run never panics; it returns a Unix-style exit code (0=ok, 1=runtime
//     failure, 2=usage error) and lets main.go translate to os.Exit.
package cli

import (
	"fmt"
	"os"
)

const usage = `Usage: grok-search cli <command> [flags]

Commands:
  search <query>      Run a web search through Grok/TinyFish/Exa/Tavily fallbacks.
  fetch  <url>        Fetch a URL as Markdown (Jina/TinyFish/Exa/Tavily fallbacks).
  exa-search <query>  Run Exa Search directly with advanced Exa-only options.
  exa-contents <url>  Run Exa Contents directly with advanced Exa-only options.
  map    <url>        Discover URLs on a site (Tavily Map; needs TAVILY_API_KEY).
  crawl  <url>        Crawl a site and extract content (Tavily Crawl; needs TAVILY_API_KEY).
  probe               Show config and probe each Grok endpoint (/models).
  plan   <query>      Print a deterministic multi-step search plan.
  research <query>    Run a composable in-memory research workflow.
  tinyfish-bench      Benchmark TinyFish Search, Fetch, and Agent locally.

Common flags (subcommand-dependent):
  --json              Emit machine-readable JSON instead of human text.
  --platform <name>   Focus a platform, e.g. 'Twitter' or 'GitHub, Reddit'.
                      Useful for content blocked by CF or hosted on X.
  --model <name>      One-shot Grok model override, e.g. 'grok-4.20-fast'.
  --timeout <dur>     Per-call timeout, e.g. '60s', '2m'.
  --help, -h          Show this usage.

Examples:
  grok-search cli search "X 上 grok 4 的最新评价" --platform Twitter --json
  grok-search cli fetch  "https://example.com/article" --json
  grok-search cli exa-search "latest AI chip launches" --type deep --output-schema-json '{"type":"object"}' --json
  grok-search cli exa-contents "https://example.com/docs" --subpages 3 --subpage-target api --json
  grok-search cli crawl  "https://example.com/docs" --instructions "Find API pages" --limit 10 --json
  grok-search cli probe  --json
  grok-search cli plan   "Notion AI agents" --depth deep
  grok-search cli research "Notion AI agents" --depth deep --domain example.com --max-fetches 6 --json
  grok-search cli tinyfish-bench --cases docs/tinyfish-benchmark-cases.sample.json --json
`

// Run dispatches the cli subcommand tree. args is everything after the
// leading "cli" token (so args[0] is the subcommand name).
func Run(args []string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprint(os.Stdout, usage)
		return 0
	}

	cmd, rest := args[0], args[1:]
	switch cmd {
	case "search":
		return runSearch(rest)
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
	case "probe":
		return runProbe(rest)
	case "plan":
		return runPlan(rest)
	case "research":
		return runResearch(rest)
	case "tinyfish-bench":
		return runTinyFishBench(rest)
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q\n\n%s", cmd, usage)
		return 2
	}
}
