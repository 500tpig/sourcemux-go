package server

import (
	"context"
	"fmt"
	"sync"

	"github.com/bettas/grok-search-go/internal/config"
	"github.com/bettas/grok-search-go/internal/engine"
	"github.com/bettas/grok-search-go/internal/tools"
	mcp "github.com/mark3labs/mcp-go/server"
)

// App holds shared state for all MCP tools.
type App struct {
	Cfg       *config.Config
	Grok      *engine.GrokClient
	Tavily    *engine.TavilyClient
	Firecrawl *engine.FirecrawlClient

	// Source cache: sessionID -> []string (URLs)
	SourcesMu sync.RWMutex
	Sources   map[string][]string
}

// Run starts the MCP server on stdio.
func Run(cfg *config.Config) error {
	app := &App{
		Cfg:     cfg,
		Grok:    engine.NewGrokClient(cfg.GrokAPIURL, cfg.GrokAPIKey, cfg.GrokModel),
		Sources: make(map[string][]string),
	}

	if cfg.TavilyEnabled && cfg.TavilyAPIKey != "" {
		app.Tavily = engine.NewTavilyClient(cfg.TavilyAPIURL, cfg.TavilyAPIKey)
	}
	if cfg.FirecrawlAPIKey != "" {
		app.Firecrawl = engine.NewFirecrawlClient(cfg.FirecrawlAPIURL, cfg.FirecrawlAPIKey)
	}

	s := mcp.NewMCPServer(
		"grok-search",
		"0.1.0",
	)

	// Register tools
	tools.RegisterSearch(s, app.Grok, app)
	tools.RegisterFetch(s, app.Tavily, app.Firecrawl)
	tools.RegisterMap(s, app.Tavily)
	tools.RegisterSources(s, app)
	tools.RegisterConfig(s, cfg, app.Grok)

	// Serve on stdio
	fmt.Fprintln(nil) // placeholder — real stdio transport below
	stdioServer := mcp.NewStdioServer(s)
	return stdioServer.Listen(context.Background(), nil)
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
