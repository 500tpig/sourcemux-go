package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bettas/grok-search-go/internal/engine"
	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// SourceCacher is implemented by server.App to cache sources.
type SourceCacher interface {
	CacheSources(sessionID string, urls []string)
}

// RegisterSearch registers the web_search tool.
//
// Routing order:
//  1. Grok            — primary AI web search.
//  2. Tavily Search   — fallback when Grok errors out (rate limit, auth, 5xx, ...).
func RegisterSearch(s *mcpserver.MCPServer, grok *engine.GrokClient, tavily *engine.TavilyClient, cache SourceCacher) {
	tool := mcp.NewTool("web_search",
		mcp.WithDescription("AI-powered web search via Grok with Tavily Search fallback. Returns answer text and a session_id for source retrieval."),
		mcp.WithString("query", mcp.Required(), mcp.Description("Search query")),
		mcp.WithString("platform", mcp.Description("Focus platform, e.g. 'Twitter', 'GitHub, Reddit'")),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query, _ := req.Params.Arguments["query"].(string)
		if query == "" {
			return mcp.NewToolResultError("query is required"), nil
		}

		// Inject time context for temporal queries.
		query = injectTimeContext(query)

		if platform, _ := req.Params.Arguments["platform"].(string); platform != "" {
			query = fmt.Sprintf("[Focus: %s] %s", platform, query)
		}

		// 1) Try Grok first.
		result, grokErr := grok.Search(ctx, query)
		if grokErr == nil && result != nil && result.Content != "" {
			sessionID := uuid.New().String()
			cache.CacheSources(sessionID, result.SourceURLs)
			response := fmt.Sprintf(
				"engine: Grok\nsession_id: %s\nsources_count: %d\n\n%s",
				sessionID, result.SourcesCount, result.Content,
			)
			return mcp.NewToolResultText(response), nil
		}

		// 2) Fallback to Tavily Search.
		if tavily != nil {
			if tres, terr := tavily.Search(ctx, query); terr == nil {
				return mcp.NewToolResultText(formatTavilyResponse(tres, grokErr, cache)), nil
			}
		}

		if grokErr != nil {
			return mcp.NewToolResultError(fmt.Sprintf("search failed: %v", grokErr)), nil
		}
		return mcp.NewToolResultError("search returned empty result and no fallback configured"), nil
	})
}

// formatTavilyResponse renders a Tavily Search result in the same shape as the
// Grok branch (engine + session_id + sources_count + body), and registers the
// source URLs in the session cache for later get_sources calls.
func formatTavilyResponse(res *engine.TavilySearchResult, grokErr error, cache SourceCacher) string {
	sessionID := uuid.New().String()
	urls := make([]string, 0, len(res.Results))
	for _, r := range res.Results {
		if r.URL != "" {
			urls = append(urls, r.URL)
		}
	}
	cache.CacheSources(sessionID, urls)

	var sb strings.Builder
	if grokErr != nil {
		fmt.Fprintf(&sb, "engine: Tavily Search (Grok fallback: %v)\n", grokErr)
	} else {
		sb.WriteString("engine: Tavily Search (Grok returned empty)\n")
	}
	fmt.Fprintf(&sb, "session_id: %s\nsources_count: %d\n\n", sessionID, len(urls))
	if res.Answer != "" {
		sb.WriteString(res.Answer)
		sb.WriteString("\n\n")
	}
	if len(res.Results) > 0 {
		sb.WriteString("Sources:\n")
		for _, r := range res.Results {
			title := r.Title
			if title == "" {
				title = r.URL
			}
			fmt.Fprintf(&sb, "- %s \u2014 %s\n", title, r.URL)
		}
	}
	return sb.String()
}

// injectTimeContext prepends local time info when the query contains temporal keywords.
func injectTimeContext(query string) string {
	temporalKeywords := []string{
		"最新", "今天", "昨天", "本周", "本月", "近期",
		"recent", "today", "yesterday", "this week", "latest", "current",
	}
	q := strings.ToLower(query)
	for _, kw := range temporalKeywords {
		if strings.Contains(q, kw) {
			now := time.Now().Format("2006-01-02 15:04 MST")
			return fmt.Sprintf("[Current time: %s] %s", now, query)
		}
	}
	return query
}
