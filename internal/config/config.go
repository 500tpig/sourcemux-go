package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
		return nil, fmt.Errorf("no Grok endpoints configured: set GROK_ENDPOINTS_JSON, GROK_ENDPOINTS_FILE, GROK_API_URL + GROK_API_KEY, or place a JSON config at %s", defaultConfigPath())
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
		// Final fallback: optional default config file at the user's XDG config dir.
		return loadDefaultConfigFile()
	}
	return []engine.GrokEndpoint{ {
		Name:           envOr("GROK_NAME", "default"),
		BaseURL:        url,
		APIKey:         key,
		Model:          envOr("GROK_MODEL", "grok-3-mini"),
		SendSearchFlag: strings.ToLower(envOr("GROK_SEND_SEARCH_FLAG", "true")) == "true",
	} }, nil
}

// loadDefaultConfigFile attempts to read the default user-level config file at
// $XDG_CONFIG_HOME/grok-search/endpoints.json (or ~/.config/grok-search/endpoints.json
// if XDG_CONFIG_HOME is unset). A missing file returns (nil, nil); other errors
// (permission, malformed JSON, etc.) are surfaced so users notice typos early.
func loadDefaultConfigFile() ([]engine.GrokEndpoint, error) {
	path := defaultConfigPath()
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	eps, err := parseEndpoints(data)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return eps, nil
}

// defaultConfigPath returns the platform-default location of the optional
// endpoints config file: $XDG_CONFIG_HOME/grok-search/endpoints.json, falling
// back to $HOME/.config/grok-search/endpoints.json. Returns "" if no home
// directory can be resolved.
func defaultConfigPath() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "grok-search", "endpoints.json")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".config", "grok-search", "endpoints.json")
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
