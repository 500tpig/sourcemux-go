package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/500tpig/sourcemux-go/internal/capability"
	"github.com/500tpig/sourcemux-go/internal/engine"
	"github.com/500tpig/sourcemux-go/internal/router"
	"github.com/500tpig/sourcemux-go/internal/router/adapters"
	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

const mcpDocsSearchExcerptRunes = 1600

type DocsSearchClients struct {
	Exa   *engine.ExaClient
	Cache SourceCacher
}

type DocsSearchResult struct {
	Query        string            `json:"query"`
	Engine       string            `json:"engine"`
	EndpointName string            `json:"endpoint_name,omitempty"`
	SessionID    string            `json:"session_id,omitempty"`
	Content      string            `json:"content"`
	SourceURLs   []string          `json:"source_urls"`
	SourcesCount int               `json:"sources_count"`
	Fallback     string            `json:"fallback,omitempty"`
	RouteTrace   router.RouteTrace `json:"route_trace,omitempty"`
}

func RegisterDocsSearch(s *mcpserver.MCPServer, exa *engine.ExaClient, cache SourceCacher) {
	tool := mcp.NewTool("docs_search",
		mcp.WithDescription("Search documentation sources through the configured Exa docs/web search fallback."),
		mcp.WithString("query", mcp.Required(), mcp.Description("Documentation question or task")),
		mcp.WithBoolean("include_trace", mcp.Description("Return full route trace in _meta.route_trace")),
	)
	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query, _ := req.Params.Arguments["query"].(string)
		includeTrace := boolArgOr(req.Params.Arguments, "include_trace", false)

		res, err := RunDocsSearch(ctx, DocsSearchClients{
			Exa:   exa,
			Cache: cache,
		}, DocsSearchOptions{
			Query: query,
		})
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		out := mcp.NewToolResultText(FormatDocsSearchResult(res))
		if includeTrace {
			out.Meta = map[string]any{"route_trace": res.RouteTrace}
		} else {
			out.Meta = map[string]any{"route_trace": res.RouteTrace.Compact()}
		}
		return out, nil
	})
}

type DocsSearchOptions struct {
	Query string
}

func RunDocsSearch(ctx context.Context, clients DocsSearchClients, opts DocsSearchOptions) (*DocsSearchResult, error) {
	query := strings.TrimSpace(opts.Query)
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}
	r := router.New(docsSearchProviders(clients)...)
	res, trace := r.Run(ctx, capability.DocsSearch, capability.Request{
		Query: query,
	})
	if strings.TrimSpace(res.Content) == "" {
		if detail := firstFailureDetail(trace); detail != "" {
			return nil, fmt.Errorf("docs search failed: %s", detail)
		}
		return nil, fmt.Errorf("docs search returned empty result and no provider configured")
	}
	sessionID := uuid.New().String()
	urls := sourceURLsFromCapability(res.Sources)
	cacheSources(clients.Cache, sessionID, urls)
	return &DocsSearchResult{
		Query:        query,
		Engine:       metadataString(res.Metadata, "engine", trace.FinalProvider),
		EndpointName: metadataString(res.Metadata, "endpoint_name", ""),
		SessionID:    sessionID,
		Content:      res.Content,
		SourceURLs:   urls,
		SourcesCount: len(urls),
		Fallback:     metadataString(res.Metadata, "fallback", ""),
		RouteTrace:   trace,
	}, nil
}

func FormatDocsSearchResult(res *DocsSearchResult) string {
	if res == nil {
		return ""
	}
	var sb strings.Builder
	if res.EndpointName != "" {
		fmt.Fprintf(&sb, "engine: %s (%s)\n", res.Engine, res.EndpointName)
	} else {
		fmt.Fprintf(&sb, "engine: %s\n", res.Engine)
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
	if content != "" {
		fmt.Fprintf(&sb, "\nsummary:\n%s", indentContinuation(clipRunes(content, mcpDocsSearchExcerptRunes), "  "))
	}
	return strings.TrimSpace(sb.String())
}

func docsSearchProviders(clients DocsSearchClients) []capability.Provider {
	var providers []capability.Provider
	if clients.Exa != nil {
		providers = append(providers, adapters.NewExaDocsSearch(clients.Exa))
	}
	return providers
}
