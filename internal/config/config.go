package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/500tpig/grok-search-go/internal/engine"
)

const DefaultConfigFilename = "grok-search.json"

// Config holds all runtime configuration, resolved from one explicit JSON file.
type Config struct {
	Version        int
	MinimumProfile string

	MainSearchConfigured bool
	DocsSearchConfigured bool
	WebFetchConfigured   bool

	GrokEndpoints      []engine.GrokEndpoint
	ReasoningEndpoints []engine.ReasoningEndpoint

	TavilyAPIURL  string
	TavilyAPIKey  string
	TavilyEnabled bool

	ExaAPIURL  string
	ExaAPIKey  string
	ExaEnabled bool

	Context7Endpoints []engine.Context7Endpoint

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
	Version        *int                    `json:"version"`
	MinimumProfile string                  `json:"minimum_profile"`
	Capabilities   *capabilitiesFileConfig `json:"capabilities"`

	GrokEndpoints        []engine.GrokEndpoint      `json:"grokEndpoints"`
	ReasoningEndpoints   []engine.ReasoningEndpoint `json:"reasoningEndpoints"`
	ReasoningEndpointsV2 []engine.ReasoningEndpoint `json:"reasoning_endpoints"`

	Tavily   serviceFileConfig  `json:"tavily"`
	Exa      serviceFileConfig  `json:"exa"`
	Context7 context7FileConfig `json:"context7"`
	Jina     serviceFileConfig  `json:"jina"`

	TinyFish tinyFishFileConfig `json:"tinyfish"`

	Debug              *bool  `json:"debug"`
	LogLevel           string `json:"logLevel"`
	GrokPoolTimeoutSec *int   `json:"grokPoolTimeoutSec"`
}

type capabilitiesFileConfig struct {
	MainSearch capabilityFileConfig `json:"main_search"`
	DocsSearch capabilityFileConfig `json:"docs_search"`
	WebFetch   capabilityFileConfig `json:"web_fetch"`
	WebEnhance capabilityFileConfig `json:"web_enhance"`
}

type capabilityFileConfig struct {
	Providers []providerFileConfig `json:"providers"`
}

type providerFileConfig struct {
	Type string `json:"type"`
	Name string `json:"name"`

	APIURL  string `json:"apiURL"`
	APIKey  string `json:"apiKey"`
	Enabled *bool  `json:"enabled"`

	BaseURL        string                `json:"baseURL"`
	Model          string                `json:"model"`
	APIType        string                `json:"apiType"`
	SendSearchFlag bool                  `json:"sendSearchFlag"`
	ResponseTools  []string              `json:"responseTools,omitempty"`
	Endpoints      []engine.GrokEndpoint `json:"endpoints,omitempty"`

	Keys      []engine.TinyFishKey `json:"keys,omitempty"`
	SearchURL string               `json:"searchURL"`
	FetchURL  string               `json:"fetchURL"`

	Priority        int      `json:"priority,omitempty"`
	LibraryScopes   []string `json:"library_scopes,omitempty"`
	MonthlyBudget   int      `json:"monthly_budget,omitempty"`
	CooldownSeconds int      `json:"cooldown_on_rate_limit_seconds,omitempty"`
}

type serviceFileConfig struct {
	APIURL  string `json:"apiURL"`
	APIKey  string `json:"apiKey"`
	Enabled *bool  `json:"enabled"`
}

type context7FileConfig struct {
	APIURL    string                    `json:"apiURL"`
	APIKey    string                    `json:"apiKey"`
	Enabled   *bool                     `json:"enabled"`
	Providers []engine.Context7Endpoint `json:"providers,omitempty"`
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
	var raw map[string]json.RawMessage
	_ = json.Unmarshal(data, &raw)
	return buildConfig(fileCfg, raw, path)
}

func buildConfig(fileCfg fileConfig, raw map[string]json.RawMessage, path string) (*Config, error) {
	if fileCfg.Capabilities != nil {
		return buildConfigV2(fileCfg, raw, path)
	}
	return buildConfigV1(fileCfg, path)
}

func buildConfigV1(fileCfg fileConfig, path string) (*Config, error) {
	endpoints, err := normalizeEndpoints(fileCfg.GrokEndpoints)
	if err != nil {
		return nil, fmt.Errorf("parse %s grokEndpoints: %w", path, err)
	}
	reasoningEndpoints, err := normalizeReasoningEndpoints(fileCfg.ReasoningEndpoints)
	if err != nil {
		return nil, fmt.Errorf("parse %s reasoningEndpoints: %w", path, err)
	}

	return &Config{
		Version:              1,
		MinimumProfile:       "off",
		MainSearchConfigured: len(endpoints) > 0,
		DocsSearchConfigured: boolPtrOr(fileCfg.Exa.Enabled, true) && strings.TrimSpace(fileCfg.Exa.APIKey) != "",
		WebFetchConfigured:   stringOr(fileCfg.Jina.APIURL, "https://r.jina.ai") != "",
		GrokEndpoints:        endpoints,
		ReasoningEndpoints:   reasoningEndpoints,

		TavilyAPIURL:  stringOr(fileCfg.Tavily.APIURL, "https://api.tavily.com"),
		TavilyAPIKey:  strings.TrimSpace(fileCfg.Tavily.APIKey),
		TavilyEnabled: boolPtrOr(fileCfg.Tavily.Enabled, true),

		ExaAPIURL:  stringOr(fileCfg.Exa.APIURL, "https://api.exa.ai"),
		ExaAPIKey:  strings.TrimSpace(fileCfg.Exa.APIKey),
		ExaEnabled: boolPtrOr(fileCfg.Exa.Enabled, true),

		Context7Endpoints: normalizeContext7Endpoints(fileCfg.Context7),

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

func buildConfigV2(fileCfg fileConfig, raw map[string]json.RawMessage, path string) (*Config, error) {
	if err := rejectMixedV1V2(raw); err != nil {
		return nil, err
	}
	if fileCfg.Version != nil && *fileCfg.Version != 2 {
		return nil, fmt.Errorf("config version %d is not supported with capabilities; expected version 2", *fileCfg.Version)
	}

	out := &Config{
		Version:           2,
		MinimumProfile:    normalizeMinimumProfile(fileCfg.MinimumProfile),
		TavilyAPIURL:      "https://api.tavily.com",
		TavilyEnabled:     true,
		ExaAPIURL:         "https://api.exa.ai",
		ExaEnabled:        true,
		JinaAPIURL:        "https://r.jina.ai",
		TinyFishEnabled:   true,
		TinyFishSearchURL: engine.DefaultTinyFishSearchURL,
		TinyFishFetchURL:  engine.DefaultTinyFishFetchURL,
		Debug:             boolPtrOr(fileCfg.Debug, false),
		LogLevel:          stringOr(fileCfg.LogLevel, "INFO"),
		GrokPoolTimeout:   secondsPtrToDuration(fileCfg.GrokPoolTimeoutSec),
	}

	reasoning, err := normalizeReasoningEndpoints(fileCfg.ReasoningEndpointsV2)
	if err != nil {
		return nil, fmt.Errorf("parse %s reasoning_endpoints: %w", path, err)
	}
	out.ReasoningEndpoints = reasoning

	for _, capCfg := range []capabilityFileConfig{
		fileCfg.Capabilities.MainSearch,
		fileCfg.Capabilities.DocsSearch,
		fileCfg.Capabilities.WebFetch,
		fileCfg.Capabilities.WebEnhance,
	} {
		if err := applyV2Providers(out, capCfg.Providers, path); err != nil {
			return nil, err
		}
	}
	endpoints, err := normalizeEndpoints(out.GrokEndpoints)
	if err != nil {
		return nil, fmt.Errorf("parse %s capabilities grok providers: %w", path, err)
	}
	out.GrokEndpoints = endpoints
	out.TinyFishKeys = normalizeTinyFishKeys(out.TinyFishKeys)
	out.Context7Endpoints = engine.SortContext7Endpoints(out.Context7Endpoints)
	out.MainSearchConfigured = v2HasMainSearch(fileCfg.Capabilities.MainSearch.Providers)
	out.DocsSearchConfigured = v2HasDocsSearch(fileCfg.Capabilities.DocsSearch.Providers)
	out.WebFetchConfigured = v2HasWebFetch(fileCfg.Capabilities.WebFetch.Providers)
	return out, nil
}

func rejectMixedV1V2(raw map[string]json.RawMessage) error {
	for _, key := range []string{"grokEndpoints", "reasoningEndpoints", "tavily", "exa", "context7", "jina", "tinyfish"} {
		if _, ok := raw[key]; ok {
			return fmt.Errorf("config mixes v2 capabilities with legacy %q; use either v1 fields or v2 capabilities, not both", key)
		}
	}
	return nil
}

func applyV2Providers(out *Config, providers []providerFileConfig, path string) error {
	for i, p := range providers {
		switch strings.TrimSpace(p.Type) {
		case "", "disabled":
			continue
		case "grok-pool":
			out.GrokEndpoints = append(out.GrokEndpoints, p.Endpoints...)
			if len(p.Endpoints) == 0 && strings.TrimSpace(p.BaseURL) != "" && strings.TrimSpace(p.APIKey) != "" {
				out.GrokEndpoints = append(out.GrokEndpoints, grokEndpointFromProvider(p))
			}
		case "openai-compatible":
			out.GrokEndpoints = append(out.GrokEndpoints, grokEndpointFromProvider(p))
		case "exa":
			out.ExaAPIURL = stringOr(p.APIURL, out.ExaAPIURL)
			out.ExaAPIKey = strings.TrimSpace(p.APIKey)
			out.ExaEnabled = boolPtrOr(p.Enabled, true)
		case "context7":
			if providerEnabled(p) && strings.TrimSpace(p.APIKey) != "" {
				out.Context7Endpoints = append(out.Context7Endpoints, engine.Context7Endpoint{
					Name:            strings.TrimSpace(p.Name),
					APIURL:          stringOr(p.APIURL, engine.DefaultContext7APIURL),
					APIKey:          strings.TrimSpace(p.APIKey),
					Priority:        p.Priority,
					LibraryScopes:   append([]string(nil), p.LibraryScopes...),
					MonthlyBudget:   p.MonthlyBudget,
					CooldownSeconds: p.CooldownSeconds,
				})
			}
		case "jina":
			out.JinaAPIURL = stringOr(p.APIURL, out.JinaAPIURL)
			out.JinaAPIKey = strings.TrimSpace(p.APIKey)
		case "tinyfish":
			out.TinyFishEnabled = boolPtrOr(p.Enabled, true)
			out.TinyFishKeys = append(out.TinyFishKeys, p.Keys...)
			out.TinyFishSearchURL = stringOr(p.SearchURL, out.TinyFishSearchURL)
			out.TinyFishFetchURL = stringOr(p.FetchURL, out.TinyFishFetchURL)
		case "tavily":
			out.TavilyAPIURL = stringOr(p.APIURL, out.TavilyAPIURL)
			out.TavilyAPIKey = strings.TrimSpace(p.APIKey)
			out.TavilyEnabled = boolPtrOr(p.Enabled, true)
		default:
			return fmt.Errorf("parse %s capabilities provider #%d: unsupported type %q", path, i, p.Type)
		}
	}
	return nil
}

func grokEndpointFromProvider(p providerFileConfig) engine.GrokEndpoint {
	return engine.GrokEndpoint{
		Name:           strings.TrimSpace(p.Name),
		BaseURL:        strings.TrimSpace(p.BaseURL),
		APIKey:         strings.TrimSpace(p.APIKey),
		Model:          strings.TrimSpace(p.Model),
		APIType:        strings.TrimSpace(p.APIType),
		SendSearchFlag: p.SendSearchFlag,
		ResponseTools:  append([]string(nil), p.ResponseTools...),
	}
}

func v2HasMainSearch(providers []providerFileConfig) bool {
	for _, p := range providers {
		if !providerEnabled(p) {
			continue
		}
		switch strings.TrimSpace(p.Type) {
		case "grok-pool":
			if len(p.Endpoints) > 0 || (strings.TrimSpace(p.BaseURL) != "" && strings.TrimSpace(p.APIKey) != "") {
				return true
			}
		case "openai-compatible":
			if strings.TrimSpace(p.BaseURL) != "" && strings.TrimSpace(p.APIKey) != "" {
				return true
			}
		}
	}
	return false
}

func v2HasDocsSearch(providers []providerFileConfig) bool {
	for _, p := range providers {
		if providerEnabled(p) && strings.TrimSpace(p.Type) == "exa" && strings.TrimSpace(p.APIKey) != "" {
			return true
		}
	}
	return false
}

func normalizeContext7Endpoints(cfg context7FileConfig) []engine.Context7Endpoint {
	if !boolPtrOr(cfg.Enabled, true) {
		return nil
	}
	var out []engine.Context7Endpoint
	for _, ep := range cfg.Providers {
		ep.Name = strings.TrimSpace(ep.Name)
		ep.APIURL = stringOr(ep.APIURL, engine.DefaultContext7APIURL)
		ep.APIKey = strings.TrimSpace(ep.APIKey)
		if ep.APIKey == "" {
			continue
		}
		if ep.Name == "" {
			ep.Name = fmt.Sprintf("context7-%d", len(out))
		}
		out = append(out, ep)
	}
	if strings.TrimSpace(cfg.APIKey) != "" {
		out = append(out, engine.Context7Endpoint{
			Name:   "context7-main",
			APIURL: stringOr(cfg.APIURL, engine.DefaultContext7APIURL),
			APIKey: strings.TrimSpace(cfg.APIKey),
		})
	}
	return engine.SortContext7Endpoints(out)
}

func v2HasWebFetch(providers []providerFileConfig) bool {
	for _, p := range providers {
		if !providerEnabled(p) {
			continue
		}
		switch strings.TrimSpace(p.Type) {
		case "jina":
			return true
		case "tinyfish":
			return len(p.Keys) > 0
		case "exa", "tavily":
			return strings.TrimSpace(p.APIKey) != ""
		}
	}
	return false
}

func providerEnabled(p providerFileConfig) bool {
	t := strings.TrimSpace(p.Type)
	if t == "" || t == "disabled" {
		return false
	}
	return boolPtrOr(p.Enabled, true)
}

func normalizeMinimumProfile(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "standard":
		return "standard"
	case "off":
		return "off"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
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
