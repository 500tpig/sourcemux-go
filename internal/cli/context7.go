package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/500tpig/sourcemux-go/internal/engine"
	"github.com/500tpig/sourcemux-go/internal/tools"
)

type context7Output struct {
	Engine        string                     `json:"engine"`
	EndpointName  string                     `json:"endpoint_name"`
	LibraryName   string                     `json:"library_name,omitempty"`
	LibraryID     string                     `json:"library_id,omitempty"`
	Query         string                     `json:"query"`
	SearchResults []engine.Context7Library   `json:"search_results,omitempty"`
	Docs          *engine.Context7DocsResult `json:"docs,omitempty"`
	Content       string                     `json:"content"`
	SourceURLs    []string                   `json:"source_urls,omitempty"`
	SourcesCount  int                        `json:"sources_count"`
	RouteDecision []map[string]string        `json:"route_decision,omitempty"`
	Error         string                     `json:"error,omitempty"`
}

func runContext7Library(args []string) int {
	fs := flag.NewFlagSet("context7-library", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	provider := fs.String("provider", "", "Named Context7 provider instance")
	fast := fs.Bool("fast", false, "Use Context7 fast mode")
	timeout := fs.Duration("timeout", 60*time.Second, "Per-call timeout")
	jsonOut := fs.Bool("json", false, "Emit JSON")
	positional, err := parsePositional(fs, args)
	if err != nil {
		return 2
	}
	if len(positional) < 2 {
		fmt.Fprintln(os.Stderr, "context7-library: library name and query are required")
		fs.Usage()
		return 2
	}
	libraryName := positional[0]
	query := strings.Join(positional[1:], " ")
	client, cfgErr := selectedContext7Client(*provider)
	if cfgErr != nil {
		return reportContext7Err(*jsonOut, query, cfgErr.Error())
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	search, err := client.SearchLibraries(ctx, engine.Context7LibrarySearchRequest{LibraryName: libraryName, Query: query, Fast: *fast})
	if err != nil {
		return reportContext7Err(*jsonOut, query, err.Error())
	}
	libraryID := ""
	if len(search.Results) > 0 {
		libraryID = search.Results[0].ID
	}
	if libraryID == "" {
		return reportContext7Err(*jsonOut, query, "context7 library search returned no library id")
	}
	docs, err := client.GetDocs(ctx, engine.Context7DocsRequest{LibraryID: libraryID, Query: query, Type: "json", Fast: *fast})
	if err != nil {
		return reportContext7Err(*jsonOut, query, err.Error())
	}
	return emitContext7(*jsonOut, context7Output{
		Engine:        "Context7",
		EndpointName:  client.Name(),
		LibraryName:   libraryName,
		LibraryID:     libraryID,
		Query:         query,
		SearchResults: search.Results,
		Docs:          docs,
		Content:       engine.FormatContext7DocsContent(docs, 1800),
		SourceURLs:    engine.Context7DocsSourceURLs(docs),
		SourcesCount:  len(engine.Context7DocsSourceURLs(docs)),
	})
}

func runContext7Docs(args []string) int {
	fs := flag.NewFlagSet("context7-docs", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	provider := fs.String("provider", "", "Named Context7 provider instance")
	fast := fs.Bool("fast", false, "Use Context7 fast mode")
	timeout := fs.Duration("timeout", 60*time.Second, "Per-call timeout")
	jsonOut := fs.Bool("json", false, "Emit JSON")
	positional, err := parsePositional(fs, args)
	if err != nil {
		return 2
	}
	if len(positional) < 2 {
		fmt.Fprintln(os.Stderr, "context7-docs: library id and query are required")
		fs.Usage()
		return 2
	}
	libraryID := positional[0]
	query := strings.Join(positional[1:], " ")
	client, cfgErr := selectedContext7Client(*provider)
	if cfgErr != nil {
		return reportContext7Err(*jsonOut, query, cfgErr.Error())
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	docs, err := client.GetDocs(ctx, engine.Context7DocsRequest{LibraryID: libraryID, Query: query, Type: "json", Fast: *fast})
	if err != nil {
		return reportContext7Err(*jsonOut, query, err.Error())
	}
	sourceURLs := engine.Context7DocsSourceURLs(docs)
	return emitContext7(*jsonOut, context7Output{
		Engine:       "Context7",
		EndpointName: client.Name(),
		LibraryID:    libraryID,
		Query:        query,
		Docs:         docs,
		Content:      engine.FormatContext7DocsContent(docs, 1800),
		SourceURLs:   sourceURLs,
		SourcesCount: len(sourceURLs),
	})
}

func runDocsSearch(args []string) int {
	fs := flag.NewFlagSet("docs-search", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	libraryID := fs.String("library-id", "", "Context7-compatible library ID")
	libraryName := fs.String("library-name", "", "Library name to resolve with Context7")
	provider := fs.String("provider", "", "Named Context7 provider instance")
	fast := fs.Bool("fast", false, "Use Context7 fast mode when applicable")
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
		return reportContext7Err(*jsonOut, query, fmt.Sprintf("config: %v", err))
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	res, err := tools.RunDocsSearch(ctx, buildDocsSearchClients(cfg, tools.NewMemorySourceCache()), tools.DocsSearchOptions{
		Query:       query,
		LibraryID:   *libraryID,
		LibraryName: *libraryName,
		Provider:    *provider,
		Fast:        *fast,
	})
	if err != nil {
		return reportContext7Err(*jsonOut, query, err.Error())
	}
	out := context7Output{
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
	return emitContext7(*jsonOut, out)
}

func selectedContext7Client(provider string) (*engine.Context7Client, error) {
	cfg, err := loadConfig()
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	clients := buildContext7Clients(cfg)
	if len(clients) == 0 {
		return nil, fmt.Errorf("context7 is not configured")
	}
	provider = strings.TrimSpace(provider)
	for _, client := range clients {
		if client == nil {
			continue
		}
		if provider == "" || provider == client.Name() {
			return client, nil
		}
	}
	return nil, fmt.Errorf("context7 provider %q not found", provider)
}

func emitContext7(asJSON bool, out context7Output) int {
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
		return 0
	}
	fmt.Printf("engine: %s (%s)\n", out.Engine, out.EndpointName)
	if out.LibraryID != "" {
		fmt.Printf("library_id: %s\n", out.LibraryID)
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

func reportContext7Err(asJSON bool, query, msg string) int {
	if asJSON {
		_ = json.NewEncoder(os.Stdout).Encode(context7Output{Query: query, Error: msg})
	} else {
		fmt.Fprintln(os.Stderr, msg)
	}
	return 1
}
