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

type exaContentsSubpageOutput struct {
	URL     string `json:"url"`
	Title   string `json:"title,omitempty"`
	Content string `json:"content,omitempty"`
}

type exaContentsOutput struct {
	Source    string                     `json:"source"`
	URL       string                     `json:"url"`
	ResultURL string                     `json:"result_url,omitempty"`
	Content   string                     `json:"content"`
	Subpages  []exaContentsSubpageOutput `json:"subpages,omitempty"`
}

func runExaContents(args []string) int {
	fs := flag.NewFlagSet("exa-contents", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	text := fs.Bool("text", true, "Return full text content")
	textMaxCharacters := fs.Int("text-max-characters", 0, "Optional max characters for text content")
	highlights := fs.Bool("highlights", false, "Return highlights")
	highlightsQuery := fs.String("highlights-query", "", "Optional query to steer Exa highlight extraction")
	subpages := fs.Int("subpages", 0, "Number of subpages to crawl")
	timeout := fs.Duration("timeout", 60*time.Second, "Per-call timeout")
	jsonOut := fs.Bool("json", false, "Emit JSON")
	maxAgeHours := fs.Int("max-age-hours", -2, "Optional Exa maxAgeHours; use 0 for fresh crawl or -1 for cache-only")
	var subpageTargets repeatedStringFlag
	fs.Var(&subpageTargets, "subpage-target", "Subpage target string; may be repeated")

	positional, err := parsePositional(fs, args)
	if err != nil {
		return 2
	}
	if len(positional) == 0 {
		fmt.Fprintln(os.Stderr, "exa-contents: url is required")
		fs.Usage()
		return 2
	}
	url := positional[0]

	cfg, err := loadConfig()
	if err != nil {
		return reportCLIError(*jsonOut, map[string]any{"url": url}, fmt.Sprintf("config: %v", err))
	}
	if !cfg.ExaEnabled || cfg.ExaAPIKey == "" {
		return reportCLIError(*jsonOut, map[string]any{"url": url}, "Exa is not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	var agePtr *int
	if *maxAgeHours != -2 {
		agePtr = maxAgeHours
	}

	client := engine.NewExaClient(cfg.ExaAPIURL, cfg.ExaAPIKey)
	result, err := client.ContentsAdvanced(ctx, engine.ExaContentsRequest{
		URL: url,
		Text: engine.ExaSearchTextOptions{
			Enabled:       *text,
			MaxCharacters: *textMaxCharacters,
		},
		Highlights: engine.ExaHighlightsOptions{
			Enabled:       *highlights,
			Query:         *highlightsQuery,
			MaxCharacters: 0,
		},
		Subpages:      *subpages,
		SubpageTarget: subpageTargets,
		MaxAgeHours:   agePtr,
	})
	if err != nil {
		return reportCLIError(*jsonOut, map[string]any{"url": url}, err.Error())
	}

	first := result.Results[0]
	out := exaContentsOutput{
		Source:    "exa-contents-advanced",
		URL:       url,
		ResultURL: first.URL,
		Content:   engine.FormatExaContentsContent(result, 1800, 500),
	}
	for _, subpage := range first.Subpages {
		out.Subpages = append(out.Subpages, exaContentsSubpageOutput{
			URL:     subpage.URL,
			Title:   subpage.Title,
			Content: engine.FormatExaContentsContent(&engine.ExaContentsResult{Results: []engine.ExaSearchHit{subpage}}, 500, 300),
		})
	}

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
		return 0
	}
	fmt.Printf("Source: %s\nURL: %s\n\n%s\n", out.Source, out.ResultURL, out.Content)
	return 0
}

func parseCLIJSONObject(raw string) (map[string]any, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("must decode to a non-empty JSON object")
	}
	return out, nil
}

func reportCLIError(asJSON bool, payload map[string]any, msg string) int {
	if asJSON {
		payload["error"] = msg
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(payload)
	} else {
		fmt.Fprintln(os.Stderr, msg)
	}
	return 1
}
