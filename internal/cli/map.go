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

type mapOutput struct {
	URL   string   `json:"url"`
	URLs  []string `json:"urls"`
	Count int      `json:"count"`
	Error string   `json:"error,omitempty"`
}

func runMap(args []string) int {
	fs := flag.NewFlagSet("map", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	depth := fs.Int("max-depth", 1, "Max crawl depth (1-5)")
	breadth := fs.Int("max-breadth", 20, "Max links per page (1-500)")
	limit := fs.Int("limit", 50, "Total URL limit (1-500)")
	timeout := fs.Duration("timeout", 90*time.Second, "Per-call timeout")
	jsonOut := fs.Bool("json", false, "Emit JSON")
	positional, err := parsePositional(fs, args)
	if err != nil {
		return 2
	}
	if len(positional) == 0 {
		fmt.Fprintln(os.Stderr, "map: url is required")
		fs.Usage()
		return 2
	}
	url := positional[0]

	cfg, err := loadConfig()
	if err != nil {
		return reportMapErr(*jsonOut, url, fmt.Sprintf("config: %v", err))
	}
	if !cfg.TavilyEnabled || cfg.TavilyAPIKey == "" {
		return reportMapErr(*jsonOut, url, "Tavily is not configured; map is unavailable")
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	t := engine.NewTavilyClient(cfg.TavilyAPIURL, cfg.TavilyAPIKey)
	r, err := t.Map(ctx, url, *depth, *breadth, *limit)
	if err != nil {
		return reportMapErr(*jsonOut, url, err.Error())
	}

	out := mapOutput{URL: url, URLs: r.URLs, Count: len(r.URLs)}
	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
		return 0
	}
	fmt.Printf("Found %d URLs:\n%s\n", out.Count, strings.Join(out.URLs, "\n"))
	return 0
}

func reportMapErr(asJSON bool, url, msg string) int {
	if asJSON {
		_ = json.NewEncoder(os.Stdout).Encode(mapOutput{URL: url, Error: msg})
	} else {
		fmt.Fprintln(os.Stderr, msg)
	}
	return 1
}
