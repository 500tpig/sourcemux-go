package server

import (
	"context"
	"os"
	"sync"

	"github.com/500tpig/sourcemux-go/internal/config"
	"github.com/500tpig/sourcemux-go/internal/engine"
	"github.com/500tpig/sourcemux-go/internal/tools"
	mcp "github.com/mark3labs/mcp-go/server"
)

// App holds shared state for all MCP tools.
type App struct {
	Cfg           *config.Config
	GrokPool      *engine.GrokPool
	ReasoningPool *engine.ReasoningPool
	Tavily        *engine.TavilyClient
	Exa           *engine.ExaClient
	Jina          *engine.JinaClient
	TinyFish      *engine.TinyFishPool

	// Source cache: sessionID -> []string (URLs)
	SourcesMu sync.RWMutex
	Sources   map[string][]string
}

// Run starts the MCP server on stdio.
func Run(cfg *config.Config) error {
	app := &App{
		Cfg:           cfg,
		GrokPool:      engine.NewGrokPool(cfg.GrokEndpoints),
		ReasoningPool: engine.NewReasoningPool(cfg.ReasoningEndpoints),
		Sources:       make(map[string][]string),
	}
	app.GrokPool.OverallTimeout = cfg.GrokPoolTimeout

	if cfg.TavilyEnabled && cfg.TavilyAPIKey != "" {
		app.Tavily = engine.NewTavilyClient(cfg.TavilyAPIURL, cfg.TavilyAPIKey)
	}
	if cfg.ExaEnabled && cfg.ExaAPIKey != "" {
		app.Exa = engine.NewExaClient(cfg.ExaAPIURL, cfg.ExaAPIKey)
	}
	// Jina Reader works without a key — always enabled.
	app.Jina = engine.NewJinaClient(cfg.JinaAPIURL, cfg.JinaAPIKey)
	if cfg.TinyFishEnabled && len(cfg.TinyFishKeys) > 0 {
		app.TinyFish = engine.NewTinyFishPool(cfg.TinyFishKeys, cfg.TinyFishSearchURL, cfg.TinyFishFetchURL)
	}

	s := mcp.NewMCPServer(
		"sourcemux",
		"0.2.0",
	)

	// Register tools
	tools.RegisterSearch(s, app.GrokPool, app.TinyFish, app.Exa, app.Tavily, app, cfg.SearchPolicy)
	tools.RegisterDocsSearch(s, app.Exa, app)
	tools.RegisterFetch(s, app.Jina, app.TinyFish, app.Exa, app.Tavily)
	tools.RegisterExaSearchAdvanced(s, app.Exa)
	tools.RegisterExaContentsAdvanced(s, app.Exa)
	tools.RegisterMap(s, app.Tavily)
	tools.RegisterCrawl(s, app.Tavily)
	tools.RegisterSources(s, app)
	tools.RegisterConfig(s, cfg, app.GrokPool)
	tools.RegisterSearchPlanning(s)
	researchExecutor := tools.NewResearchExecutor(tools.ResearchExecutorDeps{
		Search: tools.WebSearchClients{
			Pool:         app.GrokPool,
			TinyFish:     app.TinyFish,
			Exa:          app.Exa,
			Tavily:       app.Tavily,
			Cache:        app,
			SearchPolicy: cfg.SearchPolicy,
		},
		Fetch: tools.WebFetchClients{
			Jina:     app.Jina,
			TinyFish: app.TinyFish,
			Exa:      app.Exa,
			Tavily:   app.Tavily,
		},
		Sources: app,
		Mapper:  app.Tavily,
		Crawler: app.Tavily,
	})
	tools.RegisterResearchRun(s, researchExecutor)
	tools.RegisterSmartAnswer(s, &tools.SmartAnswerer{
		Researcher: researchExecutor,
		Reasoner:   app.ReasoningPool,
	})

	// Serve on stdio
	stdioServer := mcp.NewStdioServer(s)
	return stdioServer.Listen(context.Background(), os.Stdin, os.Stdout)
}

// CacheSources stores source URLs for a session ID.
func (a *App) CacheSources(sessionID string, urls []string) {
	a.SourcesMu.Lock()
	defer a.SourcesMu.Unlock()
	a.Sources[sessionID] = urls
}

// GetSources retrieves cached source URLs for a session ID.
func (a *App) GetSources(sessionID string) ([]string, bool) {
	a.SourcesMu.RLock()
	defer a.SourcesMu.RUnlock()
	urls, ok := a.Sources[sessionID]
	return urls, ok
}
