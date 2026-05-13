package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/500tpig/grok-search-go/internal/engine"
)

// searchOutput is the JSON envelope for `cli search`. Keep the fields stable
// so external scripts can rely on the shape.
type searchOutput struct {
	Engine       string   `json:"engine"`
	EndpointName string   `json:"endpoint_name,omitempty"`
	Model        string   `json:"model,omitempty"`
	Query        string   `json:"query"`
	Content      string   `json:"content"`
	SourceURLs   []string `json:"source_urls"`
	SourcesCount int      `json:"sources_count"`
	Fallback     string   `json:"fallback,omitempty"`
	GrokError    string   `json:"grok_error,omitempty"`
}

func runSearch(args []string) int {
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	model := fs.String("model", "", "One-shot Grok model override (e.g. grok-4.20-fast)")
	platform := fs.String("platform", "", "Focus a platform, e.g. 'Twitter', 'GitHub, Reddit'")
	timeout := fs.Duration("timeout", 60*time.Second, "Per-call timeout")
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

	cfg, err := loadConfig()
	if err != nil {
		return reportSearchErr(*jsonOut, query, fmt.Sprintf("config: %v", err))
	}

	pool := engine.NewGrokPool(cfg.GrokEndpoints)
	pool.OverallTimeout = cfg.GrokPoolTimeout

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	out := searchOutput{Query: query}

	res, gErr := pool.SearchWithModel(ctx, query, *model)
	if gErr == nil && res != nil && res.Content != "" {
		out.Engine = res.EndpointName
		out.EndpointName = res.EndpointName
		out.Model = res.EndpointModel
		out.Content = res.Content
		out.SourceURLs = res.SourceURLs
		out.SourcesCount = res.SourcesCount
		return emitSearch(*jsonOut, out)
	}
	if gErr != nil {
		out.GrokError = gErr.Error()
	}

	// TinyFish Search fallback.
	if cfg.TinyFishEnabled && len(cfg.TinyFishKeys) > 0 {
		tf := engine.NewTinyFishPool(cfg.TinyFishKeys, cfg.TinyFishSearchURL, cfg.TinyFishFetchURL)
		if tres, terr := tf.Search(ctx, engine.TinyFishSearchRequest{Query: query}); terr == nil && tres != nil {
			out.Engine = "tinyfish"
			out.EndpointName = tres.KeyName
			out.Fallback = "tinyfish"
			out.Content = engine.FormatTinyFishSearchContent(tres.TinyFishSearchResponse)
			out.SourceURLs = engine.TinyFishSearchSourceURLs(tres.TinyFishSearchResponse)
			out.SourcesCount = len(out.SourceURLs)
			return emitSearch(*jsonOut, out)
		}
	}

	// Exa Search fallback.
	if cfg.ExaEnabled && cfg.ExaAPIKey != "" {
		e := engine.NewExaClient(cfg.ExaAPIURL, cfg.ExaAPIKey)
		if eres, eerr := e.Search(ctx, query); eerr == nil && eres != nil {
			out.Engine = "exa"
			out.Fallback = "exa"
			out.Content = engine.FormatExaSearchContent(eres)
			out.SourceURLs = engine.ExaSearchSourceURLs(eres)
			out.SourcesCount = len(out.SourceURLs)
			return emitSearch(*jsonOut, out)
		}
	}

	// Tavily Search final fallback.
	if cfg.TavilyEnabled && cfg.TavilyAPIKey != "" {
		t := engine.NewTavilyClient(cfg.TavilyAPIURL, cfg.TavilyAPIKey)
		if tres, terr := t.Search(ctx, query); terr == nil && tres != nil {
			out.Engine = "tavily"
			out.Fallback = "tavily"
			out.Content = tres.Answer
			urls := make([]string, 0, len(tres.Results))
			for _, r := range tres.Results {
				if r.URL != "" {
					urls = append(urls, r.URL)
				}
			}
			out.SourceURLs = urls
			out.SourcesCount = len(urls)
			return emitSearch(*jsonOut, out)
		}
	}

	msg := "search failed: no engine returned content"
	if out.GrokError != "" {
		msg = "search failed: " + out.GrokError
	}
	return reportSearchErr(*jsonOut, query, msg)
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
	fmt.Printf("sources_count: %d\n\n", out.SourcesCount)
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
	if asJSON {
		_ = json.NewEncoder(os.Stdout).Encode(searchOutput{Query: query, GrokError: msg})
	} else {
		fmt.Fprintln(os.Stderr, msg)
	}
	return 1
}
