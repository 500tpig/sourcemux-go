package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/bettas/grok-search-go/internal/engine"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

const mcpFetchExcerptRunes = 1800

// WebFetchClients groups the production fetch providers in fallback order.
type WebFetchClients struct {
	Jina     *engine.JinaClient
	TinyFish *engine.TinyFishPool
	Exa      *engine.ExaClient
	Tavily   *engine.TavilyClient
}

// WebFetchResult is the shared fetch envelope used by MCP, CLI, and research.
type WebFetchResult struct {
	Source  string `json:"source"`
	URL     string `json:"url"`
	Content string `json:"content"`
}

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
		result, err := RunWebFetch(ctx, WebFetchClients{
			Jina:     jina,
			TinyFish: tinyfish,
			Exa:      exa,
			Tavily:   tavily,
		}, url)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(FormatWebFetchResult(result)), nil
	})
}

// RunWebFetch executes the production web_fetch fallback chain.
func RunWebFetch(ctx context.Context, clients WebFetchClients, url string) (*WebFetchResult, error) {
	if url == "" {
		return nil, fmt.Errorf("url is required")
	}

	// Try Jina Reader first.
	if clients.Jina != nil {
		result, err := clients.Jina.Fetch(ctx, url)
		if err == nil && result.Content != "" {
			return &WebFetchResult{Source: "Jina Reader", URL: url, Content: result.Content}, nil
		}
	}

	// Fallback to TinyFish Fetch.
	if clients.TinyFish != nil && clients.TinyFish.Len() > 0 {
		result, err := clients.TinyFish.Fetch(ctx, engine.TinyFishFetchRequest{
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
				return &WebFetchResult{Source: "TinyFish Fetch (" + result.KeyName + ")", URL: resultURL, Content: content}, nil
			}
		}
	}

	// Fallback to Exa Contents.
	if clients.Exa != nil {
		result, err := clients.Exa.Extract(ctx, url)
		if err == nil && result.Content != "" {
			return &WebFetchResult{Source: "Exa Contents", URL: result.URL, Content: result.Content}, nil
		}
	}

	// Final fallback to Tavily Extract.
	if clients.Tavily != nil {
		result, err := clients.Tavily.Extract(ctx, url)
		if err == nil && result.Content != "" {
			return &WebFetchResult{Source: "Tavily Extract", URL: url, Content: result.Content}, nil
		}
	}

	return nil, fmt.Errorf("Jina Reader, TinyFish Fetch, Exa Contents, and Tavily Extract all failed or are not configured")
}

func FormatWebFetchResult(result *WebFetchResult) string {
	if result == nil {
		return ""
	}
	content := strings.TrimSpace(result.Content)
	var sb strings.Builder
	fmt.Fprintf(&sb, "Source: %s\nURL: %s\n", result.Source, result.URL)
	if content == "" {
		return strings.TrimSpace(sb.String())
	}
	fmt.Fprintf(&sb, "content_chars: %d\n\nexcerpt:\n%s",
		len([]rune(content)),
		indentContinuation(clipRunes(content, mcpFetchExcerptRunes), "  "),
	)
	return sb.String()
}
