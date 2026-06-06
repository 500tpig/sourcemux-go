package adapters

import (
	"context"
	"fmt"
	"strings"

	"github.com/500tpig/sourcemux-go/internal/capability"
	"github.com/500tpig/sourcemux-go/internal/engine"
)

type JinaFetchProvider struct {
	Client *engine.JinaClient
}

func NewJinaFetch(client *engine.JinaClient) *JinaFetchProvider {
	return &JinaFetchProvider{Client: client}
}

func (p *JinaFetchProvider) Name() string { return "jina-reader" }
func (p *JinaFetchProvider) Kind() capability.Kind {
	return capability.WebFetch
}

func (p *JinaFetchProvider) Try(ctx context.Context, req capability.Request) (capability.Result, error) {
	if p == nil || p.Client == nil {
		return capability.Result{}, fmt.Errorf("jina reader is not configured")
	}
	res, err := p.Client.Fetch(ctx, req.URL)
	if err != nil {
		return capability.Result{}, err
	}
	return fetchResult("Jina Reader", res.URL, res.Content), nil
}

type TinyFishFetchProvider struct {
	Pool *engine.TinyFishPool
}

func NewTinyFishFetch(pool *engine.TinyFishPool) *TinyFishFetchProvider {
	return &TinyFishFetchProvider{Pool: pool}
}

func (p *TinyFishFetchProvider) Name() string { return "tinyfish-fetch" }
func (p *TinyFishFetchProvider) Kind() capability.Kind {
	return capability.WebFetch
}
func (p *TinyFishFetchProvider) AttemptCount() int {
	if p == nil || p.Pool == nil {
		return 0
	}
	return p.Pool.Len()
}

func (p *TinyFishFetchProvider) Try(ctx context.Context, req capability.Request) (capability.Result, error) {
	if p == nil || p.Pool == nil || p.Pool.Len() == 0 {
		return capability.Result{}, fmt.Errorf("tinyfish pool is empty: no keys configured")
	}
	res, err := p.Pool.Fetch(ctx, engine.TinyFishFetchRequest{URLs: []string{req.URL}, Format: "markdown"})
	if err != nil {
		return capability.Result{}, err
	}
	content := engine.TinyFishFetchContent(res.TinyFishFetchResponse)
	resultURL := req.URL
	if len(res.Results) > 0 {
		if res.Results[0].FinalURL != "" {
			resultURL = res.Results[0].FinalURL
		} else if res.Results[0].URL != "" {
			resultURL = res.Results[0].URL
		}
	}
	return fetchResult("TinyFish Fetch ("+res.KeyName+")", resultURL, content), nil
}

type FirecrawlFetchProvider struct {
	Pool *engine.FirecrawlPool
}

func NewFirecrawlFetch(pool *engine.FirecrawlPool) *FirecrawlFetchProvider {
	return &FirecrawlFetchProvider{Pool: pool}
}

func (p *FirecrawlFetchProvider) Name() string { return "firecrawl-scrape" }
func (p *FirecrawlFetchProvider) Kind() capability.Kind {
	return capability.WebFetch
}
func (p *FirecrawlFetchProvider) AttemptCount() int {
	if p == nil || p.Pool == nil {
		return 0
	}
	return p.Pool.Len()
}

func (p *FirecrawlFetchProvider) Try(ctx context.Context, req capability.Request) (capability.Result, error) {
	if p == nil || p.Pool == nil || p.Pool.Len() == 0 {
		return capability.Result{}, fmt.Errorf("firecrawl pool is empty: no keys configured")
	}
	onlyClean := true
	removeBase64Images := true
	blockAds := true
	storeInCache := true
	res, err := p.Pool.Scrape(ctx, engine.FirecrawlScrapeRequest{
		URL:                req.URL,
		Formats:            []string{"markdown"},
		OnlyCleanContent:   &onlyClean,
		RemoveBase64Images: &removeBase64Images,
		BlockAds:           &blockAds,
		StoreInCache:       &storeInCache,
	})
	if err != nil {
		return capability.Result{}, err
	}
	resultURL := engine.FirecrawlScrapeResultURL(res.FirecrawlScrapeResult, req.URL)
	return fetchResult("Firecrawl Scrape ("+res.KeyName+")", resultURL, res.Data.Markdown), nil
}

func (p *FirecrawlFetchProvider) Classify(result capability.Result, err error) (capability.Outcome, capability.FallbackReason, string) {
	if err != nil {
		detail := err.Error()
		lower := strings.ToLower(detail)
		reason := capability.ReasonUpstreamError
		if strings.Contains(lower, "429") || strings.Contains(lower, "rate limit") || strings.Contains(lower, "too many requests") {
			reason = capability.ReasonRateLimited
		}
		if strings.Contains(lower, "timeout") || strings.Contains(lower, "deadline") {
			reason = capability.ReasonTimeout
		}
		return capability.Transient, reason, detail
	}
	if strings.TrimSpace(result.Content) == "" {
		return capability.Empty, capability.ReasonNoContent, "empty content"
	}
	return capability.OK, capability.ReasonNone, ""
}

type ExaContentsProvider struct {
	Client *engine.ExaClient
}

func NewExaContents(client *engine.ExaClient) *ExaContentsProvider {
	return &ExaContentsProvider{Client: client}
}

func (p *ExaContentsProvider) Name() string { return "exa-contents" }
func (p *ExaContentsProvider) Kind() capability.Kind {
	return capability.WebFetch
}

func (p *ExaContentsProvider) Try(ctx context.Context, req capability.Request) (capability.Result, error) {
	if p == nil || p.Client == nil {
		return capability.Result{}, fmt.Errorf("exa contents is not configured")
	}
	res, err := p.Client.Extract(ctx, req.URL)
	if err != nil {
		return capability.Result{}, err
	}
	return fetchResult("Exa Contents", res.URL, res.Content), nil
}

type TavilyExtractProvider struct {
	Client *engine.TavilyClient
}

func NewTavilyExtract(client *engine.TavilyClient) *TavilyExtractProvider {
	return &TavilyExtractProvider{Client: client}
}

func (p *TavilyExtractProvider) Name() string { return "tavily-extract" }
func (p *TavilyExtractProvider) Kind() capability.Kind {
	return capability.WebFetch
}

func (p *TavilyExtractProvider) Try(ctx context.Context, req capability.Request) (capability.Result, error) {
	if p == nil || p.Client == nil {
		return capability.Result{}, fmt.Errorf("tavily extract is not configured")
	}
	res, err := p.Client.Extract(ctx, req.URL)
	if err != nil {
		return capability.Result{}, err
	}
	return fetchResult("Tavily Extract", res.URL, res.Content), nil
}

func fetchResult(source, url, content string) capability.Result {
	return capability.Result{
		Content: content,
		Sources: []capability.Source{
			{URL: url},
		},
		Metadata: map[string]any{
			metaEngine: source,
			metaURL:    url,
		},
	}
}
