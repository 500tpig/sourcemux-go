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

const mcpSearchExcerptRunes = 1400

// SourceCacher is implemented by server.App to cache sources.
type SourceCacher interface {
	CacheSources(sessionID string, urls []string)
}

// WebSearchClients groups the production search providers in the same order
// used by the web_search MCP tool and CLI search command.
type WebSearchClients struct {
	Pool     *engine.GrokPool
	TinyFish *engine.TinyFishPool
	Exa      *engine.ExaClient
	Tavily   *engine.TavilyClient
	Cache    SourceCacher
}

// WebSearchResult is the provider-agnostic result envelope shared by MCP, CLI,
// and the higher-level research workflow.
type WebSearchResult struct {
	Query        string   `json:"query"`
	Engine       string   `json:"engine"`
	EndpointName string   `json:"endpoint_name,omitempty"`
	Model        string   `json:"model,omitempty"`
	SessionID    string   `json:"session_id,omitempty"`
	Content      string   `json:"content"`
	SourceURLs   []string `json:"source_urls"`
	SourcesCount int      `json:"sources_count"`
	Fallback     string   `json:"fallback,omitempty"`
	GrokError    string   `json:"grok_error,omitempty"`
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
		platform, _ := req.Params.Arguments["platform"].(string)
		model, _ := req.Params.Arguments["model"].(string)

		res, err := RunWebSearch(ctx, WebSearchClients{
			Pool:     pool,
			TinyFish: tinyfish,
			Exa:      exa,
			Tavily:   tavily,
			Cache:    cache,
		}, query, platform, model)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(FormatWebSearchResult(res)), nil
	})
}

// RunWebSearch executes the production web_search routing chain. Keep this as
// the single shared implementation so higher-level workflows do not fork the
// provider fallback order.
func RunWebSearch(ctx context.Context, clients WebSearchClients, query, platform, model string) (*WebSearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}

	// Inject time context for temporal queries.
	query = injectTimeContext(query)

	if platform = strings.TrimSpace(platform); platform != "" {
		query = fmt.Sprintf("[Focus: %s] %s", platform, query)
	}

	// 1) Try Grok pool.
	var poolErr error
	if clients.Pool != nil && clients.Pool.Len() > 0 {
		res, err := clients.Pool.SearchWithModel(ctx, query, model)
		if err == nil && res != nil && res.Content != "" {
			sessionID := uuid.New().String()
			cacheSources(clients.Cache, sessionID, res.SourceURLs)
			return &WebSearchResult{
				Query:        query,
				Engine:       res.EndpointName,
				EndpointName: res.EndpointName,
				Model:        res.EndpointModel,
				SessionID:    sessionID,
				Content:      res.Content,
				SourceURLs:   res.SourceURLs,
				SourcesCount: res.SourcesCount,
			}, nil
		}
		poolErr = err
	}

	// 2) Fallback to TinyFish Search.
	if clients.TinyFish != nil && clients.TinyFish.Len() > 0 {
		if tres, terr := clients.TinyFish.Search(ctx, engine.TinyFishSearchRequest{Query: query}); terr == nil && tres != nil {
			return webSearchTinyFishResult(query, tres, poolErr, clients.Cache), nil
		}
	}

	// 3) Fallback to Exa Search.
	if clients.Exa != nil {
		if eres, eerr := clients.Exa.Search(ctx, query); eerr == nil && eres != nil {
			return webSearchExaResult(query, eres, poolErr, clients.Cache), nil
		}
	}

	// 4) Fallback to Tavily Search.
	if clients.Tavily != nil {
		if tres, terr := clients.Tavily.Search(ctx, query); terr == nil && tres != nil {
			return webSearchTavilyResult(query, tres, poolErr, clients.Cache), nil
		}
	}

	if poolErr != nil {
		return nil, fmt.Errorf("search failed: %v", poolErr)
	}
	return nil, fmt.Errorf("search returned empty result and no fallback configured")
}

func webSearchTinyFishResult(query string, res *engine.TinyFishPoolSearchResult, grokErr error, cache SourceCacher) *WebSearchResult {
	sessionID := uuid.New().String()
	urls := engine.TinyFishSearchSourceURLs(res.TinyFishSearchResponse)
	cacheSources(cache, sessionID, urls)

	out := &WebSearchResult{
		Query:        query,
		Engine:       "TinyFish Search",
		EndpointName: res.KeyName,
		SessionID:    sessionID,
		Content:      engine.FormatTinyFishSearchContent(res.TinyFishSearchResponse),
		SourceURLs:   urls,
		SourcesCount: len(urls),
		Fallback:     "tinyfish",
	}
	if grokErr != nil {
		out.GrokError = grokErr.Error()
	}
	return out
}

func webSearchExaResult(query string, res *engine.ExaSearchResult, grokErr error, cache SourceCacher) *WebSearchResult {
	sessionID := uuid.New().String()
	urls := engine.ExaSearchSourceURLs(res)
	cacheSources(cache, sessionID, urls)

	out := &WebSearchResult{
		Query:        query,
		Engine:       "Exa Search",
		SessionID:    sessionID,
		Content:      engine.FormatExaSearchContent(res),
		SourceURLs:   urls,
		SourcesCount: len(urls),
		Fallback:     "exa",
	}
	if grokErr != nil {
		out.GrokError = grokErr.Error()
	}
	return out
}

func webSearchTavilyResult(query string, res *engine.TavilySearchResult, grokErr error, cache SourceCacher) *WebSearchResult {
	sessionID := uuid.New().String()
	urls := make([]string, 0, len(res.Results))
	for _, r := range res.Results {
		if r.URL != "" {
			urls = append(urls, r.URL)
		}
	}
	cacheSources(cache, sessionID, urls)

	var sb strings.Builder
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

	out := &WebSearchResult{
		Query:        query,
		Engine:       "Tavily Search",
		SessionID:    sessionID,
		Content:      strings.TrimSpace(sb.String()),
		SourceURLs:   urls,
		SourcesCount: len(urls),
		Fallback:     "tavily",
	}
	if grokErr != nil {
		out.GrokError = grokErr.Error()
	}
	return out
}

// FormatWebSearchResult renders a thin MCP envelope. Full search content stays
// on the CLI/JSON surfaces; MCP should remain compact and defer URL inspection
// to get_sources(session_id).
func FormatWebSearchResult(res *WebSearchResult) string {
	if res == nil {
		return ""
	}
	var sb strings.Builder
	switch res.Fallback {
	case "tinyfish":
		fmt.Fprintf(&sb, "engine: TinyFish Search (%s; %s)\n", res.EndpointName, grokFallbackText(res.GrokError))
	case "exa":
		fmt.Fprintf(&sb, "engine: Exa Search (%s)\n", grokFallbackText(res.GrokError))
	case "tavily":
		fmt.Fprintf(&sb, "engine: Tavily Search (%s)\n", grokFallbackText(res.GrokError))
	default:
		fmt.Fprintf(&sb, "engine: %s (%s)\n", res.Engine, res.Model)
	}
	if res.SessionID != "" {
		fmt.Fprintf(&sb, "session_id: %s\n", res.SessionID)
	}
	fmt.Fprintf(&sb, "sources_count: %d\n", res.SourcesCount)
	if res.SessionID != "" && res.SourcesCount > 0 {
		sb.WriteString("sources: call get_sources(session_id) for URLs\n")
	}
	content := strings.TrimSpace(res.Content)
	if content == "" {
		return strings.TrimSpace(sb.String())
	}
	fmt.Fprintf(&sb, "content_chars: %d\n\nsummary:\n%s",
		len([]rune(content)),
		indentContinuation(clipRunes(content, mcpSearchExcerptRunes), "  "),
	)
	return sb.String()
}

func grokFallbackText(grokErr string) string {
	if grokErr != "" {
		return "Grok pool fallback: " + grokErr
	}
	return "no Grok endpoint configured"
}

func cacheSources(cache SourceCacher, sessionID string, urls []string) {
	if cache != nil {
		cache.CacheSources(sessionID, urls)
	}
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
