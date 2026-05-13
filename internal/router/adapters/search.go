package adapters

import (
	"context"
	"fmt"
	"strings"

	"github.com/500tpig/grok-search-go/internal/capability"
	"github.com/500tpig/grok-search-go/internal/engine"
)

const (
	metaEngine       = "engine"
	metaEndpointName = "endpoint_name"
	metaModel        = "model"
	metaFallback     = "fallback"
	metaURL          = "url"
)

type GrokSearchProvider struct {
	Pool *engine.GrokPool
}

func NewGrokSearch(pool *engine.GrokPool) *GrokSearchProvider {
	return &GrokSearchProvider{Pool: pool}
}

func (p *GrokSearchProvider) Name() string { return "grok-pool" }
func (p *GrokSearchProvider) Kind() capability.Kind {
	return capability.MainSearch
}
func (p *GrokSearchProvider) AttemptCount() int {
	if p == nil || p.Pool == nil {
		return 0
	}
	return p.Pool.Len()
}

func (p *GrokSearchProvider) Try(ctx context.Context, req capability.Request) (capability.Result, error) {
	if p == nil || p.Pool == nil || p.Pool.Len() == 0 {
		return capability.Result{}, fmt.Errorf("grok pool is empty: no endpoints configured")
	}
	res, err := p.Pool.SearchWithModel(ctx, req.Query, stringOption(req, "model"))
	if err != nil {
		return capability.Result{}, err
	}
	return capability.Result{
		Content: res.Content,
		Sources: sourcesFromURLs(res.SourceURLs),
		Metadata: map[string]any{
			metaEngine:       res.EndpointName,
			metaEndpointName: res.EndpointName,
			metaModel:        res.EndpointModel,
		},
	}, nil
}

type TinyFishSearchProvider struct {
	Pool *engine.TinyFishPool
}

func NewTinyFishSearch(pool *engine.TinyFishPool) *TinyFishSearchProvider {
	return &TinyFishSearchProvider{Pool: pool}
}

func (p *TinyFishSearchProvider) Name() string { return "tinyfish-search" }
func (p *TinyFishSearchProvider) Kind() capability.Kind {
	return capability.MainSearch
}
func (p *TinyFishSearchProvider) AttemptCount() int {
	if p == nil || p.Pool == nil {
		return 0
	}
	return p.Pool.Len()
}

func (p *TinyFishSearchProvider) Try(ctx context.Context, req capability.Request) (capability.Result, error) {
	if p == nil || p.Pool == nil || p.Pool.Len() == 0 {
		return capability.Result{}, fmt.Errorf("tinyfish pool is empty: no keys configured")
	}
	res, err := p.Pool.Search(ctx, engine.TinyFishSearchRequest{Query: req.Query})
	if err != nil {
		return capability.Result{}, err
	}
	return capability.Result{
		Content: engine.FormatTinyFishSearchContent(res.TinyFishSearchResponse),
		Sources: sourcesFromURLs(engine.TinyFishSearchSourceURLs(res.TinyFishSearchResponse)),
		Metadata: map[string]any{
			metaEngine:       "TinyFish Search",
			metaEndpointName: res.KeyName,
			metaFallback:     "tinyfish",
		},
	}, nil
}

type ExaSearchProvider struct {
	Client *engine.ExaClient
	kind   capability.Kind
	name   string
}

func NewExaSearch(client *engine.ExaClient) *ExaSearchProvider {
	return &ExaSearchProvider{Client: client, kind: capability.MainSearch, name: "exa-search"}
}

func NewExaDocsSearch(client *engine.ExaClient) *ExaSearchProvider {
	return &ExaSearchProvider{Client: client, kind: capability.DocsSearch, name: "exa-docs-search"}
}

func (p *ExaSearchProvider) Name() string {
	if p.name != "" {
		return p.name
	}
	return "exa-search"
}
func (p *ExaSearchProvider) Kind() capability.Kind {
	if p.kind != "" {
		return p.kind
	}
	return capability.MainSearch
}

func (p *ExaSearchProvider) Try(ctx context.Context, req capability.Request) (capability.Result, error) {
	if p == nil || p.Client == nil {
		return capability.Result{}, fmt.Errorf("exa search is not configured")
	}
	res, err := p.Client.Search(ctx, req.Query)
	if err != nil {
		return capability.Result{}, err
	}
	return capability.Result{
		Content: engine.FormatExaSearchContent(res),
		Sources: sourcesFromURLs(engine.ExaSearchSourceURLs(res)),
		Metadata: map[string]any{
			metaEngine:   "Exa Search",
			metaFallback: "exa",
		},
	}, nil
}

type TavilySearchProvider struct {
	Client *engine.TavilyClient
}

func NewTavilySearch(client *engine.TavilyClient) *TavilySearchProvider {
	return &TavilySearchProvider{Client: client}
}

func (p *TavilySearchProvider) Name() string { return "tavily-search" }
func (p *TavilySearchProvider) Kind() capability.Kind {
	return capability.MainSearch
}

func (p *TavilySearchProvider) Try(ctx context.Context, req capability.Request) (capability.Result, error) {
	if p == nil || p.Client == nil {
		return capability.Result{}, fmt.Errorf("tavily search is not configured")
	}
	res, err := p.Client.Search(ctx, req.Query)
	if err != nil {
		return capability.Result{}, err
	}
	return capability.Result{
		Content: formatTavilySearchContent(res),
		Sources: sourcesFromURLs(tavilySearchSourceURLs(res)),
		Metadata: map[string]any{
			metaEngine:   "Tavily Search",
			metaFallback: "tavily",
		},
	}, nil
}

func sourcesFromURLs(urls []string) []capability.Source {
	out := make([]capability.Source, 0, len(urls))
	for _, u := range urls {
		if u != "" {
			out = append(out, capability.Source{URL: u})
		}
	}
	return out
}

func stringOption(req capability.Request, key string) string {
	if req.Options == nil {
		return ""
	}
	if v, ok := req.Options[key].(string); ok {
		return v
	}
	return ""
}

func tavilySearchSourceURLs(res *engine.TavilySearchResult) []string {
	if res == nil {
		return nil
	}
	urls := make([]string, 0, len(res.Results))
	for _, r := range res.Results {
		if r.URL != "" {
			urls = append(urls, r.URL)
		}
	}
	return urls
}

func formatTavilySearchContent(res *engine.TavilySearchResult) string {
	if res == nil {
		return ""
	}
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
			fmt.Fprintf(&sb, "- %s - %s\n", title, r.URL)
		}
	}
	return strings.TrimSpace(sb.String())
}
