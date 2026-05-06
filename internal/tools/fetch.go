package tools

import (
	"context"
	"fmt"

	"github.com/bettas/grok-search-go/internal/engine"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// RegisterFetch registers the web_fetch tool.
//
// Routing order:
//  1. Jina Reader  — free, lightweight, handles 90% of pages well.
//  2. TinyFish Fetch — browser-rendered fallback for JS-heavy pages.
//  3. Exa Contents — paid/source-aware fallback when Jina/TinyFish miss.
//  4. Tavily Extract — final fallback for JS-heavy / Jina-blocked pages.
func RegisterFetch(s *mcpserver.MCPServer, jina *engine.JinaClient, tinyfish *engine.TinyFishPool, exa *engine.ExaClient, tavily *engine.TavilyClient) {
	tool := mcp.NewTool("web_fetch",
		mcp.WithDescription("Fetch and extract web page content as Markdown. Uses Jina Reader (primary), then TinyFish Fetch, Exa Contents, and Tavily Extract fallback."),
		mcp.WithString("url", mcp.Required(), mcp.Description("Target URL to fetch")),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		url, _ := req.Params.Arguments["url"].(string)
		if url == "" {
			return mcp.NewToolResultError("url is required"), nil
		}

		// Try Jina Reader first.
		if jina != nil {
			result, err := jina.Fetch(ctx, url)
			if err == nil && result.Content != "" {
				return mcp.NewToolResultText(fmt.Sprintf("Source: Jina Reader\nURL: %s\n\n%s", url, result.Content)), nil
			}
		}

		// Fallback to TinyFish Fetch.
		if tinyfish != nil && tinyfish.Len() > 0 {
			result, err := tinyfish.Fetch(ctx, engine.TinyFishFetchRequest{
				URLs:   []string{url},
				Format: "markdown",
			})
			if err == nil {
				content := engine.TinyFishFetchContent(result.TinyFishFetchResponse)
				if content != "" {
					resultURL := url
					if len(result.Results) > 0 {
						if result.Results[0].FinalURL != "" {
							resultURL = result.Results[0].FinalURL
						} else if result.Results[0].URL != "" {
							resultURL = result.Results[0].URL
						}
					}
					return mcp.NewToolResultText(fmt.Sprintf("Source: TinyFish Fetch (%s)\nURL: %s\n\n%s", result.KeyName, resultURL, content)), nil
				}
			}
		}

		// Fallback to Exa Contents.
		if exa != nil {
			result, err := exa.Extract(ctx, url)
			if err == nil && result.Content != "" {
				return mcp.NewToolResultText(fmt.Sprintf("Source: Exa Contents\nURL: %s\n\n%s", result.URL, result.Content)), nil
			}
		}

		// Final fallback to Tavily Extract.
		if tavily != nil {
			result, err := tavily.Extract(ctx, url)
			if err == nil && result.Content != "" {
				return mcp.NewToolResultText(fmt.Sprintf("Source: Tavily Extract\nURL: %s\n\n%s", url, result.Content)), nil
			}
		}

		return mcp.NewToolResultError("Jina Reader, TinyFish Fetch, Exa Contents, and Tavily Extract all failed or are not configured"), nil
	})
}
