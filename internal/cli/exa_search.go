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
)

type exaSearchOutput struct {
	Source           string                `json:"source"`
	Query            string                `json:"query"`
	SearchType       string                `json:"search_type"`
	ResultCount      int                   `json:"result_count"`
	SourceURLs       []string              `json:"source_urls"`
	Content          string                `json:"content"`
	StructuredOutput any                   `json:"structured_output,omitempty"`
	Grounding        []engine.ExaGrounding `json:"grounding,omitempty"`
}

func runExaSearch(args []string) int {
	fs := flag.NewFlagSet("exa-search", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	searchType := fs.String("type", "auto", "Exa search type: instant, fast, auto, neural, deep-lite, deep, deep-reasoning")
	numResults := fs.Int("num-results", 5, "Number of search results to request")
	text := fs.Bool("text", false, "Return full text content")
	textMaxCharacters := fs.Int("text-max-characters", 0, "Optional max characters for text content")
	highlights := fs.Bool("highlights", true, "Return highlight snippets")
	highlightsQuery := fs.String("highlights-query", "", "Optional query to steer Exa highlight extraction")
	systemPrompt := fs.String("system-prompt", "", "Optional Exa systemPrompt")
	outputSchemaJSON := fs.String("output-schema-json", "", "Optional JSON object string for Exa outputSchema")
	timeout := fs.Duration("timeout", 60*time.Second, "Per-call timeout")
	jsonOut := fs.Bool("json", false, "Emit JSON")

	positional, err := parsePositional(fs, args)
	if err != nil {
		return 2
	}
	if len(positional) == 0 {
		fmt.Fprintln(os.Stderr, "exa-search: query is required")
		fs.Usage()
		return 2
	}
	query := strings.Join(positional, " ")

	schema, err := parseCLIJSONObject(*outputSchemaJSON)
	if err != nil {
		return reportCLIError(*jsonOut, map[string]any{"query": query}, fmt.Sprintf("output-schema-json: %v", err))
	}

	cfg, err := config.Load()
	if err != nil {
		return reportCLIError(*jsonOut, map[string]any{"query": query}, fmt.Sprintf("config: %v", err))
	}
	if !cfg.ExaEnabled || cfg.ExaAPIKey == "" {
		return reportCLIError(*jsonOut, map[string]any{"query": query}, "Exa is not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	client := engine.NewExaClient(cfg.ExaAPIURL, cfg.ExaAPIKey)
	result, err := client.SearchAdvanced(ctx, engine.ExaSearchRequest{
		Query:      query,
		Type:       *searchType,
		NumResults: *numResults,
		Text: engine.ExaSearchTextOptions{
			Enabled:       *text,
			MaxCharacters: *textMaxCharacters,
		},
		Highlights: engine.ExaHighlightsOptions{
			Enabled:       *highlights,
			Query:         *highlightsQuery,
			MaxCharacters: 0,
		},
		SystemPrompt: *systemPrompt,
		OutputSchema: schema,
	})
	if err != nil {
		return reportCLIError(*jsonOut, map[string]any{"query": query}, err.Error())
	}

	out := exaSearchOutput{
		Source:      "exa-search-advanced",
		Query:       query,
		SearchType:  result.SearchType,
		ResultCount: len(result.Results),
		SourceURLs:  engine.ExaSearchSourceURLs(result),
		Content:     engine.FormatExaSearchContent(result),
	}
	if result.Output != nil {
		out.StructuredOutput = result.Output.Content
		out.Grounding = result.Output.Grounding
	}
	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
		return 0
	}
	fmt.Printf("Source: %s\nSearchType: %s\nResults: %d\n\n%s\n", out.Source, out.SearchType, out.ResultCount, out.Content)
	return 0
}
