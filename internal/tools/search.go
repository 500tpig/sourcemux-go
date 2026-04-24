package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bettas/grok-search-go/internal/engine"
	"github.com/google/uuid"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/mark3labs/mcp-go/mcp"
)

// SourceCacher is implemented by server.App to cache sources.
type SourceCacher interface {
	CacheSources(sessionID string, urls []string)
}

// RegisterSearch registers the web_search tool.
func RegisterSearch(s *mcpserver.MCPServer, grok *engine.GrokClient, cache SourceCacher) {
	tool := mcp.NewTool("web_search",
		mcp.WithDescription("AI-powered web search via Grok. Returns answer text and a session_id for source retrieval."),
		mcp.WithString("query", mcp.Required(), mcp.Description("Search query")),
		mcp.WithString("platform", mcp.Description("Focus platform, e.g. 'Twitter', 'GitHub, Reddit'")),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query, _ := req.Params.Arguments["query"].(string)
		if query == "" {
			return mcp.NewToolResultError("query is required"), nil
		}

		// Inject time context for temporal queries
		query = injectTimeContext(query)

		platform, _ := req.Params.Arguments["platform"].(string)
		if platform != "" {
			query = fmt.Sprintf("[Focus: %s] %s", platform, query)
		}

		result, err := grok.Search(ctx, query)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("search failed: %v", err)), nil
		}

		sessionID := uuid.New().String()
		cache.CacheSources(sessionID, result.SourceURLs)

		response := fmt.Sprintf(
			"session_id: %s\nsources_count: %d\n\n%s",
			sessionID, result.SourcesCount, result.Content,
		)

		return mcp.NewToolResultText(response), nil
	})
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
