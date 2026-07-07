package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/500tpig/sourcemux-go/internal/config"
	"github.com/500tpig/sourcemux-go/internal/tools"
)

type repeatedStringFlag []string

func (f *repeatedStringFlag) String() string {
	return strings.Join(*f, ",")
}

func (f *repeatedStringFlag) Set(value string) error {
	*f = append(*f, value)
	return nil
}

type researchRunner interface {
	Run(ctx context.Context, opts tools.ResearchOptions) (tools.ResearchPack, error)
}

func runResearch(args []string) int {
	return runResearchWithRunner(args, nil)
}

func runResearchWithRunner(args []string, runner researchRunner) int {
	fs := flag.NewFlagSet("research", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	depth := fs.String("depth", "standard", "Research depth: quick, standard, or deep")
	profile := fs.String("profile", tools.SearchProfileAuto, "Grok endpoint profile to use: auto, default, heavy, or another configured profile")
	platform := fs.String("platform", "", "Optional platform focus, e.g. 'GitHub, Reddit'")
	maxFetches := fs.Int("max-fetches", 0, "Maximum number of ranked URLs to fetch")
	timeout := fs.Duration("timeout", 300*time.Second, "End-to-end research timeout")
	jsonOut := fs.Bool("json", false, "Emit JSON")
	agentOut := fs.Bool("agent", false, "Emit compact agent-friendly JSON")
	var domains repeatedStringFlag
	fs.Var(&domains, "domain", "Domain/site allow-list entry; may be repeated")

	positional, err := parsePositional(fs, args)
	if err != nil {
		return 2
	}
	timeoutProvided := flagWasProvided(fs, "timeout")
	if len(positional) == 0 {
		fmt.Fprintln(os.Stderr, "research: query is required")
		fs.Usage()
		return 2
	}
	query := strings.Join(positional, " ")
	opts := tools.ResearchOptions{
		Query:      query,
		Depth:      *depth,
		Profile:    *profile,
		Platform:   *platform,
		Domains:    domains,
		MaxFetches: *maxFetches,
	}

	if runner == nil {
		cfg, err := loadConfig()
		if err != nil {
			return reportResearchErr(*jsonOut, *agentOut, query, *depth, fmt.Sprintf("config: %v", err))
		}
		if !timeoutProvided && cfg.SearchPolicy.Timeout > 0 {
			*timeout = cfg.SearchPolicy.Timeout
		}
		runner = buildResearchRunner(cfg)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	pack, err := runner.Run(ctx, opts)
	if err != nil {
		if pack.Query == "" {
			pack.Query = query
		}
		if pack.EffectiveDepth == "" {
			pack.EffectiveDepth = *depth
		}
		pack.Error = err.Error()
		if *agentOut {
			return emitAgent(tools.BuildResearchAgentOutput(pack))
		}
		if *jsonOut {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			_ = enc.Encode(pack)
		} else {
			fmt.Fprintln(os.Stderr, err.Error())
		}
		return 1
	}

	if *agentOut {
		return emitAgent(tools.BuildResearchAgentOutput(pack))
	}
	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(pack)
		return 0
	}
	fmt.Println(tools.FormatResearchPack(pack))
	return 0
}

func buildResearchRunner(cfg *config.Config) researchRunner {
	cache := tools.NewMemorySourceCache()
	search := buildWebSearchClients(cfg, cache)
	fetch := buildWebFetchClients(cfg)

	return tools.NewResearchExecutor(tools.ResearchExecutorDeps{
		Search:  search,
		Fetch:   fetch,
		Sources: cache,
		Mapper:  search.Tavily,
		Crawler: search.Tavily,
	})
}

func reportResearchErr(asJSON, asAgent bool, query, depth, msg string) int {
	if asAgent {
		return emitAgentError("research", msg, 1)
	}
	if asJSON {
		_ = json.NewEncoder(os.Stdout).Encode(tools.ResearchPack{
			Query:          query,
			EffectiveDepth: depth,
			Error:          msg,
		})
	} else {
		fmt.Fprintln(os.Stderr, msg)
	}
	return 1
}
