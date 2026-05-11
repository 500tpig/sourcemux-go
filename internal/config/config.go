package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bettas/grok-search-go/internal/engine"
)

const DefaultConfigFilename = "grok-search.json"

// Config holds all runtime configuration, resolved from one explicit JSON file.
type Config struct {
	GrokEndpoints      []engine.GrokEndpoint
	ReasoningEndpoints []engine.ReasoningEndpoint

	TavilyAPIURL  string
	TavilyAPIKey  string
	TavilyEnabled bool

	ExaAPIURL  string
	ExaAPIKey  string
	ExaEnabled bool

	JinaAPIURL string
	JinaAPIKey string

	TinyFishEnabled   bool
	TinyFishKeys      []engine.TinyFishKey
	TinyFishSearchURL string
	TinyFishFetchURL  string

	Debug    bool
	LogLevel string

	GrokPoolTimeout time.Duration
}

type fileConfig struct {
	GrokEndpoints      []engine.GrokEndpoint      `json:"grokEndpoints"`
	ReasoningEndpoints []engine.ReasoningEndpoint `json:"reasoningEndpoints"`

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

// Load reads the default single-file config from ./grok-search.json.
func Load() (*Config, error) {
	return LoadFile(DefaultConfigPath())
}

// LoadFile reads one explicit JSON config file. No environment variables,
// hidden user config directories, or legacy fallback files are consulted.
func LoadFile(path string) (*Config, error) {
	if strings.TrimSpace(path) == "" {
		path = DefaultConfigPath()
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("config file not found: %s", path)
		}
		return nil, fmt.Errorf("read config file %s: %w", path, err)
	}
	var fileCfg fileConfig
	if err := json.Unmarshal(data, &fileCfg); err != nil {
		return nil, fmt.Errorf("parse config file %s: %w", path, err)
	}
	return buildConfig(fileCfg, path)
}

func buildConfig(fileCfg fileConfig, path string) (*Config, error) {
	endpoints, err := normalizeEndpoints(fileCfg.GrokEndpoints)
	if err != nil {
		return nil, fmt.Errorf("parse %s grokEndpoints: %w", path, err)
	}
	reasoningEndpoints, err := normalizeReasoningEndpoints(fileCfg.ReasoningEndpoints)
	if err != nil {
		return nil, fmt.Errorf("parse %s reasoningEndpoints: %w", path, err)
	}

	return &Config{
		GrokEndpoints:      endpoints,
		ReasoningEndpoints: reasoningEndpoints,

		TavilyAPIURL:  stringOr(fileCfg.Tavily.APIURL, "https://api.tavily.com"),
		TavilyAPIKey:  strings.TrimSpace(fileCfg.Tavily.APIKey),
		TavilyEnabled: boolPtrOr(fileCfg.Tavily.Enabled, true),

		ExaAPIURL:  stringOr(fileCfg.Exa.APIURL, "https://api.exa.ai"),
		ExaAPIKey:  strings.TrimSpace(fileCfg.Exa.APIKey),
		ExaEnabled: boolPtrOr(fileCfg.Exa.Enabled, true),

		JinaAPIURL: stringOr(fileCfg.Jina.APIURL, "https://r.jina.ai"),
		JinaAPIKey: strings.TrimSpace(fileCfg.Jina.APIKey),

		TinyFishEnabled:   boolPtrOr(fileCfg.TinyFish.Enabled, true),
		TinyFishKeys:      normalizeTinyFishKeys(fileCfg.TinyFish.Keys),
		TinyFishSearchURL: stringOr(fileCfg.TinyFish.SearchURL, engine.DefaultTinyFishSearchURL),
		TinyFishFetchURL:  stringOr(fileCfg.TinyFish.FetchURL, engine.DefaultTinyFishFetchURL),

		Debug:           boolPtrOr(fileCfg.Debug, false),
		LogLevel:        stringOr(fileCfg.LogLevel, "INFO"),
		GrokPoolTimeout: secondsPtrToDuration(fileCfg.GrokPoolTimeoutSec),
	}, nil
}

func DefaultConfigPath() string {
	return DefaultConfigFilename
}

func DefaultConfigAbsPath() string {
	abs, err := filepath.Abs(DefaultConfigPath())
	if err != nil {
		return DefaultConfigPath()
	}
	return abs
}

func normalizeEndpoints(eps []engine.GrokEndpoint) ([]engine.GrokEndpoint, error) {
	out := make([]engine.GrokEndpoint, 0, len(eps))
	for i, ep := range eps {
		ep.Name = strings.TrimSpace(ep.Name)
		ep.BaseURL = strings.TrimSpace(ep.BaseURL)
		ep.APIKey = strings.TrimSpace(ep.APIKey)
		ep.Model = strings.TrimSpace(ep.Model)
		ep.APIType = strings.TrimSpace(ep.APIType)
		responseTools, err := engine.NormalizeResponseTools(ep.ResponseTools)
		if err != nil {
			return nil, fmt.Errorf("endpoint #%d (name=%q): %w", i, ep.Name, err)
		}
		ep.ResponseTools = responseTools
		if len(ep.ResponseTools) > 0 && ep.APIType != "responses" {
			return nil, fmt.Errorf("endpoint #%d (name=%q) responseTools require apiType \"responses\"", i, ep.Name)
		}
		if ep.BaseURL == "" || ep.APIKey == "" {
			return nil, fmt.Errorf("endpoint #%d (name=%q) missing baseURL or apiKey", i, ep.Name)
		}
		ep.BaseURL = normalizeOpenAIBaseURL(ep.BaseURL)
		if ep.Name == "" {
			ep.Name = fmt.Sprintf("endpoint-%d", i)
		}
		if ep.Model == "" {
			ep.Model = "grok-3-mini"
		}
		switch ep.APIType {
		case "", "chat", "responses":
		default:
			return nil, fmt.Errorf("endpoint #%d (name=%q) has invalid apiType %q: must be \"\" (or \"chat\") or \"responses\"", i, ep.Name, ep.APIType)
		}
		out = append(out, ep)
	}
	return out, nil
}

func normalizeReasoningEndpoints(eps []engine.ReasoningEndpoint) ([]engine.ReasoningEndpoint, error) {
	out := make([]engine.ReasoningEndpoint, 0, len(eps))
	for i, ep := range eps {
		ep.Name = strings.TrimSpace(ep.Name)
		ep.BaseURL = strings.TrimSpace(ep.BaseURL)
		ep.APIKey = strings.TrimSpace(ep.APIKey)
		ep.Model = strings.TrimSpace(ep.Model)
		if ep.BaseURL == "" || ep.APIKey == "" {
			return nil, fmt.Errorf("endpoint #%d (name=%q) missing baseURL or apiKey", i, ep.Name)
		}
		ep.BaseURL = normalizeOpenAIBaseURL(ep.BaseURL)
		if ep.Name == "" {
			ep.Name = fmt.Sprintf("reasoning-%d", i)
		}
		if ep.Model == "" {
			ep.Model = "deepseek-v4-flash"
		}
		out = append(out, ep)
	}
	return out, nil
}

func normalizeOpenAIBaseURL(baseURL string) string {
	baseURL = strings.TrimRight(baseURL, "/")
	if !strings.HasSuffix(baseURL, "/v1") {
		baseURL += "/v1"
	}
	return baseURL
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

func stringOr(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func boolPtrOr(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

func secondsPtrToDuration(value *int) time.Duration {
	if value == nil || *value <= 0 {
		return 0
	}
	return time.Duration(*value) * time.Second
}
