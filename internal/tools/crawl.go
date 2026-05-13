package tools

import (
	"context"
	"fmt"

	"github.com/500tpig/grok-search-go/internal/engine"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// RegisterCrawl registers the web_crawl tool for site-level crawl + extract.
func RegisterCrawl(s *mcpserver.MCPServer, tavily *engine.TavilyClient) {
	tool := mcp.NewTool("web_crawl",
		mcp.WithDescription("Crawl a website and extract page content via Tavily Crawl API. Use for deep research when web_map URL discovery is not enough; web_map only returns URLs, web_crawl returns extracted content."),
		mcp.WithString("url", mcp.Required(), mcp.Description("Root URL to begin the crawl")),
		mcp.WithString("instructions", mcp.Description("Natural language guidance for what the crawler should find")),
		mcp.WithNumber("max_depth", mcp.Description("Max crawl depth (1-5, default 1)"), mcp.Min(1), mcp.Max(5), mcp.DefaultNumber(1)),
		mcp.WithNumber("max_breadth", mcp.Description("Max links per page/level (1-500, default 20)"), mcp.Min(1), mcp.Max(500), mcp.DefaultNumber(20)),
		mcp.WithNumber("limit", mcp.Description("Total page limit (default 10)"), mcp.Min(1), mcp.DefaultNumber(10)),
		mcp.WithString("extract_depth", mcp.Description("Extraction depth: basic or advanced (default basic)"), mcp.Enum("basic", "advanced")),
		mcp.WithString("format", mcp.Description("Extracted content format: markdown or text (default markdown)"), mcp.Enum("markdown", "text")),
		mcp.WithBoolean("include_images", mcp.Description("Include image URLs in crawl results (default false)"), mcp.DefaultBool(false)),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if tavily == nil {
			return mcp.NewToolResultError("Tavily is not configured; web_crawl is unavailable"), nil
		}

		url, _ := req.Params.Arguments["url"].(string)
		if url == "" {
			return mcp.NewToolResultError("url is required"), nil
		}

		result, err := tavily.Crawl(ctx, engine.TavilyCrawlRequest{
			URL:           url,
			Instructions:  stringArgOr(req.Params.Arguments, "instructions", ""),
			MaxDepth:      intArgOr(req.Params.Arguments, "max_depth", 1),
			MaxBreadth:    intArgOr(req.Params.Arguments, "max_breadth", 20),
			Limit:         intArgOr(req.Params.Arguments, "limit", 10),
			ExtractDepth:  stringArgOr(req.Params.Arguments, "extract_depth", "basic"),
			Format:        stringArgOr(req.Params.Arguments, "format", "markdown"),
			IncludeImages: boolArgOr(req.Params.Arguments, "include_images", false),
		})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("web_crawl failed: %v", err)), nil
		}

		return mcp.NewToolResultText("Source: Tavily Crawl\n\n" + engine.FormatTavilyCrawlContent(result, 1200)), nil
	})
}

func stringArgOr(args map[string]any, key, fallback string) string {
	v, ok := args[key].(string)
	if !ok || v == "" {
		return fallback
	}
	return v
}

func boolArgOr(args map[string]any, key string, fallback bool) bool {
	v, ok := args[key].(bool)
	if !ok {
		return fallback
	}
	return v
}
