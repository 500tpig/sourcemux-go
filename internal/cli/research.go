package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/bettas/grok-search-go/internal/config"
	"github.com/bettas/grok-search-go/internal/engine"
	"github.com/bettas/grok-search-go/internal/tools"
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
	platform := fs.String("platform", "", "Optional platform focus, e.g. 'GitHub, Reddit'")
	maxFetches := fs.Int("max-fetches", 0, "Maximum number of ranked URLs to fetch")
	timeout := fs.Duration("timeout", 180*time.Second, "End-to-end research timeout")
	jsonOut := fs.Bool("json", false, "Emit JSON")
	var domains repeatedStringFlag
	fs.Var(&domains, "domain", "Domain/site allow-list entry; may be repeated")

	positional, err := parsePositional(fs, args)
	if err != nil {
		return 2
	}
	if len(positional) == 0 {
		fmt.Fprintln(os.Stderr, "research: query is required")
		fs.Usage()
		return 2
	}
	query := strings.Join(positional, " ")
	opts := tools.ResearchOptions{
		Query:      query,
		Depth:      *depth,
		Platform:   *platform,
		Domains:    domains,
		MaxFetches: *maxFetches,
	}

	if runner == nil {
		cfg, err := loadConfig()
		if err != nil {
			return reportResearchErr(*jsonOut, query, *depth, fmt.Sprintf("config: %v", err))
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
		if *jsonOut {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			_ = enc.Encode(pack)
		} else {
			fmt.Fprintln(os.Stderr, err.Error())
		}
		return 1
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
	pool := engine.NewGrokPool(cfg.GrokEndpoints)
	pool.OverallTimeout = cfg.GrokPoolTimeout

	var tavily *engine.TavilyClient
	if cfg.TavilyEnabled && cfg.TavilyAPIKey != "" {
		tavily = engine.NewTavilyClient(cfg.TavilyAPIURL, cfg.TavilyAPIKey)
	}
	var exa *engine.ExaClient
	if cfg.ExaEnabled && cfg.ExaAPIKey != "" {
		exa = engine.NewExaClient(cfg.ExaAPIURL, cfg.ExaAPIKey)
	}
	var tinyfish *engine.TinyFishPool
	if cfg.TinyFishEnabled && len(cfg.TinyFishKeys) > 0 {
		tinyfish = engine.NewTinyFishPool(cfg.TinyFishKeys, cfg.TinyFishSearchURL, cfg.TinyFishFetchURL)
	}
	jina := engine.NewJinaClient(cfg.JinaAPIURL, cfg.JinaAPIKey)

	return tools.NewResearchExecutor(tools.ResearchExecutorDeps{
		Search: tools.WebSearchClients{
			Pool:     pool,
			TinyFish: tinyfish,
			Exa:      exa,
			Tavily:   tavily,
			Cache:    cache,
		},
		Fetch: tools.WebFetchClients{
			Jina:     jina,
			TinyFish: tinyfish,
			Exa:      exa,
			Tavily:   tavily,
		},
		Sources: cache,
		Mapper:  tavily,
		Crawler: tavily,
	})
}

func reportResearchErr(asJSON bool, query, depth, msg string) int {
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
