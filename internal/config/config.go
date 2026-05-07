package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/bettas/grok-search-go/internal/engine"
)

// Config holds all runtime configuration, resolved from env vars and optional
// user-level config files.
type Config struct {
	// Grok endpoint pool, ordered by priority. Tried in order; first non-empty
	// success wins. Built from one of:
	//   - GROK_ENDPOINTS_JSON  (inline JSON array)
	//   - GROK_ENDPOINTS_FILE  (path to JSON file)
	//   - GROK_API_URL + GROK_API_KEY [+ GROK_MODEL] (legacy single endpoint)
	GrokEndpoints []engine.GrokEndpoint

	// Tavily — web_search final fallback + web_fetch / web_map / web_crawl source.
	TavilyAPIURL  string
	TavilyAPIKey  string
	TavilyEnabled bool

	// Exa — source-first search fallback + web_fetch extraction fallback.
	ExaAPIURL  string
	ExaAPIKey  string
	ExaEnabled bool

	// Jina Reader — primary web fetch (free, no key needed).
	JinaAPIURL string
	JinaAPIKey string

	// TinyFish — browser-rendered Search/Fetch production fallback.
	TinyFishEnabled   bool
	TinyFishKeys      []engine.TinyFishKey
	TinyFishSearchURL string
	TinyFishFetchURL  string

	// General
	Debug    bool
	LogLevel string

	// GrokPoolTimeout caps the total wall-clock budget GrokPool.Search will
	// spend across all endpoints + retries. 0 means no cap (legacy behavior).
	// Source env: GROK_POOL_TIMEOUT_SEC.
	GrokPoolTimeout time.Duration
}

type fileConfig struct {
	GrokEndpoints []engine.GrokEndpoint `json:"grokEndpoints"`
	Endpoints     []engine.GrokEndpoint `json:"endpoints"`

	Tavily serviceFileConfig `json:"tavily"`
	Exa    serviceFileConfig `json:"exa"`
	Jina   serviceFileConfig `json:"jina"`

	TinyFish tinyFishFileConfig `json:"tinyfish"`

	Debug              *bool  `json:"debug"`
	LogLevel           string `json:"logLevel"`
	GrokPoolTimeoutSec *int   `json:"grokPoolTimeoutSec"`
}

type serviceFileConfig struct {
	APIURL  string `json:"apiURL"`
	APIKey  string `json:"apiKey"`
	Enabled *bool  `json:"enabled"`
}

type tinyFishFileConfig struct {
	Enabled   *bool                `json:"enabled"`
	Keys      []engine.TinyFishKey `json:"keys"`
	SearchURL string               `json:"searchURL"`
	FetchURL  string               `json:"fetchURL"`
}

// Load reads environment variables and returns a validated Config.
//
// At least one Grok endpoint must resolve to a non-empty {baseURL, apiKey}.
func Load() (*Config, error) {
	fileCfg, err := loadDefaultAppConfigFile()
	if err != nil {
		return nil, err
	}

	endpoints, err := loadGrokEndpoints(fileCfg)
	if err != nil {
		return nil, err
	}
	if len(endpoints) == 0 {
		return nil, fmt.Errorf("no Grok endpoints configured: set GROK_ENDPOINTS_JSON, GROK_ENDPOINTS_FILE, GROK_API_URL + GROK_API_KEY, or place a JSON config at %s / %s", defaultAppConfigPath(), defaultConfigPath())
	}

	cfg := &Config{
		GrokEndpoints:   endpoints,
		TavilyAPIURL:    envOrFile("TAVILY_API_URL", fileCfg.tavilyAPIURL(), "https://api.tavily.com"),
		TavilyAPIKey:    envOrFile("TAVILY_API_KEY", fileCfg.tavilyAPIKey(), ""),
		TavilyEnabled:   boolEnvOrFile("TAVILY_ENABLED", fileCfg.tavilyEnabled(), true),
		ExaAPIURL:       envOrFile("EXA_API_URL", fileCfg.exaAPIURL(), "https://api.exa.ai"),
		ExaAPIKey:       envOrFile("EXA_API_KEY", fileCfg.exaAPIKey(), ""),
		ExaEnabled:      boolEnvOrFile("EXA_ENABLED", fileCfg.exaEnabled(), true),
		JinaAPIURL:      envOrFile("JINA_API_URL", fileCfg.jinaAPIURL(), "https://r.jina.ai"),
		JinaAPIKey:      envOrFile("JINA_API_KEY", fileCfg.jinaAPIKey(), ""),
		TinyFishEnabled: boolEnvOrFile("TINYFISH_ENABLED", fileCfg.tinyFishEnabled(), true),
		TinyFishKeys:    loadTinyFishKeys(fileCfg),
		TinyFishSearchURL: envOrFile(
			"TINYFISH_SEARCH_URL",
			fileCfg.tinyFishSearchURL(),
			engine.DefaultTinyFishSearchURL,
		),
		TinyFishFetchURL: envOrFile(
			"TINYFISH_FETCH_URL",
			fileCfg.tinyFishFetchURL(),
			engine.DefaultTinyFishFetchURL,
		),
		Debug:           boolEnvOrFile("GROK_DEBUG", fileCfg.debug(), false),
		LogLevel:        envOrFile("GROK_LOG_LEVEL", fileCfg.logLevel(), "INFO"),
		GrokPoolTimeout: parsePoolTimeout(fileCfg),
	}
	return cfg, nil
}

func loadGrokEndpoints(fileCfg *fileConfig) ([]engine.GrokEndpoint, error) {
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
		if eps := fileCfg.endpoints(); len(eps) > 0 {
			normalized, err := normalizeEndpoints(eps)
			if err != nil {
				return nil, fmt.Errorf("parse %s grokEndpoints: %w", defaultAppConfigPath(), err)
			}
			return normalized, nil
		}
		// Final fallback: legacy endpoints-only config file at the user's XDG config dir.
		return loadDefaultConfigFile()
	}
	return []engine.GrokEndpoint{{
		Name:           envOr("GROK_NAME", "default"),
		BaseURL:        url,
		APIKey:         key,
		Model:          envOr("GROK_MODEL", "grok-3-mini"),
		SendSearchFlag: strings.ToLower(envOr("GROK_SEND_SEARCH_FLAG", "true")) == "true",
	}}, nil
}

func loadDefaultAppConfigFile() (*fileConfig, error) {
	path := defaultAppConfigPath()
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
	var cfg fileConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &cfg, nil
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

func defaultAppConfigPath() string {
	dir := defaultConfigDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "config.json")
}

// defaultConfigPath returns the platform-default location of the optional
// endpoints config file: $XDG_CONFIG_HOME/grok-search/endpoints.json, falling
// back to $HOME/.config/grok-search/endpoints.json. Returns "" if no home
// directory can be resolved.
func defaultConfigPath() string {
	dir := defaultConfigDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "endpoints.json")
}

func defaultConfigDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "grok-search")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".config", "grok-search")
}

func parseEndpoints(data []byte) ([]engine.GrokEndpoint, error) {
	var eps []engine.GrokEndpoint
	if err := json.Unmarshal(data, &eps); err != nil {
		return nil, err
	}
	return normalizeEndpoints(eps)
}

func normalizeEndpoints(eps []engine.GrokEndpoint) ([]engine.GrokEndpoint, error) {
	for i, ep := range eps {
		if ep.BaseURL == "" || ep.APIKey == "" {
			return nil, fmt.Errorf("endpoint #%d (name=%q) missing baseURL or apiKey", i, ep.Name)
		}
		eps[i].BaseURL = normalizeOpenAIBaseURL(ep.BaseURL)
		if ep.Name == "" {
			eps[i].Name = fmt.Sprintf("endpoint-%d", i)
		}
		if ep.Model == "" {
			eps[i].Model = "grok-3-mini"
		}
		switch ep.APIType {
		case "", "chat", "responses":
			// valid
		default:
			return nil, fmt.Errorf("endpoint #%d (name=%q) has invalid apiType %q: must be \"\" (or \"chat\") or \"responses\"", i, ep.Name, ep.APIType)
		}
	}
	return eps, nil
}

func normalizeOpenAIBaseURL(baseURL string) string {
	baseURL = strings.TrimRight(baseURL, "/")
	if !strings.HasSuffix(baseURL, "/v1") {
		baseURL += "/v1"
	}
	return baseURL
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envOrFile(key, fileValue, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	if fileValue != "" {
		return fileValue
	}
	return fallback
}

func boolEnvOrFile(key string, fileValue *bool, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		return strings.ToLower(v) == "true"
	}
	if fileValue != nil {
		return *fileValue
	}
	return fallback
}

func loadTinyFishKeys(fileCfg *fileConfig) []engine.TinyFishKey {
	raw := os.Getenv("TINYFISH_API_KEYS")
	if raw == "" {
		raw = os.Getenv("TINYFISH_API_KEY")
	}
	if raw != "" {
		keys := splitCSV(raw)
		names := splitCSV(os.Getenv("TINYFISH_KEY_NAMES"))
		out := make([]engine.TinyFishKey, 0, len(keys))
		for i, key := range keys {
			item := engine.TinyFishKey{APIKey: key}
			if i < len(names) {
				item.Name = names[i]
			}
			if item.Name == "" {
				item.Name = fmt.Sprintf("env-%d", i)
			}
			out = append(out, item)
		}
		return normalizeTinyFishKeys(out)
	}
	if fileCfg == nil {
		return nil
	}
	return normalizeTinyFishKeys(fileCfg.TinyFish.Keys)
}

func normalizeTinyFishKeys(keys []engine.TinyFishKey) []engine.TinyFishKey {
	out := make([]engine.TinyFishKey, 0, len(keys))
	for i, key := range keys {
		key.APIKey = strings.TrimSpace(key.APIKey)
		key.Name = strings.TrimSpace(key.Name)
		if key.APIKey == "" {
			continue
		}
		if key.Name == "" {
			key.Name = fmt.Sprintf("key-%d", i)
		}
		out = append(out, key)
	}
	return out
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

// parsePoolTimeout parses GROK_POOL_TIMEOUT_SEC into a time.Duration. An empty,
// zero, or non-numeric value disables the cap (returns 0).
func parsePoolTimeout(fileCfg *fileConfig) time.Duration {
	v := os.Getenv("GROK_POOL_TIMEOUT_SEC")
	if v != "" {
		return secondsToDuration(v)
	}
	if fileCfg != nil && fileCfg.GrokPoolTimeoutSec != nil && *fileCfg.GrokPoolTimeoutSec > 0 {
		return time.Duration(*fileCfg.GrokPoolTimeoutSec) * time.Second
	}
	return 0
}

func secondsToDuration(v string) time.Duration {
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 0
	}
	return time.Duration(n) * time.Second
}

func (c *fileConfig) endpoints() []engine.GrokEndpoint {
	if c == nil {
		return nil
	}
	if len(c.GrokEndpoints) > 0 {
		return c.GrokEndpoints
	}
	return c.Endpoints
}

func (c *fileConfig) tavilyAPIURL() string {
	if c == nil {
		return ""
	}
	return c.Tavily.APIURL
}

func (c *fileConfig) tavilyAPIKey() string {
	if c == nil {
		return ""
	}
	return c.Tavily.APIKey
}

func (c *fileConfig) tavilyEnabled() *bool {
	if c == nil {
		return nil
	}
	return c.Tavily.Enabled
}

func (c *fileConfig) exaAPIURL() string {
	if c == nil {
		return ""
	}
	return c.Exa.APIURL
}

func (c *fileConfig) exaAPIKey() string {
	if c == nil {
		return ""
	}
	return c.Exa.APIKey
}

func (c *fileConfig) exaEnabled() *bool {
	if c == nil {
		return nil
	}
	return c.Exa.Enabled
}

func (c *fileConfig) jinaAPIURL() string {
	if c == nil {
		return ""
	}
	return c.Jina.APIURL
}

func (c *fileConfig) jinaAPIKey() string {
	if c == nil {
		return ""
	}
	return c.Jina.APIKey
}

func (c *fileConfig) tinyFishEnabled() *bool {
	if c == nil {
		return nil
	}
	return c.TinyFish.Enabled
}

func (c *fileConfig) tinyFishSearchURL() string {
	if c == nil {
		return ""
	}
	return c.TinyFish.SearchURL
}

func (c *fileConfig) tinyFishFetchURL() string {
	if c == nil {
		return ""
	}
	return c.TinyFish.FetchURL
}

func (c *fileConfig) debug() *bool {
	if c == nil {
		return nil
	}
	return c.Debug
}

func (c *fileConfig) logLevel() string {
	if c == nil {
		return ""
	}
	return c.LogLevel
}
