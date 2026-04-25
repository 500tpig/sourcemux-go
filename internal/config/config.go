package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/bettas/grok-search-go/internal/engine"
)

// Config holds all runtime configuration, resolved from env vars.
type Config struct {
	// Grok endpoint pool, ordered by priority. Tried in order; first non-empty
	// success wins. Built from one of:
	//   - GROK_ENDPOINTS_JSON  (inline JSON array)
	//   - GROK_ENDPOINTS_FILE  (path to JSON file)
	//   - GROK_API_URL + GROK_API_KEY [+ GROK_MODEL] (legacy single endpoint)
	GrokEndpoints []engine.GrokEndpoint

	// Tavily — web_search final fallback + web_fetch / web_map source.
	TavilyAPIURL  string
	TavilyAPIKey  string
	TavilyEnabled bool

	// Jina Reader — primary web fetch (free, no key needed).
	JinaAPIURL string
	JinaAPIKey string

	// General
	Debug    bool
	LogLevel string
}

// Load reads environment variables and returns a validated Config.
//
// At least one Grok endpoint must resolve to a non-empty {baseURL, apiKey}.
func Load() (*Config, error) {
	endpoints, err := loadGrokEndpoints()
	if err != nil {
		return nil, err
	}
	if len(endpoints) == 0 {
		return nil, fmt.Errorf("no Grok endpoints configured: set GROK_ENDPOINTS_JSON, GROK_ENDPOINTS_FILE, or GROK_API_URL + GROK_API_KEY")
	}

	cfg := &Config{
		GrokEndpoints: endpoints,
		TavilyAPIURL:  envOr("TAVILY_API_URL", "https://api.tavily.com"),
		TavilyAPIKey:  os.Getenv("TAVILY_API_KEY"),
		TavilyEnabled: strings.ToLower(envOr("TAVILY_ENABLED", "true")) == "true",
		JinaAPIURL:    envOr("JINA_API_URL", "https://r.jina.ai"),
		JinaAPIKey:    os.Getenv("JINA_API_KEY"),
		Debug:         strings.ToLower(os.Getenv("GROK_DEBUG")) == "true",
		LogLevel:      envOr("GROK_LOG_LEVEL", "INFO"),
	}
	return cfg, nil
}

func loadGrokEndpoints() ([]engine.GrokEndpoint, error) {
	if raw := os.Getenv("GROK_ENDPOINTS_JSON"); raw != "" {
		eps, err := parseEndpoints([]byte(raw))
		if err != nil {
			return nil, fmt.Errorf("parse GROK_ENDPOINTS_JSON: %w", err)
		}
		return eps, nil
	}
	if path := os.Getenv("GROK_ENDPOINTS_FILE"); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read GROK_ENDPOINTS_FILE %q: %w", path, err)
		}
		eps, err := parseEndpoints(data)
		if err != nil {
			return nil, fmt.Errorf("parse GROK_ENDPOINTS_FILE: %w", err)
		}
		return eps, nil
	}
	// Legacy single-endpoint fallback.
	url := os.Getenv("GROK_API_URL")
	key := os.Getenv("GROK_API_KEY")
	if url == "" || key == "" {
		return nil, nil
	}
	return []engine.GrokEndpoint{ {
		Name:           envOr("GROK_NAME", "default"),
		BaseURL:        url,
		APIKey:         key,
		Model:          envOr("GROK_MODEL", "grok-3-mini"),
		SendSearchFlag: strings.ToLower(envOr("GROK_SEND_SEARCH_FLAG", "true")) == "true",
	} }, nil
}

func parseEndpoints(data []byte) ([]engine.GrokEndpoint, error) {
	var eps []engine.GrokEndpoint
	if err := json.Unmarshal(data, &eps); err != nil {
		return nil, err
	}
	for i, ep := range eps {
		if ep.BaseURL == "" || ep.APIKey == "" {
			return nil, fmt.Errorf("endpoint #%d (name=%q) missing baseURL or apiKey", i, ep.Name)
		}
		if ep.Name == "" {
			eps[i].Name = fmt.Sprintf("endpoint-%d", i)
		}
		if ep.Model == "" {
			eps[i].Model = "grok-3-mini"
		}
	}
	return eps, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
