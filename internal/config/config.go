package config

import (
	"fmt"
	"os"
	"strings"
)

// Config holds all runtime configuration, resolved from env vars.
type Config struct {
	// Grok (AI search)
	GrokAPIURL string
	GrokAPIKey string
	GrokModel  string

	// Tavily (web fetch + map)
	TavilyAPIURL  string
	TavilyAPIKey  string
	TavilyEnabled bool

	// Firecrawl (fallback scraper)
	FirecrawlAPIURL string
	FirecrawlAPIKey string

	// General
	Debug    bool
	LogLevel string
}

// Load reads environment variables and returns a validated Config.
// Priority: explicit env vars > GUDA_API_KEY derived values > defaults.
func Load() (*Config, error) {
	gudaKey := os.Getenv("GUDA_API_KEY")
	gudaBase := envOr("GUDA_BASE_URL", "https://code.guda.studio")

	cfg := &Config{
		GrokAPIURL:      envOrDerived("GROK_API_URL", gudaBase+"/grok/v1", gudaKey),
		GrokAPIKey:      envOrDerived("GROK_API_KEY", gudaKey, gudaKey),
		GrokModel:       envOr("GROK_MODEL", "grok-3-mini"),
		TavilyAPIURL:    envOrDerived("TAVILY_API_URL", gudaBase+"/tavily", gudaKey),
		TavilyAPIKey:    envOrDerived("TAVILY_API_KEY", gudaKey, gudaKey),
		TavilyEnabled:   strings.ToLower(envOr("TAVILY_ENABLED", "true")) == "true",
		FirecrawlAPIURL: envOrDerived("FIRECRAWL_API_URL", gudaBase+"/firecrawl", gudaKey),
		FirecrawlAPIKey: envOrDerived("FIRECRAWL_API_KEY", gudaKey, gudaKey),
		Debug:           strings.ToLower(os.Getenv("GROK_DEBUG")) == "true",
		LogLevel:        envOr("GROK_LOG_LEVEL", "INFO"),
	}

	if cfg.GrokAPIURL == "" || cfg.GrokAPIKey == "" {
		return nil, fmt.Errorf("GROK_API_URL and GROK_API_KEY (or GUDA_API_KEY) are required")
	}
	return cfg, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// envOrDerived returns explicit env, else derived value (only if gudaKey is set).
func envOrDerived(key, derived, gudaKey string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	if gudaKey != "" {
		return derived
	}
	return ""
}
