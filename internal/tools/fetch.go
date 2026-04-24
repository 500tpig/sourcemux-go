package tools

import (
	"context"
	"fmt"

	"github.com/bettas/grok-search-go/internal/engine"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/mark3labs/mcp-go/mcp"
)

// RegisterFetch registers the web_fetch tool with Tavily → Firecrawl fallback.
func RegisterFetch(s *mcpserver.MCPServer, tavily *engine.TavilyClient, firecrawl *engine.FirecrawlClient) {
	tool := mcp.NewTool("web_fetch",
		mcp.WithDescription("Fetch and extract web page content as Markdown. Uses Tavily Extract with Firecrawl fallback."),
		mcp.WithString("url", mcp.Required(), mcp.Description("Target URL to fetch")),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		url, _ := req.Params.Arguments["url"].(string)
		if url == "" {
			return mcp.NewToolResultError("url is required"), nil
		}

		// Try Tavily first
		if tavily != nil {
			result, err := tavily.Extract(ctx, url)
			if err == nil && result.Content != "" {
				return mcp.NewToolResultText(fmt.Sprintf("Source: Tavily Extract\nURL: %s\n\n%s", url, result.Content)), nil
			}
			// Tavily failed, fall through to Firecrawl
		}

		// Fallback to Firecrawl
		if firecrawl != nil {
			result, err := firecrawl.Scrape(ctx, url)
			if err == nil && result.Content != "" {
				return mcp.NewToolResultText(fmt.Sprintf("Source: Firecrawl Scrape\nURL: %s\n\n%s", url, result.Content)), nil
			}
		}

		return mcp.NewToolResultError("both Tavily and Firecrawl failed or are not configured"), nil
	})
}
