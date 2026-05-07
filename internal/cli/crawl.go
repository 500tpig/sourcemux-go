package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/bettas/grok-search-go/internal/config"
	"github.com/bettas/grok-search-go/internal/engine"
)

type crawlOutput struct {
	Source       string                   `json:"source"`
	URL          string                   `json:"url"`
	BaseURL      string                   `json:"base_url"`
	Results      []engine.TavilyCrawlPage `json:"results"`
	Count        int                      `json:"count"`
	ResponseTime float64                  `json:"response_time,omitempty"`
	Error        string                   `json:"error,omitempty"`
}

func runCrawl(args []string) int {
	fs := flag.NewFlagSet("crawl", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	depth := fs.Int("max-depth", 1, "Max crawl depth (1-5)")
	breadth := fs.Int("max-breadth", 20, "Max links per page/level (1-500)")
	limit := fs.Int("limit", 10, "Total page limit")
	instructions := fs.String("instructions", "", "Natural language crawl guidance")
	extractDepth := fs.String("extract-depth", "basic", "Extraction depth: basic or advanced")
	format := fs.String("format", "markdown", "Extracted content format: markdown or text")
	includeImages := fs.Bool("include-images", false, "Include image URLs in crawl results")
	timeout := fs.Duration("timeout", 150*time.Second, "Per-call timeout")
	jsonOut := fs.Bool("json", false, "Emit JSON")
	positional, err := parsePositional(fs, args)
	if err != nil {
		return 2
	}
	if len(positional) == 0 {
		fmt.Fprintln(os.Stderr, "crawl: url is required")
		fs.Usage()
		return 2
	}
	url := positional[0]

	cfg, err := config.Load()
	if err != nil {
		return reportCrawlErr(*jsonOut, url, fmt.Sprintf("config: %v", err))
	}
	if !cfg.TavilyEnabled || cfg.TavilyAPIKey == "" {
		return reportCrawlErr(*jsonOut, url, "Tavily is not configured; crawl is unavailable")
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	t := engine.NewTavilyClient(cfg.TavilyAPIURL, cfg.TavilyAPIKey)
	r, err := t.Crawl(ctx, engine.TavilyCrawlRequest{
		URL:           url,
		Instructions:  *instructions,
		MaxDepth:      *depth,
		MaxBreadth:    *breadth,
		Limit:         *limit,
		ExtractDepth:  *extractDepth,
		Format:        *format,
		IncludeImages: *includeImages,
	})
	if err != nil {
		return reportCrawlErr(*jsonOut, url, err.Error())
	}

	out := crawlOutput{
		Source:       "tavily",
		URL:          url,
		BaseURL:      r.BaseURL,
		Results:      r.Results,
		Count:        len(r.Results),
		ResponseTime: r.ResponseTime,
	}
	return emitCrawl(*jsonOut, out)
}

func emitCrawl(asJSON bool, out crawlOutput) int {
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
		return 0
	}
	fmt.Printf("Source: %s\nURL: %s\n\n%s\n", out.Source, out.URL, engine.FormatTavilyCrawlContent(&engine.TavilyCrawlResult{
		BaseURL:      out.BaseURL,
		Results:      out.Results,
		ResponseTime: out.ResponseTime,
	}, 1200))
	return 0
}

func reportCrawlErr(asJSON bool, url, msg string) int {
	if asJSON {
		_ = json.NewEncoder(os.Stdout).Encode(crawlOutput{URL: url, Error: msg})
	} else {
		fmt.Fprintln(os.Stderr, msg)
	}
	return 1
}
