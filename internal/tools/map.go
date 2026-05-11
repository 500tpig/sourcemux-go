package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/bettas/grok-search-go/internal/engine"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// RegisterMap registers the web_map tool for site structure discovery.
func RegisterMap(s *mcpserver.MCPServer, tavily *engine.TavilyClient) {
	tool := mcp.NewTool("web_map",
		mcp.WithDescription("Discover URLs on a website via Tavily Map API."),
		mcp.WithString("url", mcp.Required(), mcp.Description("Starting URL")),
		mcp.WithNumber("max_depth", mcp.Description("Max crawl depth (1-5, default 1)")),
		mcp.WithNumber("max_breadth", mcp.Description("Max links per page (1-500, default 20)")),
		mcp.WithNumber("limit", mcp.Description("Total URL limit (1-500, default 50)")),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if tavily == nil {
			return mcp.NewToolResultError("Tavily is not configured; web_map is unavailable"), nil
		}

		url, _ := req.Params.Arguments["url"].(string)
		if url == "" {
			return mcp.NewToolResultError("url is required"), nil
		}

		maxDepth := intArgOr(req.Params.Arguments, "max_depth", 1)
		maxBreadth := intArgOr(req.Params.Arguments, "max_breadth", 20)
		limit := intArgOr(req.Params.Arguments, "limit", 50)

		result, err := tavily.Map(ctx, url, maxDepth, maxBreadth, limit)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("web_map failed: %v", err)), nil
		}

		output := fmt.Sprintf("Found %d URLs:\n%s", len(result.URLs), strings.Join(result.URLs, "\n"))
		return mcp.NewToolResultText(output), nil
	})
}

func intArgOr(args map[string]any, key string, fallback int) int {
	v, ok := args[key].(float64)
	if !ok {
		return fallback
	}
	return int(v)
}
