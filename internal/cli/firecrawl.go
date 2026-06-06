package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/500tpig/sourcemux-go/internal/capability"
	"github.com/500tpig/sourcemux-go/internal/engine"
	"github.com/500tpig/sourcemux-go/internal/router"
)

type firecrawlScrapeOutput struct {
	Source        string                         `json:"source"`
	URL           string                         `json:"url"`
	ResultURL     string                         `json:"result_url"`
	Content       string                         `json:"content"`
	Metadata      engine.FirecrawlScrapeMetadata `json:"metadata,omitempty"`
	Links         []string                       `json:"links,omitempty"`
	RouteDecision []router.RouteDecision         `json:"route_decision,omitempty"`
	Error         string                         `json:"error,omitempty"`
}

type firecrawlMapOutput struct {
	Source        string                    `json:"source"`
	URL           string                    `json:"url"`
	Search        string                    `json:"search,omitempty"`
	Links         []engine.FirecrawlMapLink `json:"links"`
	URLs          []string                  `json:"urls"`
	Count         int                       `json:"count"`
	RouteDecision []router.RouteDecision    `json:"route_decision,omitempty"`
	Error         string                    `json:"error,omitempty"`
}

func runFirecrawlScrape(args []string) int {
	fs := flag.NewFlagSet("firecrawl-scrape", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	formats := "markdown"
	fs.StringVar(&formats, "formats", "markdown", "Comma-separated Firecrawl formats; markdown is required for content output")
	fs.StringVar(&formats, "format", "markdown", "Alias for --formats")
	onlyMainContent := fs.Bool("only-main-content", true, "Only return the main page content")
	onlyCleanContent := fs.Bool("only-clean-content", false, "Ask Firecrawl to clean residual boilerplate")
	includeTags := fs.String("include-tags", "", "Comma-separated HTML tags to include")
	excludeTags := fs.String("exclude-tags", "", "Comma-separated HTML tags to exclude")
	waitFor := fs.Int("wait-for", 0, "Wait before scraping in milliseconds")
	firecrawlTimeout := fs.Int("firecrawl-timeout", 0, "Firecrawl request timeout in milliseconds")
	mobile := fs.Bool("mobile", false, "Use mobile emulation")
	removeBase64Images := fs.Bool("remove-base64-images", true, "Remove base64 images from markdown")
	blockAds := fs.Bool("block-ads", true, "Enable Firecrawl ad blocking")
	proxy := fs.String("proxy", "", "Firecrawl proxy mode: basic, enhanced, or auto")
	storeInCache := fs.Bool("store-in-cache", true, "Allow Firecrawl cache storage")
	zeroDataRetention := fs.Bool("zero-data-retention", false, "Request zero data retention")
	timeout := fs.Duration("timeout", 90*time.Second, "Per-call client timeout")
	jsonOut := fs.Bool("json", false, "Emit JSON")
	positional, err := parsePositional(fs, args)
	if err != nil {
		return 2
	}
	if len(positional) == 0 {
		fmt.Fprintln(os.Stderr, "firecrawl-scrape: url is required")
		fs.Usage()
		return 2
	}
	url := positional[0]

	cfg, err := loadConfig()
	if err != nil {
		return reportFirecrawlScrapeErr(*jsonOut, url, fmt.Sprintf("config: %v", err))
	}
	client := buildFirecrawlClient(cfg)
	if client == nil {
		return reportFirecrawlScrapeErr(*jsonOut, url, "Firecrawl is not configured; firecrawl-scrape is unavailable")
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	req := engine.FirecrawlScrapeRequest{
		URL:                url,
		Formats:            splitCSV(formats),
		OnlyMainContent:    onlyMainContent,
		OnlyCleanContent:   onlyCleanContent,
		IncludeTags:        splitCSV(*includeTags),
		ExcludeTags:        splitCSV(*excludeTags),
		WaitFor:            *waitFor,
		Timeout:            *firecrawlTimeout,
		Mobile:             mobile,
		RemoveBase64Images: removeBase64Images,
		BlockAds:           blockAds,
		Proxy:              *proxy,
		StoreInCache:       storeInCache,
		ZeroDataRetention:  zeroDataRetention,
	}
	res, err := client.Scrape(ctx, req)
	trace := directFirecrawlTrace("firecrawl-scrape", err)
	if err != nil {
		return reportFirecrawlScrapeErrWithTrace(*jsonOut, url, err.Error(), trace.Decisions)
	}
	out := firecrawlScrapeOutput{
		Source:        "Firecrawl Scrape",
		URL:           url,
		ResultURL:     engine.FirecrawlScrapeResultURL(res, url),
		Content:       res.Data.Markdown,
		Metadata:      res.Data.Metadata,
		Links:         res.Data.Links,
		RouteDecision: trace.Decisions,
	}
	return emitFirecrawlScrape(*jsonOut, out)
}

func runFirecrawlMap(args []string) int {
	fs := flag.NewFlagSet("firecrawl-map", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	search := fs.String("search", "", "Search query to order map results by relevance")
	limit := fs.Int("limit", 5000, "Maximum number of links to return")
	sitemap := fs.String("sitemap", "include", "Sitemap mode: skip, include, or only")
	includeSubdomains := fs.Bool("include-subdomains", true, "Include subdomains")
	ignoreQueryParameters := fs.Bool("ignore-query-parameters", true, "Ignore query parameters")
	ignoreCache := fs.Bool("ignore-cache", false, "Bypass sitemap cache")
	firecrawlTimeout := fs.Int("firecrawl-timeout", 0, "Firecrawl map timeout in milliseconds")
	timeout := fs.Duration("timeout", 90*time.Second, "Per-call client timeout")
	jsonOut := fs.Bool("json", false, "Emit JSON")
	positional, err := parsePositional(fs, args)
	if err != nil {
		return 2
	}
	if len(positional) == 0 {
		fmt.Fprintln(os.Stderr, "firecrawl-map: url is required")
		fs.Usage()
		return 2
	}
	url := positional[0]

	cfg, err := loadConfig()
	if err != nil {
		return reportFirecrawlMapErr(*jsonOut, url, fmt.Sprintf("config: %v", err))
	}
	client := buildFirecrawlClient(cfg)
	if client == nil {
		return reportFirecrawlMapErr(*jsonOut, url, "Firecrawl is not configured; firecrawl-map is unavailable")
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	req := engine.FirecrawlMapRequest{
		URL:                   url,
		Search:                *search,
		Sitemap:               *sitemap,
		IncludeSubdomains:     includeSubdomains,
		IgnoreQueryParameters: ignoreQueryParameters,
		IgnoreCache:           ignoreCache,
		Limit:                 *limit,
		Timeout:               *firecrawlTimeout,
	}
	res, err := client.Map(ctx, req)
	trace := directFirecrawlTrace("firecrawl-map", err)
	if err != nil {
		return reportFirecrawlMapErrWithTrace(*jsonOut, url, err.Error(), trace.Decisions)
	}
	out := firecrawlMapOutput{
		Source:        "Firecrawl Map",
		URL:           url,
		Search:        *search,
		Links:         res.Links,
		URLs:          engine.FirecrawlMapURLs(res),
		Count:         len(res.Links),
		RouteDecision: trace.Decisions,
	}
	return emitFirecrawlMap(*jsonOut, out)
}

func emitFirecrawlScrape(asJSON bool, out firecrawlScrapeOutput) int {
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
		return 0
	}
	fmt.Printf("Source: %s\nURL: %s\nResult URL: %s\n\n%s\n", out.Source, out.URL, out.ResultURL, engine.FormatFirecrawlScrapeContent(&engine.FirecrawlScrapeResult{
		Success: true,
		Data: engine.FirecrawlScrapeData{
			Markdown: out.Content,
			Links:    out.Links,
			Metadata: out.Metadata,
		},
	}, 1800))
	return 0
}

func emitFirecrawlMap(asJSON bool, out firecrawlMapOutput) int {
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
		return 0
	}
	fmt.Printf("Source: %s\nURL: %s\n", out.Source, out.URL)
	if out.Search != "" {
		fmt.Printf("Search: %s\n", out.Search)
	}
	fmt.Println()
	fmt.Println(engine.FormatFirecrawlMapContent(&engine.FirecrawlMapResult{Success: true, Links: out.Links}, 100))
	return 0
}

func reportFirecrawlScrapeErr(asJSON bool, url, msg string) int {
	return reportFirecrawlScrapeErrWithTrace(asJSON, url, msg, nil)
}

func reportFirecrawlScrapeErrWithTrace(asJSON bool, url, msg string, decisions []router.RouteDecision) int {
	if asJSON {
		_ = json.NewEncoder(os.Stdout).Encode(firecrawlScrapeOutput{URL: url, Error: msg, RouteDecision: decisions})
	} else {
		fmt.Fprintln(os.Stderr, msg)
	}
	return 1
}

func reportFirecrawlMapErr(asJSON bool, url, msg string) int {
	return reportFirecrawlMapErrWithTrace(asJSON, url, msg, nil)
}

func reportFirecrawlMapErrWithTrace(asJSON bool, url, msg string, decisions []router.RouteDecision) int {
	if asJSON {
		_ = json.NewEncoder(os.Stdout).Encode(firecrawlMapOutput{URL: url, Error: msg, RouteDecision: decisions})
	} else {
		fmt.Fprintln(os.Stderr, msg)
	}
	return 1
}

func directFirecrawlTrace(provider string, err error) router.RouteTrace {
	status := "ok"
	reason := capability.ReasonNone
	detail := ""
	if err != nil {
		status = "error"
		reason = capability.ReasonUpstreamError
		detail = err.Error()
	}
	return router.RouteTrace{
		FinalProvider:     provider,
		FallbackTriggered: false,
		AttemptsCount:     1,
		Decisions: []router.RouteDecision{{
			Capability:     capability.WebEnhance,
			Provider:       provider,
			Attempt:        1,
			Status:         status,
			FallbackReason: reason,
			FallbackDetail: detail,
		}},
	}
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
