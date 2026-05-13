package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/500tpig/grok-search-go/internal/router"
	"github.com/500tpig/grok-search-go/internal/tools"
)

// searchOutput is the JSON envelope for `cli search`. Keep the fields stable
// so external scripts can rely on the shape.
type searchOutput struct {
	Engine        string                 `json:"engine"`
	EndpointName  string                 `json:"endpoint_name,omitempty"`
	Model         string                 `json:"model,omitempty"`
	Query         string                 `json:"query"`
	Content       string                 `json:"content"`
	SourceURLs    []string               `json:"source_urls"`
	SourcesCount  int                    `json:"sources_count"`
	Fallback      string                 `json:"fallback,omitempty"`
	GrokError     string                 `json:"grok_error,omitempty"`
	RouteDecision []router.RouteDecision `json:"route_decision,omitempty"`
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
	if msg := minimumProfileError(cfg); msg != "" {
		return reportSearchErrCode(*jsonOut, query, msg, 3)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	res, err := tools.RunWebSearch(ctx, buildWebSearchClients(cfg, tools.NewMemorySourceCache()), query, "", *model)
	if err != nil {
		return reportSearchErr(*jsonOut, query, err.Error())
	}
	return emitSearch(*jsonOut, searchOutput{
		Engine:        res.Engine,
		EndpointName:  res.EndpointName,
		Model:         res.Model,
		Query:         res.Query,
		Content:       res.Content,
		SourceURLs:    res.SourceURLs,
		SourcesCount:  res.SourcesCount,
		Fallback:      res.Fallback,
		GrokError:     res.GrokError,
		RouteDecision: res.RouteTrace.Decisions,
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
