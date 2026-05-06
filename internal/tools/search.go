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
//  1. Grok pool (each endpoint in priority order) — primary AI web search.
//  2. TinyFish Search                            — browser-backed source-first fallback.
//  3. Exa Search                                 — source-first fallback.
//  4. Tavily Search                              — final fallback when every
//     previous engine fails.
//     endpoint either errors or returns empty content.
func RegisterSearch(s *mcpserver.MCPServer, pool *engine.GrokPool, tinyfish *engine.TinyFishPool, exa *engine.ExaClient, tavily *engine.TavilyClient, cache SourceCacher) {
	tool := mcp.NewTool("web_search",
		mcp.WithDescription("AI-powered web search. Tries each configured Grok endpoint in priority order, then falls back to TinyFish Search, Exa Search, and Tavily Search. Returns answer/source text, an engine label, and a session_id for source retrieval."),
		mcp.WithString("query", mcp.Required(), mcp.Description("Search query")),
		mcp.WithString("platform", mcp.Description("Focus platform, e.g. 'Twitter', 'GitHub, Reddit'")),
		mcp.WithString("model", mcp.Description("Optional one-shot Grok model override, e.g. 'grok-4.20-fast'")),
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
		model, _ := req.Params.Arguments["model"].(string)

		// 1) Try Grok pool.
		var poolErr error
		if pool != nil && pool.Len() > 0 {
			res, err := pool.SearchWithModel(ctx, query, model)
			if err == nil && res != nil && res.Content != "" {
				sessionID := uuid.New().String()
				cache.CacheSources(sessionID, res.SourceURLs)
				response := fmt.Sprintf(
					"engine: %s (%s)\nsession_id: %s\nsources_count: %d\n\n%s",
					res.EndpointName, res.EndpointModel,
					sessionID, res.SourcesCount, res.Content,
				)
				return mcp.NewToolResultText(response), nil
			}
			poolErr = err
		}

		// 2) Fallback to TinyFish Search.
		if tinyfish != nil && tinyfish.Len() > 0 {
			if tres, terr := tinyfish.Search(ctx, engine.TinyFishSearchRequest{Query: query}); terr == nil {
				return mcp.NewToolResultText(formatTinyFishResponse(tres, poolErr, cache)), nil
			}
		}

		// 3) Fallback to Exa Search.
		if exa != nil {
			if eres, eerr := exa.Search(ctx, query); eerr == nil {
				return mcp.NewToolResultText(formatExaResponse(eres, poolErr, cache)), nil
			}
		}

		// 4) Fallback to Tavily Search.
		if tavily != nil {
			if tres, terr := tavily.Search(ctx, query); terr == nil {
				return mcp.NewToolResultText(formatTavilyResponse(tres, poolErr, cache)), nil
			}
		}

		if poolErr != nil {
			return mcp.NewToolResultError(fmt.Sprintf("search failed: %v", poolErr)), nil
		}
		return mcp.NewToolResultError("search returned empty result and no fallback configured"), nil
	})
}

func formatTinyFishResponse(res *engine.TinyFishPoolSearchResult, grokErr error, cache SourceCacher) string {
	sessionID := uuid.New().String()
	urls := engine.TinyFishSearchSourceURLs(res.TinyFishSearchResponse)
	cache.CacheSources(sessionID, urls)

	var sb strings.Builder
	if grokErr != nil {
		fmt.Fprintf(&sb, "engine: TinyFish Search (%s; Grok pool fallback: %v)\n", res.KeyName, grokErr)
	} else {
		fmt.Fprintf(&sb, "engine: TinyFish Search (%s; no Grok endpoint configured)\n", res.KeyName)
	}
	fmt.Fprintf(&sb, "session_id: %s\nsources_count: %d\n\n", sessionID, len(urls))
	sb.WriteString(engine.FormatTinyFishSearchContent(res.TinyFishSearchResponse))
	return sb.String()
}

// formatExaResponse renders Exa Search results in the same envelope as the
// Grok branch and registers source URLs in the session cache.
func formatExaResponse(res *engine.ExaSearchResult, grokErr error, cache SourceCacher) string {
	sessionID := uuid.New().String()
	urls := engine.ExaSearchSourceURLs(res)
	cache.CacheSources(sessionID, urls)

	var sb strings.Builder
	if grokErr != nil {
		fmt.Fprintf(&sb, "engine: Exa Search (Grok pool fallback: %v)\n", grokErr)
	} else {
		sb.WriteString("engine: Exa Search (no Grok endpoint configured)\n")
	}
	fmt.Fprintf(&sb, "session_id: %s\nsources_count: %d\n\n", sessionID, len(urls))
	sb.WriteString(engine.FormatExaSearchContent(res))
	return sb.String()
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
		fmt.Fprintf(&sb, "engine: Tavily Search (Grok pool fallback: %v)\n", grokErr)
	} else {
		sb.WriteString("engine: Tavily Search (no Grok endpoint configured)\n")
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
