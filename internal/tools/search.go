package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/500tpig/sourcemux-go/internal/capability"
	"github.com/500tpig/sourcemux-go/internal/engine"
	"github.com/500tpig/sourcemux-go/internal/router"
	"github.com/500tpig/sourcemux-go/internal/router/adapters"
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
	Query        string            `json:"query"`
	Engine       string            `json:"engine"`
	EndpointName string            `json:"endpoint_name,omitempty"`
	Model        string            `json:"model,omitempty"`
	SessionID    string            `json:"session_id,omitempty"`
	Content      string            `json:"content"`
	SourceURLs   []string          `json:"source_urls"`
	SourcesCount int               `json:"sources_count"`
	Fallback     string            `json:"fallback,omitempty"`
	GrokError    string            `json:"grok_error,omitempty"`
	RouteTrace   router.RouteTrace `json:"route_trace,omitempty"`
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
		mcp.WithBoolean("include_trace", mcp.Description("Return full route trace in _meta.route_trace")),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query, _ := req.Params.Arguments["query"].(string)
		platform, _ := req.Params.Arguments["platform"].(string)
		model, _ := req.Params.Arguments["model"].(string)
		includeTrace := boolArgOr(req.Params.Arguments, "include_trace", false)

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
		out := mcp.NewToolResultText(FormatWebSearchResult(res))
		if includeTrace {
			out.Meta = map[string]any{"route_trace": res.RouteTrace}
		} else {
			out.Meta = map[string]any{"route_trace": res.RouteTrace.Compact()}
		}
		return out, nil
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

	r := router.New(searchProviders(clients)...)
	res, trace := r.Run(ctx, capability.MainSearch, capability.Request{
		Query: query,
		Options: map[string]any{
			"model": model,
		},
	})
	if strings.TrimSpace(res.Content) == "" {
		if detail := firstFailureDetail(trace); detail != "" {
			return nil, fmt.Errorf("search failed: %s", detail)
		}
		return nil, fmt.Errorf("search returned empty result and no fallback configured")
	}

	sessionID := uuid.New().String()
	urls := sourceURLsFromCapability(res.Sources)
	cacheSources(clients.Cache, sessionID, urls)
	return &WebSearchResult{
		Query:        query,
		Engine:       metadataString(res.Metadata, "engine", trace.FinalProvider),
		EndpointName: metadataString(res.Metadata, "endpoint_name", ""),
		Model:        metadataString(res.Metadata, "model", ""),
		SessionID:    sessionID,
		Content:      res.Content,
		SourceURLs:   urls,
		SourcesCount: len(urls),
		Fallback:     metadataString(res.Metadata, "fallback", ""),
		GrokError:    grokFailureDetail(trace),
		RouteTrace:   trace,
	}, nil
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
	if res.RouteTrace.AttemptsCount > 0 {
		fmt.Fprintf(&sb, "route: final_provider=%s fallback_triggered=%v attempts_count=%d\n",
			res.RouteTrace.FinalProvider, res.RouteTrace.FallbackTriggered, res.RouteTrace.AttemptsCount)
	}
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

func searchProviders(clients WebSearchClients) []capability.Provider {
	var providers []capability.Provider
	if clients.Pool != nil && clients.Pool.Len() > 0 {
		providers = append(providers, adapters.NewGrokSearch(clients.Pool))
	}
	if clients.TinyFish != nil && clients.TinyFish.Len() > 0 {
		providers = append(providers, adapters.NewTinyFishSearch(clients.TinyFish))
	}
	if clients.Exa != nil {
		providers = append(providers, adapters.NewExaSearch(clients.Exa))
	}
	if clients.Tavily != nil {
		providers = append(providers, adapters.NewTavilySearch(clients.Tavily))
	}
	return providers
}

func sourceURLsFromCapability(sources []capability.Source) []string {
	urls := make([]string, 0, len(sources))
	for _, s := range sources {
		if s.URL != "" {
			urls = append(urls, s.URL)
		}
	}
	return urls
}

func metadataString(metadata map[string]any, key, fallback string) string {
	if metadata == nil {
		return fallback
	}
	if v, ok := metadata[key].(string); ok && v != "" {
		return v
	}
	return fallback
}

func firstFailureDetail(trace router.RouteTrace) string {
	for _, d := range trace.Decisions {
		if d.FallbackDetail != "" {
			return d.FallbackDetail
		}
	}
	return ""
}

func grokFailureDetail(trace router.RouteTrace) string {
	for _, d := range trace.Decisions {
		if d.Provider == "grok-pool" && d.Status != "ok" {
			return d.FallbackDetail
		}
	}
	return ""
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
