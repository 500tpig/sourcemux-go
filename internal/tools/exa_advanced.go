package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/500tpig/sourcemux-go/internal/engine"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// RegisterExaSearchAdvanced exposes advanced Exa /search knobs without
// changing the default web_search fallback route.
func RegisterExaSearchAdvanced(s *mcpserver.MCPServer, exa *engine.ExaClient) {
	tool := mcp.NewTool("exa_search_advanced",
		mcp.WithDescription("Run Exa Search directly with advanced options such as deep/deep-reasoning, structured output schema, and contents controls. This does not change the default web_search route."),
		mcp.WithString("query", mcp.Required(), mcp.Description("Search query")),
		mcp.WithString("search_type", mcp.Description("Exa search type"), mcp.Enum("instant", "fast", "auto", "neural", "deep-lite", "deep", "deep-reasoning")),
		mcp.WithNumber("num_results", mcp.Description("Number of results to request (default 5)")),
		mcp.WithBoolean("text", mcp.Description("Return full text content for results")),
		mcp.WithNumber("text_max_characters", mcp.Description("Optional max characters for returned text")),
		mcp.WithBoolean("highlights", mcp.Description("Return highlights for results (default true when no other content mode is selected)")),
		mcp.WithString("highlights_query", mcp.Description("Optional query to steer Exa highlight extraction")),
		mcp.WithString("system_prompt", mcp.Description("Optional Exa systemPrompt for structured/deep search guidance")),
		mcp.WithString("output_schema_json", mcp.Description("Optional JSON object string for Exa outputSchema")),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if exa == nil {
			return mcp.NewToolResultError("Exa is not configured; exa_search_advanced is unavailable"), nil
		}
		query := stringArgOr(req.Params.Arguments, "query", "")
		if strings.TrimSpace(query) == "" {
			return mcp.NewToolResultError("query is required"), nil
		}
		schema, err := parseJSONObjectString(stringArgOr(req.Params.Arguments, "output_schema_json", ""))
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("output_schema_json: %v", err)), nil
		}

		result, err := exa.SearchAdvanced(ctx, engine.ExaSearchRequest{
			Query:      query,
			Type:       stringArgOr(req.Params.Arguments, "search_type", "auto"),
			NumResults: intArgOr(req.Params.Arguments, "num_results", 5),
			Text: engine.ExaSearchTextOptions{
				Enabled:       boolArgOr(req.Params.Arguments, "text", false),
				MaxCharacters: intArgOr(req.Params.Arguments, "text_max_characters", 0),
			},
			Highlights: engine.ExaHighlightsOptions{
				Enabled:       boolArgOr(req.Params.Arguments, "highlights", false),
				Query:         stringArgOr(req.Params.Arguments, "highlights_query", ""),
				MaxCharacters: 0,
			},
			SystemPrompt: stringArgOr(req.Params.Arguments, "system_prompt", ""),
			OutputSchema: schema,
		})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("exa_search_advanced failed: %v", err)), nil
		}
		return mcp.NewToolResultText("Source: Exa Search (advanced)\n\n" + engine.FormatExaSearchContent(result)), nil
	})
}

// RegisterExaContentsAdvanced exposes selected Exa /contents knobs such as
// subpages and cache freshness control, without changing web_fetch defaults.
func RegisterExaContentsAdvanced(s *mcpserver.MCPServer, exa *engine.ExaClient) {
	tool := mcp.NewTool("exa_contents_advanced",
		mcp.WithDescription("Run Exa Contents directly with advanced options such as subpages, subpageTarget, and maxAgeHours. This does not change the default web_fetch route."),
		mcp.WithString("url", mcp.Required(), mcp.Description("Target URL to extract")),
		mcp.WithBoolean("text", mcp.Description("Return full text content (default true when no other content mode is selected)")),
		mcp.WithNumber("text_max_characters", mcp.Description("Optional max characters for returned text")),
		mcp.WithBoolean("highlights", mcp.Description("Return highlights instead of or alongside text")),
		mcp.WithString("highlights_query", mcp.Description("Optional query to steer Exa highlight extraction")),
		mcp.WithNumber("subpages", mcp.Description("Number of subpages to crawl beneath the URL")),
		mcp.WithArray("subpage_target",
			mcp.Description("Optional list of subpageTarget strings to focus Exa subpage discovery"),
			mcp.Items(map[string]any{"type": "string"}),
		),
		mcp.WithNumber("max_age_hours", mcp.Description("Optional Exa maxAgeHours; 0 forces live crawl, -1 forces cache")),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if exa == nil {
			return mcp.NewToolResultError("Exa is not configured; exa_contents_advanced is unavailable"), nil
		}
		url := stringArgOr(req.Params.Arguments, "url", "")
		if strings.TrimSpace(url) == "" {
			return mcp.NewToolResultError("url is required"), nil
		}

		result, err := exa.ContentsAdvanced(ctx, engine.ExaContentsRequest{
			URL: url,
			Text: engine.ExaSearchTextOptions{
				Enabled:       boolArgOr(req.Params.Arguments, "text", false),
				MaxCharacters: intArgOr(req.Params.Arguments, "text_max_characters", 0),
			},
			Highlights: engine.ExaHighlightsOptions{
				Enabled:       boolArgOr(req.Params.Arguments, "highlights", false),
				Query:         stringArgOr(req.Params.Arguments, "highlights_query", ""),
				MaxCharacters: 0,
			},
			Subpages:      intArgOr(req.Params.Arguments, "subpages", 0),
			SubpageTarget: stringSliceArg(req.Params.Arguments, "subpage_target"),
			MaxAgeHours:   optionalIntArg(req.Params.Arguments, "max_age_hours"),
		})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("exa_contents_advanced failed: %v", err)), nil
		}
		return mcp.NewToolResultText("Source: Exa Contents (advanced)\n\n" + engine.FormatExaContentsContent(result, 1800, 500)), nil
	})
}

func parseJSONObjectString(raw string) (map[string]any, error) {
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

func optionalIntArg(args map[string]any, key string) *int {
	raw, ok := args[key]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case float64:
		n := int(v)
		return &n
	case int:
		n := v
		return &n
	default:
		return nil
	}
}
