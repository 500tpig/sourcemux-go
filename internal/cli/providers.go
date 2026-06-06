package cli

import (
	"github.com/500tpig/sourcemux-go/internal/config"
	"github.com/500tpig/sourcemux-go/internal/engine"
	"github.com/500tpig/sourcemux-go/internal/tools"
)

func buildWebSearchClients(cfg *config.Config, cache tools.SourceCacher) tools.WebSearchClients {
	pool := engine.NewGrokPool(cfg.GrokEndpoints)
	pool.OverallTimeout = cfg.GrokPoolTimeout

	var tavily *engine.TavilyClient
	if cfg.TavilyEnabled && cfg.TavilyAPIKey != "" {
		tavily = engine.NewTavilyClient(cfg.TavilyAPIURL, cfg.TavilyAPIKey)
	}
	var exa *engine.ExaClient
	if cfg.ExaEnabled && cfg.ExaAPIKey != "" {
		exa = engine.NewExaClient(cfg.ExaAPIURL, cfg.ExaAPIKey)
	}
	var tinyfish *engine.TinyFishPool
	if cfg.TinyFishEnabled && len(cfg.TinyFishKeys) > 0 {
		tinyfish = engine.NewTinyFishPool(cfg.TinyFishKeys, cfg.TinyFishSearchURL, cfg.TinyFishFetchURL)
	}

	return tools.WebSearchClients{
		Pool:         pool,
		TinyFish:     tinyfish,
		Exa:          exa,
		Tavily:       tavily,
		Cache:        cache,
		SearchPolicy: cfg.SearchPolicy,
	}
}

func buildWebFetchClients(cfg *config.Config) tools.WebFetchClients {
	var tavily *engine.TavilyClient
	if cfg.TavilyEnabled && cfg.TavilyAPIKey != "" {
		tavily = engine.NewTavilyClient(cfg.TavilyAPIURL, cfg.TavilyAPIKey)
	}
	var exa *engine.ExaClient
	if cfg.ExaEnabled && cfg.ExaAPIKey != "" {
		exa = engine.NewExaClient(cfg.ExaAPIURL, cfg.ExaAPIKey)
	}
	var tinyfish *engine.TinyFishPool
	if cfg.TinyFishEnabled && len(cfg.TinyFishKeys) > 0 {
		tinyfish = engine.NewTinyFishPool(cfg.TinyFishKeys, cfg.TinyFishSearchURL, cfg.TinyFishFetchURL)
	}
	var firecrawl *engine.FirecrawlPool
	if cfg.FirecrawlEnabled && len(cfg.FirecrawlKeys) > 0 {
		firecrawl = engine.NewFirecrawlPool(cfg.FirecrawlKeys, cfg.FirecrawlAPIURL)
	}
	return tools.WebFetchClients{
		Jina:        engine.NewJinaClient(cfg.JinaAPIURL, cfg.JinaAPIKey),
		TinyFish:    tinyfish,
		Firecrawl:   firecrawl,
		Exa:         exa,
		Tavily:      tavily,
		Order:       cfg.WebFetchOrder,
		StrictOrder: cfg.WebFetchStrictOrder,
	}
}

func buildDocsSearchClients(cfg *config.Config, cache tools.SourceCacher) tools.DocsSearchClients {
	var exa *engine.ExaClient
	if cfg.ExaEnabled && cfg.ExaAPIKey != "" {
		exa = engine.NewExaClient(cfg.ExaAPIURL, cfg.ExaAPIKey)
	}
	return tools.DocsSearchClients{
		Exa:   exa,
		Cache: cache,
	}
}

func buildFirecrawlClient(cfg *config.Config) *engine.FirecrawlClient {
	if cfg == nil || !cfg.FirecrawlEnabled || len(cfg.FirecrawlKeys) == 0 {
		return nil
	}
	return engine.NewFirecrawlClient(cfg.FirecrawlAPIURL, cfg.FirecrawlKeys[0].APIKey)
}
