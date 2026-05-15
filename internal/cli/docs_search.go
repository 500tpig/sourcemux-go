package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/500tpig/sourcemux-go/internal/tools"
)

type docsSearchOutput struct {
	Engine        string              `json:"engine"`
	EndpointName  string              `json:"endpoint_name,omitempty"`
	Query         string              `json:"query"`
	Content       string              `json:"content"`
	SourceURLs    []string            `json:"source_urls,omitempty"`
	SourcesCount  int                 `json:"sources_count"`
	RouteDecision []map[string]string `json:"route_decision,omitempty"`
	Error         string              `json:"error,omitempty"`
}

func runDocsSearch(args []string) int {
	fs := flag.NewFlagSet("docs-search", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	timeout := fs.Duration("timeout", 60*time.Second, "Per-call timeout")
	jsonOut := fs.Bool("json", false, "Emit JSON")
	positional, err := parsePositional(fs, args)
	if err != nil {
		return 2
	}
	if len(positional) == 0 {
		fmt.Fprintln(os.Stderr, "docs-search: query is required")
		fs.Usage()
		return 2
	}
	query := strings.Join(positional, " ")
	cfg, err := loadConfig()
	if err != nil {
		return reportDocsSearchErr(*jsonOut, query, fmt.Sprintf("config: %v", err))
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	res, err := tools.RunDocsSearch(ctx, buildDocsSearchClients(cfg, tools.NewMemorySourceCache()), tools.DocsSearchOptions{
		Query: query,
	})
	if err != nil {
		return reportDocsSearchErr(*jsonOut, query, err.Error())
	}
	out := docsSearchOutput{
		Engine:       res.Engine,
		EndpointName: res.EndpointName,
		Query:        res.Query,
		Content:      res.Content,
		SourceURLs:   res.SourceURLs,
		SourcesCount: res.SourcesCount,
	}
	for _, d := range res.RouteTrace.Decisions {
		out.RouteDecision = append(out.RouteDecision, map[string]string{
			"provider": d.Provider,
			"status":   d.Status,
			"reason":   string(d.FallbackReason),
		})
	}
	return emitDocsSearch(*jsonOut, out)
}

func emitDocsSearch(asJSON bool, out docsSearchOutput) int {
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
		return 0
	}
	if out.EndpointName != "" {
		fmt.Printf("engine: %s (%s)\n", out.Engine, out.EndpointName)
	} else {
		fmt.Printf("engine: %s\n", out.Engine)
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

func reportDocsSearchErr(asJSON bool, query, msg string) int {
	if asJSON {
		_ = json.NewEncoder(os.Stdout).Encode(docsSearchOutput{Query: query, Error: msg})
	} else {
		fmt.Fprintln(os.Stderr, msg)
	}
	return 1
}
