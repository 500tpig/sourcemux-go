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
		Pool:     pool,
		TinyFish: tinyfish,
		Exa:      exa,
		Tavily:   tavily,
		Cache:    cache,
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
	return tools.WebFetchClients{
		Jina:     engine.NewJinaClient(cfg.JinaAPIURL, cfg.JinaAPIKey),
		TinyFish: tinyfish,
		Exa:      exa,
		Tavily:   tavily,
	}
}

func buildContext7Clients(cfg *config.Config) []*engine.Context7Client {
	clients := make([]*engine.Context7Client, 0, len(cfg.Context7Endpoints))
	for _, endpoint := range cfg.Context7Endpoints {
		clients = append(clients, engine.NewContext7Client(endpoint))
	}
	return clients
}

func buildDocsSearchClients(cfg *config.Config, cache tools.SourceCacher) tools.DocsSearchClients {
	var exa *engine.ExaClient
	if cfg.ExaEnabled && cfg.ExaAPIKey != "" {
		exa = engine.NewExaClient(cfg.ExaAPIURL, cfg.ExaAPIKey)
	}
	return tools.DocsSearchClients{
		Context7: buildContext7Clients(cfg),
		Exa:      exa,
		Cache:    cache,
	}
}
