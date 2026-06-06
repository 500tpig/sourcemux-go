package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/500tpig/sourcemux-go/internal/router"
	"github.com/500tpig/sourcemux-go/internal/tools"
)

// searchOutput is the JSON envelope for `cli search`. Keep the fields stable
// so external scripts can rely on the shape.
type searchOutput struct {
	Engine           string                 `json:"engine"`
	EndpointName     string                 `json:"endpoint_name,omitempty"`
	Model            string                 `json:"model,omitempty"`
	RequestedProfile string                 `json:"requested_profile"`
	EffectiveProfile string                 `json:"effective_profile"`
	ProfileReason    string                 `json:"profile_reason,omitempty"`
	Query            string                 `json:"query"`
	Content          string                 `json:"content"`
	SourceURLs       []string               `json:"source_urls"`
	SourcesCount     int                    `json:"sources_count"`
	Fallback         string                 `json:"fallback,omitempty"`
	GrokError        string                 `json:"grok_error,omitempty"`
	CallerTimeout    string                 `json:"caller_timeout,omitempty"`
	GrokPoolTimeout  string                 `json:"grok_pool_timeout,omitempty"`
	NoFallback       bool                   `json:"no_fallback,omitempty"`
	RouteDecision    []router.RouteDecision `json:"route_decision,omitempty"`
}

func runSearch(args []string) int {
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	model := fs.String("model", "", "One-shot Grok model override (e.g. grok-4.20-fast)")
	profile := fs.String("profile", "", "Grok endpoint profile to use: default, auto, heavy, or another configured profile (default: searchPolicy.defaultProfile)")
	platform := fs.String("platform", "", "Focus a platform, e.g. 'Twitter', 'GitHub, Reddit'")
	timeout := fs.Duration("timeout", 0, "Per-call timeout")
	grokPoolTimeout := fs.Duration("grok-pool-timeout", 0, "Override configured Grok pool timeout; 0 disables the pool cap")
	fallbackAfter := fs.Duration("fallback-after", 0, "Alias for --grok-pool-timeout; controls when Grok gives way to fallback providers")
	noFallback := fs.Bool("no-fallback", false, "Only try the selected Grok pool; do not fall back to TinyFish, Exa, or Tavily")
	jsonOut := fs.Bool("json", false, "Emit JSON")
	positional, err := parsePositional(fs, args)
	if err != nil {
		return 2
	}
	if len(positional) == 0 {
		fmt.Fprintln(os.Stderr, "search: query is required")
		fs.Usage()
		return 2
	}
	query := strings.Join(positional, " ")
	if *platform != "" {
		query = fmt.Sprintf("[Focus: %s] %s", *platform, query)
	}
	poolTimeoutProvided := flagWasProvided(fs, "grok-pool-timeout")
	fallbackAfterProvided := flagWasProvided(fs, "fallback-after")
	if poolTimeoutProvided && fallbackAfterProvided {
		return reportSearchErrCode(*jsonOut, query, "search: use only one of --grok-pool-timeout or --fallback-after", 2)
	}
	timeoutProvided := flagWasProvided(fs, "timeout")
	profileProvided := flagWasProvided(fs, "profile")

	cfg, err := loadConfig()
	if err != nil {
		return reportSearchErr(*jsonOut, query, fmt.Sprintf("config: %v", err))
	}
	if msg := minimumProfileError(cfg); msg != "" {
		return reportSearchErrCode(*jsonOut, query, msg, 3)
	}

	effectiveTimeout := *timeout
	if !timeoutProvided {
		effectiveTimeout = cfg.SearchPolicy.Timeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), effectiveTimeout)
	defer cancel()

	clients := buildWebSearchClients(cfg, tools.NewMemorySourceCache())
	if *noFallback {
		clients.DisableFallbacks = true
	}
	effectiveGrokPoolTimeout := cfg.GrokPoolTimeout
	if !cfg.GrokPoolTimeoutSet {
		effectiveGrokPoolTimeout = cfg.SearchPolicy.FallbackAfter
		clients = clients.WithGrokPoolTimeout(effectiveGrokPoolTimeout)
	}
	if poolTimeoutProvided || fallbackAfterProvided {
		effectiveGrokPoolTimeout = *grokPoolTimeout
		if fallbackAfterProvided {
			effectiveGrokPoolTimeout = *fallbackAfter
		}
		clients = clients.WithGrokPoolTimeout(effectiveGrokPoolTimeout)
	}

	effectiveProfile := *profile
	if !profileProvided {
		effectiveProfile = cfg.SearchPolicy.DefaultProfile
	}
	res, err := tools.RunWebSearch(ctx, clients, query, "", *model, effectiveProfile)
	if err != nil {
		return reportSearchErr(*jsonOut, query, err.Error())
	}
	return emitSearch(*jsonOut, searchOutput{
		Engine:           res.Engine,
		EndpointName:     res.EndpointName,
		Model:            res.Model,
		RequestedProfile: res.RequestedProfile,
		EffectiveProfile: res.EffectiveProfile,
		ProfileReason:    res.ProfileReason,
		Query:            res.Query,
		Content:          res.Content,
		SourceURLs:       res.SourceURLs,
		SourcesCount:     res.SourcesCount,
		Fallback:         res.Fallback,
		GrokError:        res.GrokError,
		CallerTimeout:    effectiveTimeout.String(),
		GrokPoolTimeout:  effectiveGrokPoolTimeout.String(),
		NoFallback:       *noFallback,
		RouteDecision:    res.RouteTrace.Decisions,
	})
}

func emitSearch(asJSON bool, out searchOutput) int {
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
		return 0
	}
	if out.Fallback != "" {
		fmt.Printf("engine: %s (fallback)\n", out.Engine)
	} else {
		fmt.Printf("engine: %s (%s)\n", out.Engine, out.Model)
	}
	if out.RequestedProfile != "" || out.EffectiveProfile != "" {
		fmt.Printf("profile: requested=%s effective=%s\n", out.RequestedProfile, out.EffectiveProfile)
	}
	fmt.Printf("sources_count: %d\n\n", out.SourcesCount)
	if len(out.RouteDecision) > 0 {
		fmt.Printf("route_attempts: %d\n\n", len(out.RouteDecision))
	}
	fmt.Println(out.Content)
	if len(out.SourceURLs) > 0 {
		fmt.Println("\nSources:")
		for _, u := range out.SourceURLs {
			fmt.Printf("- %s\n", u)
		}
	}
	return 0
}

func reportSearchErr(asJSON bool, query, msg string) int {
	return reportSearchErrCode(asJSON, query, msg, 1)
}

func reportSearchErrCode(asJSON bool, query, msg string, code int) int {
	if asJSON {
		_ = json.NewEncoder(os.Stdout).Encode(searchOutput{Query: query, GrokError: msg})
	} else {
		fmt.Fprintln(os.Stderr, msg)
	}
	return code
}

func flagWasProvided(fs *flag.FlagSet, name string) bool {
	found := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}
