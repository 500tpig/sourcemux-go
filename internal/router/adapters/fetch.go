package adapters

import (
	"context"
	"fmt"

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
