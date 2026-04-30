package server

import (
	"context"
	"os"
	"sync"

	"github.com/bettas/grok-search-go/internal/config"
	"github.com/bettas/grok-search-go/internal/engine"
	"github.com/bettas/grok-search-go/internal/tools"
	mcp "github.com/mark3labs/mcp-go/server"
)

// App holds shared state for all MCP tools.
type App struct {
	Cfg      *config.Config
	GrokPool *engine.GrokPool
	Tavily   *engine.TavilyClient
	Jina     *engine.JinaClient

	// Source cache: sessionID -> []string (URLs)
	SourcesMu sync.RWMutex
	Sources   map[string][]string
}

// Run starts the MCP server on stdio.
func Run(cfg *config.Config) error {
	app := &App{
		Cfg:      cfg,
		GrokPool: engine.NewGrokPool(cfg.GrokEndpoints),
		Sources:  make(map[string][]string),
	}
	app.GrokPool.OverallTimeout = cfg.GrokPoolTimeout

	if cfg.TavilyEnabled && cfg.TavilyAPIKey != "" {
		app.Tavily = engine.NewTavilyClient(cfg.TavilyAPIURL, cfg.TavilyAPIKey)
	}
	// Jina Reader works without a key — always enabled.
	app.Jina = engine.NewJinaClient(cfg.JinaAPIURL, cfg.JinaAPIKey)

	s := mcp.NewMCPServer(
		"grok-search",
		"0.2.0",
	)

	// Register tools
	tools.RegisterSearch(s, app.GrokPool, app.Tavily, app)
	tools.RegisterFetch(s, app.Jina, app.Tavily)
	tools.RegisterMap(s, app.Tavily)
	tools.RegisterSources(s, app)
	tools.RegisterConfig(s, cfg, app.GrokPool)
	tools.RegisterSearchPlanning(s)

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
