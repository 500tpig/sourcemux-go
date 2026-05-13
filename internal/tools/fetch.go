package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/500tpig/sourcemux-go/internal/capability"
	"github.com/500tpig/sourcemux-go/internal/engine"
	"github.com/500tpig/sourcemux-go/internal/router"
	"github.com/500tpig/sourcemux-go/internal/router/adapters"
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
	Source     string            `json:"source"`
	URL        string            `json:"url"`
	Content    string            `json:"content"`
	RouteTrace router.RouteTrace `json:"route_trace,omitempty"`
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
		mcp.WithBoolean("include_trace", mcp.Description("Return full route trace in _meta.route_trace")),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		url, _ := req.Params.Arguments["url"].(string)
		includeTrace := boolArgOr(req.Params.Arguments, "include_trace", false)
		result, err := RunWebFetch(ctx, WebFetchClients{
			Jina:     jina,
			TinyFish: tinyfish,
			Exa:      exa,
			Tavily:   tavily,
		}, url)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		out := mcp.NewToolResultText(FormatWebFetchResult(result))
		if includeTrace {
			out.Meta = map[string]any{"route_trace": result.RouteTrace}
		} else {
			out.Meta = map[string]any{"route_trace": result.RouteTrace.Compact()}
		}
		return out, nil
	})
}

// RunWebFetch executes the production web_fetch fallback chain.
func RunWebFetch(ctx context.Context, clients WebFetchClients, url string) (*WebFetchResult, error) {
	if url == "" {
		return nil, fmt.Errorf("url is required")
	}

	r := router.New(fetchProviders(clients)...)
	res, trace := r.Run(ctx, capability.WebFetch, capability.Request{URL: url})
	if strings.TrimSpace(res.Content) == "" {
		if detail := firstFailureDetail(trace); detail != "" {
			return nil, fmt.Errorf("web_fetch failed: %s", detail)
		}
		return nil, fmt.Errorf("Jina Reader, TinyFish Fetch, Exa Contents, and Tavily Extract all failed or are not configured")
	}
	resultURL := metadataString(res.Metadata, "url", url)
	if resultURL == "" && len(res.Sources) > 0 {
		resultURL = res.Sources[0].URL
	}
	return &WebFetchResult{
		Source:     metadataString(res.Metadata, "engine", trace.FinalProvider),
		URL:        resultURL,
		Content:    res.Content,
		RouteTrace: trace,
	}, nil
}

func FormatWebFetchResult(result *WebFetchResult) string {
	if result == nil {
		return ""
	}
	content := strings.TrimSpace(result.Content)
	var sb strings.Builder
	fmt.Fprintf(&sb, "Source: %s\nURL: %s\n", result.Source, result.URL)
	if result.RouteTrace.AttemptsCount > 0 {
		fmt.Fprintf(&sb, "route: final_provider=%s fallback_triggered=%v attempts_count=%d\n",
			result.RouteTrace.FinalProvider, result.RouteTrace.FallbackTriggered, result.RouteTrace.AttemptsCount)
	}
	if content == "" {
		return strings.TrimSpace(sb.String())
	}
	fmt.Fprintf(&sb, "content_chars: %d\n\nexcerpt:\n%s",
		len([]rune(content)),
		indentContinuation(clipRunes(content, mcpFetchExcerptRunes), "  "),
	)
	return sb.String()
}

func fetchProviders(clients WebFetchClients) []capability.Provider {
	var providers []capability.Provider
	if clients.Jina != nil {
		providers = append(providers, adapters.NewJinaFetch(clients.Jina))
	}
	if clients.TinyFish != nil && clients.TinyFish.Len() > 0 {
		providers = append(providers, adapters.NewTinyFishFetch(clients.TinyFish))
	}
	if clients.Exa != nil {
		providers = append(providers, adapters.NewExaContents(clients.Exa))
	}
	if clients.Tavily != nil {
		providers = append(providers, adapters.NewTavilyExtract(clients.Tavily))
	}
	return providers
}
