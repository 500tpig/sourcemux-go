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

type fetchOutput struct {
	Source  string `json:"source"`
	URL     string `json:"url"`
	Content string `json:"content"`
	Error   string `json:"error,omitempty"`
}

func runFetch(args []string) int {
	fs := flag.NewFlagSet("fetch", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	timeout := fs.Duration("timeout", 60*time.Second, "Per-call timeout")
	jsonOut := fs.Bool("json", false, "Emit JSON")
	positional, err := parsePositional(fs, args)
	if err != nil {
		return 2
	}
	if len(positional) == 0 {
		fmt.Fprintln(os.Stderr, "fetch: url is required")
		fs.Usage()
		return 2
	}
	url := positional[0]

	cfg, err := config.Load()
	if err != nil {
		return reportFetchErr(*jsonOut, url, fmt.Sprintf("config: %v", err))
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	// Jina Reader (primary).
	jina := engine.NewJinaClient(cfg.JinaAPIURL, cfg.JinaAPIKey)
	if r, err := jina.Fetch(ctx, url); err == nil && r.Content != "" {
		return emitFetch(*jsonOut, fetchOutput{Source: "jina", URL: url, Content: r.Content})
	}

	// Exa Contents (fallback).
	if cfg.ExaEnabled && cfg.ExaAPIKey != "" {
		e := engine.NewExaClient(cfg.ExaAPIURL, cfg.ExaAPIKey)
		if r, err := e.Extract(ctx, url); err == nil && r.Content != "" {
			return emitFetch(*jsonOut, fetchOutput{Source: "exa", URL: r.URL, Content: r.Content})
		}
	}

	// Tavily Extract (final fallback).
	if cfg.TavilyEnabled && cfg.TavilyAPIKey != "" {
		t := engine.NewTavilyClient(cfg.TavilyAPIURL, cfg.TavilyAPIKey)
		if r, err := t.Extract(ctx, url); err == nil && r.Content != "" {
			return emitFetch(*jsonOut, fetchOutput{Source: "tavily", URL: url, Content: r.Content})
		}
	}

	return reportFetchErr(*jsonOut, url, "Jina Reader, Exa Contents, and Tavily Extract all failed or are not configured")
}

func emitFetch(asJSON bool, out fetchOutput) int {
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
		return 0
	}
	fmt.Printf("Source: %s\nURL: %s\n\n%s\n", out.Source, out.URL, out.Content)
	return 0
}

func reportFetchErr(asJSON bool, url, msg string) int {
	if asJSON {
		_ = json.NewEncoder(os.Stdout).Encode(fetchOutput{URL: url, Error: msg})
	} else {
		fmt.Fprintln(os.Stderr, msg)
	}
	return 1
}
