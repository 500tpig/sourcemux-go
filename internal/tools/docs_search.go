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
	Context7 []*engine.Context7Client
	Exa      *engine.ExaClient
	Cache    SourceCacher
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

func RegisterDocsSearch(s *mcpserver.MCPServer, context7 []*engine.Context7Client, exa *engine.ExaClient, cache SourceCacher) {
	tool := mcp.NewTool("docs_search",
		mcp.WithDescription("Search documentation sources. Explicit library_id or library_name requests try Context7 first, then fall back to Exa docs search when configured."),
		mcp.WithString("query", mcp.Required(), mcp.Description("Documentation question or task")),
		mcp.WithString("library_id", mcp.Description("Optional Context7-compatible library ID, e.g. /vercel/next.js")),
		mcp.WithString("library_name", mcp.Description("Optional library name to resolve with Context7, e.g. next.js")),
		mcp.WithString("provider", mcp.Description("Optional named Context7 provider instance")),
		mcp.WithBoolean("fast", mcp.Description("Use Context7 fast mode when Context7 is applicable")),
		mcp.WithBoolean("include_trace", mcp.Description("Return full route trace in _meta.route_trace")),
	)
	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query, _ := req.Params.Arguments["query"].(string)
		libraryID, _ := req.Params.Arguments["library_id"].(string)
		libraryName, _ := req.Params.Arguments["library_name"].(string)
		provider, _ := req.Params.Arguments["provider"].(string)
		fast := boolArgOr(req.Params.Arguments, "fast", false)
		includeTrace := boolArgOr(req.Params.Arguments, "include_trace", false)

		res, err := RunDocsSearch(ctx, DocsSearchClients{
			Context7: context7,
			Exa:      exa,
			Cache:    cache,
		}, DocsSearchOptions{
			Query:       query,
			LibraryID:   libraryID,
			LibraryName: libraryName,
			Provider:    provider,
			Fast:        fast,
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
	Query       string
	LibraryID   string
	LibraryName string
	Provider    string
	Fast        bool
}

func RunDocsSearch(ctx context.Context, clients DocsSearchClients, opts DocsSearchOptions) (*DocsSearchResult, error) {
	query := strings.TrimSpace(opts.Query)
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}
	r := router.New(docsSearchProviders(clients)...)
	res, trace := r.Run(ctx, capability.DocsSearch, capability.Request{
		Query: query,
		Options: map[string]any{
			"library_id":   strings.TrimSpace(opts.LibraryID),
			"library_name": strings.TrimSpace(opts.LibraryName),
			"provider":     strings.TrimSpace(opts.Provider),
			"fast":         opts.Fast,
		},
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
	if len(clients.Context7) > 0 {
		providers = append(providers, adapters.NewContext7Docs(clients.Context7))
	}
	if clients.Exa != nil {
		providers = append(providers, adapters.NewExaDocsSearch(clients.Exa))
	}
	return providers
}
