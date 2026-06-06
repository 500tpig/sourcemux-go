package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/500tpig/sourcemux-go/internal/engine"
)

const DefaultConfigFilename = "sourcemux.json"

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

	FirecrawlAPIURL     string
	FirecrawlAPIKey     string
	FirecrawlKeys       []engine.FirecrawlKey
	FirecrawlEnabled    bool
	WebFetchOrder       []string
	WebFetchStrictOrder bool

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

	GrokPoolTimeout    time.Duration
	GrokPoolTimeoutSet bool
	SearchPolicy       SearchPolicy
}

const (
	SearchProfileDefault = engine.DefaultGrokEndpointProfile
	SearchProfileAuto    = "auto"
	SearchProfileHeavy   = engine.HeavyGrokEndpointProfile

	SearchAutoPreferenceIntentBased  = "intent-based"
	SearchAutoPreferenceHeavyFirst   = "heavy-first"
	SearchAutoPreferenceDefaultFirst = "default-first"

	DefaultSearchFallbackAfterSec = 180
	DefaultSearchTimeoutSec       = 300
)

type SearchPolicy struct {
	DefaultProfile   string        `json:"defaultProfile"`
	AgentProfile     string        `json:"agentProfile"`
	AutoPreference   string        `json:"autoPreference"`
	FallbackAfterSec int           `json:"fallbackAfterSec"`
	TimeoutSec       int           `json:"timeoutSec"`
	FallbackAfter    time.Duration `json:"-"`
	Timeout          time.Duration `json:"-"`
}

type fileConfig struct {
	Version        *int                    `json:"version"`
	MinimumProfile string                  `json:"minimum_profile"`
	Capabilities   *capabilitiesFileConfig `json:"capabilities"`

	GrokEndpoints        []engine.GrokEndpoint      `json:"grokEndpoints"`
	ReasoningEndpoints   []engine.ReasoningEndpoint `json:"reasoningEndpoints"`
	ReasoningEndpointsV2 []engine.ReasoningEndpoint `json:"reasoning_endpoints"`

	Tavily    serviceFileConfig `json:"tavily"`
	Firecrawl serviceFileConfig `json:"firecrawl"`
	Exa       serviceFileConfig `json:"exa"`
	Jina      serviceFileConfig `json:"jina"`

	TinyFish tinyFishFileConfig `json:"tinyfish"`

	Debug              *bool                   `json:"debug"`
	LogLevel           string                  `json:"logLevel"`
	GrokPoolTimeoutSec *int                    `json:"grokPoolTimeoutSec"`
	SearchPolicy       *searchPolicyFileConfig `json:"searchPolicy"`
}

type searchPolicyFileConfig struct {
	DefaultProfile   string `json:"defaultProfile"`
	AgentProfile     string `json:"agentProfile"`
	AutoPreference   string `json:"autoPreference"`
	FallbackAfterSec *int   `json:"fallbackAfterSec"`
	TimeoutSec       *int   `json:"timeoutSec"`
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
	Profile        string                `json:"profile,omitempty"`
	SendSearchFlag bool                  `json:"sendSearchFlag"`
	ResponseTools  []string              `json:"responseTools,omitempty"`
	Endpoints      []engine.GrokEndpoint `json:"endpoints,omitempty"`

	Keys      []engine.TinyFishKey `json:"keys,omitempty"`
	SearchURL string               `json:"searchURL"`
	FetchURL  string               `json:"fetchURL"`
}

type serviceFileConfig struct {
	APIURL  string                `json:"apiURL"`
	APIKey  string                `json:"apiKey"`
	Keys    []engine.FirecrawlKey `json:"keys"`
	Enabled *bool                 `json:"enabled"`
}

type tinyFishFileConfig struct {
	Enabled   *bool                `json:"enabled"`
	Keys      []engine.TinyFishKey `json:"keys"`
	SearchURL string               `json:"searchURL"`
	FetchURL  string               `json:"fetchURL"`
}

// Load reads the default single-file config from ./sourcemux.json.
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
	searchPolicy, err := normalizeSearchPolicy(fileCfg.SearchPolicy)
	if err != nil {
		return nil, fmt.Errorf("parse %s searchPolicy: %w", path, err)
	}

	return &Config{
		Version:              1,
		MinimumProfile:       "off",
		MainSearchConfigured: len(engine.FilterGrokEndpoints(endpoints, "")) > 0,
		DocsSearchConfigured: boolPtrOr(fileCfg.Exa.Enabled, true) && strings.TrimSpace(fileCfg.Exa.APIKey) != "",
		WebFetchConfigured:   stringOr(fileCfg.Jina.APIURL, "https://r.jina.ai") != "",
		GrokEndpoints:        endpoints,
		ReasoningEndpoints:   reasoningEndpoints,

		TavilyAPIURL:  stringOr(fileCfg.Tavily.APIURL, "https://api.tavily.com"),
		TavilyAPIKey:  strings.TrimSpace(fileCfg.Tavily.APIKey),
		TavilyEnabled: boolPtrOr(fileCfg.Tavily.Enabled, true),

		FirecrawlAPIURL:     stringOr(fileCfg.Firecrawl.APIURL, engine.DefaultFirecrawlAPIURL),
		FirecrawlAPIKey:     strings.TrimSpace(fileCfg.Firecrawl.APIKey),
		FirecrawlKeys:       normalizeFirecrawlKeys(fileCfg.Firecrawl.APIKey, fileCfg.Firecrawl.Keys),
		FirecrawlEnabled:    boolPtrOr(fileCfg.Firecrawl.Enabled, false),
		WebFetchOrder:       defaultWebFetchOrder(),
		WebFetchStrictOrder: false,

		ExaAPIURL:  stringOr(fileCfg.Exa.APIURL, "https://api.exa.ai"),
		ExaAPIKey:  strings.TrimSpace(fileCfg.Exa.APIKey),
		ExaEnabled: boolPtrOr(fileCfg.Exa.Enabled, true),

		JinaAPIURL: stringOr(fileCfg.Jina.APIURL, "https://r.jina.ai"),
		JinaAPIKey: strings.TrimSpace(fileCfg.Jina.APIKey),

		TinyFishEnabled:   boolPtrOr(fileCfg.TinyFish.Enabled, true),
		TinyFishKeys:      normalizeTinyFishKeys(fileCfg.TinyFish.Keys),
		TinyFishSearchURL: stringOr(fileCfg.TinyFish.SearchURL, engine.DefaultTinyFishSearchURL),
		TinyFishFetchURL:  stringOr(fileCfg.TinyFish.FetchURL, engine.DefaultTinyFishFetchURL),

		Debug:              boolPtrOr(fileCfg.Debug, false),
		LogLevel:           stringOr(fileCfg.LogLevel, "INFO"),
		GrokPoolTimeout:    secondsPtrToDuration(fileCfg.GrokPoolTimeoutSec),
		GrokPoolTimeoutSet: fileCfg.GrokPoolTimeoutSec != nil,
		SearchPolicy:       searchPolicy,
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
		Version:             2,
		MinimumProfile:      normalizeMinimumProfile(fileCfg.MinimumProfile),
		TavilyAPIURL:        "https://api.tavily.com",
		TavilyEnabled:       true,
		FirecrawlAPIURL:     engine.DefaultFirecrawlAPIURL,
		FirecrawlEnabled:    false,
		WebFetchOrder:       normalizeV2WebFetchOrder(fileCfg.Capabilities.WebFetch.Providers),
		WebFetchStrictOrder: true,
		ExaAPIURL:           "https://api.exa.ai",
		ExaEnabled:          true,
		JinaAPIURL:          "https://r.jina.ai",
		TinyFishEnabled:     true,
		TinyFishSearchURL:   engine.DefaultTinyFishSearchURL,
		TinyFishFetchURL:    engine.DefaultTinyFishFetchURL,
		Debug:               boolPtrOr(fileCfg.Debug, false),
		LogLevel:            stringOr(fileCfg.LogLevel, "INFO"),
		GrokPoolTimeout:     secondsPtrToDuration(fileCfg.GrokPoolTimeoutSec),
		GrokPoolTimeoutSet:  fileCfg.GrokPoolTimeoutSec != nil,
	}
	searchPolicy, err := normalizeSearchPolicy(fileCfg.SearchPolicy)
	if err != nil {
		return nil, fmt.Errorf("parse %s searchPolicy: %w", path, err)
	}
	out.SearchPolicy = searchPolicy

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
	out.FirecrawlKeys = normalizeFirecrawlKeys(out.FirecrawlAPIKey, out.FirecrawlKeys)
	out.MainSearchConfigured = v2HasMainSearch(fileCfg.Capabilities.MainSearch.Providers)
	out.DocsSearchConfigured = v2HasDocsSearch(fileCfg.Capabilities.DocsSearch.Providers)
	out.WebFetchConfigured = v2HasWebFetch(fileCfg.Capabilities.WebFetch.Providers)
	return out, nil
}

func rejectMixedV1V2(raw map[string]json.RawMessage) error {
	for _, key := range []string{"grokEndpoints", "reasoningEndpoints", "tavily", "firecrawl", "exa", "jina", "tinyfish"} {
		if _, ok := raw[key]; ok {
			return fmt.Errorf("config mixes v2 capabilities with legacy %q; use either v1 fields or v2 capabilities, not both", key)
		}
	}
	return nil
}

func applyV2Providers(out *Config, providers []providerFileConfig, path string) error {
	for i, p := range providers {
		providerType := strings.TrimSpace(p.Type)
		if providerType == "" || providerType == "disabled" {
			continue
		}
		if providerType == "firecrawl" {
			if !boolPtrOr(p.Enabled, false) {
				continue
			}
		} else if !providerEnabled(p) {
			continue
		}
		switch providerType {
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
		case "firecrawl":
			out.FirecrawlAPIURL = stringOr(p.APIURL, out.FirecrawlAPIURL)
			out.FirecrawlAPIKey = strings.TrimSpace(p.APIKey)
			out.FirecrawlKeys = append(out.FirecrawlKeys, p.Keys...)
			out.FirecrawlEnabled = boolPtrOr(p.Enabled, true)
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
		Profile:        strings.TrimSpace(p.Profile),
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
			ep := grokEndpointFromProvider(p)
			if len(engine.FilterGrokEndpoints(p.Endpoints, "")) > 0 || (strings.TrimSpace(ep.BaseURL) != "" && strings.TrimSpace(ep.APIKey) != "" && len(engine.FilterGrokEndpoints([]engine.GrokEndpoint{ep}, "")) > 0) {
				return true
			}
		case "openai-compatible":
			ep := grokEndpointFromProvider(p)
			if strings.TrimSpace(ep.BaseURL) != "" && strings.TrimSpace(ep.APIKey) != "" && len(engine.FilterGrokEndpoints([]engine.GrokEndpoint{ep}, "")) > 0 {
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
		case "firecrawl":
			return boolPtrOr(p.Enabled, false) && (strings.TrimSpace(p.APIKey) != "" || len(p.Keys) > 0)
		}
	}
	return false
}

func defaultWebFetchOrder() []string {
	return []string{"firecrawl", "jina", "exa", "tavily", "tinyfish"}
}

func normalizeV2WebFetchOrder(providers []providerFileConfig) []string {
	out := make([]string, 0, len(providers))
	for _, p := range providers {
		t := strings.TrimSpace(p.Type)
		if t == "" || t == "disabled" {
			continue
		}
		if t == "firecrawl" {
			if !boolPtrOr(p.Enabled, false) {
				continue
			}
		} else if !providerEnabled(p) {
			continue
		}
		out = append(out, t)
	}
	if len(out) == 0 {
		return nil
	}
	return out
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

func DefaultSearchPolicy() SearchPolicy {
	return SearchPolicy{
		DefaultProfile:   SearchProfileDefault,
		AgentProfile:     SearchProfileAuto,
		AutoPreference:   SearchAutoPreferenceIntentBased,
		FallbackAfterSec: DefaultSearchFallbackAfterSec,
		TimeoutSec:       DefaultSearchTimeoutSec,
		FallbackAfter:    time.Duration(DefaultSearchFallbackAfterSec) * time.Second,
		Timeout:          time.Duration(DefaultSearchTimeoutSec) * time.Second,
	}
}

func normalizeSearchPolicy(in *searchPolicyFileConfig) (SearchPolicy, error) {
	out := DefaultSearchPolicy()
	if in == nil {
		return out, nil
	}
	if strings.TrimSpace(in.DefaultProfile) != "" {
		profile, err := normalizeConfiguredSearchProfile(in.DefaultProfile, "defaultProfile")
		if err != nil {
			return SearchPolicy{}, err
		}
		out.DefaultProfile = profile
	}
	if strings.TrimSpace(in.AgentProfile) != "" {
		profile, err := normalizeConfiguredSearchProfile(in.AgentProfile, "agentProfile")
		if err != nil {
			return SearchPolicy{}, err
		}
		out.AgentProfile = profile
	}
	if strings.TrimSpace(in.AutoPreference) != "" {
		switch value := strings.ToLower(strings.TrimSpace(in.AutoPreference)); value {
		case SearchAutoPreferenceIntentBased, SearchAutoPreferenceHeavyFirst, SearchAutoPreferenceDefaultFirst:
			out.AutoPreference = value
		default:
			return SearchPolicy{}, fmt.Errorf("autoPreference %q is invalid: must be %q, %q, or %q", in.AutoPreference, SearchAutoPreferenceIntentBased, SearchAutoPreferenceHeavyFirst, SearchAutoPreferenceDefaultFirst)
		}
	}
	if in.FallbackAfterSec != nil {
		if *in.FallbackAfterSec < 0 {
			return SearchPolicy{}, fmt.Errorf("fallbackAfterSec must be non-negative")
		}
		out.FallbackAfterSec = *in.FallbackAfterSec
	}
	if in.TimeoutSec != nil {
		if *in.TimeoutSec <= 0 {
			return SearchPolicy{}, fmt.Errorf("timeoutSec must be positive")
		}
		out.TimeoutSec = *in.TimeoutSec
	}
	out.FallbackAfter = time.Duration(out.FallbackAfterSec) * time.Second
	out.Timeout = time.Duration(out.TimeoutSec) * time.Second
	return out, nil
}

func normalizeConfiguredSearchProfile(value, field string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case SearchProfileDefault, SearchProfileAuto, SearchProfileHeavy:
		return value, nil
	default:
		return "", fmt.Errorf("%s %q is invalid: must be %q, %q, or %q", field, value, SearchProfileDefault, SearchProfileAuto, SearchProfileHeavy)
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
		ep.Profile = ep.EffectiveProfile()
		responseTools, err := engine.NormalizeResponseTools(ep.ResponseTools)
		if err != nil {
			return nil, fmt.Errorf("endpoint #%d (name=%q): %w", i, ep.Name, err)
		}
		ep.ResponseTools = responseTools
		if len(ep.ResponseTools) > 0 && ep.APIType != "responses" {
			return nil, fmt.Errorf("endpoint #%d (name=%q) responseTools require apiType \"responses\"", i, ep.Name)
		}
		if ep.Name == "" {
			ep.Name = fmt.Sprintf("endpoint-%d", i)
		}
		if ep.Model == "" {
			ep.Model = "grok-3-mini"
		}
		if ep.BaseURL != "" {
			ep.BaseURL = normalizeOpenAIBaseURL(ep.BaseURL)
		}
		switch ep.APIType {
		case "", "chat", "responses":
		default:
			return nil, fmt.Errorf("endpoint #%d (name=%q) has invalid apiType %q: must be \"\" (or \"chat\") or \"responses\"", i, ep.Name, ep.APIType)
		}
		if !ep.IsEnabled() {
			out = append(out, ep)
			continue
		}
		if ep.BaseURL == "" || ep.APIKey == "" {
			return nil, fmt.Errorf("endpoint #%d (name=%q) missing baseURL or apiKey", i, ep.Name)
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

func normalizeFirecrawlKeys(apiKey string, keys []engine.FirecrawlKey) []engine.FirecrawlKey {
	out := make([]engine.FirecrawlKey, 0, len(keys)+1)
	seen := make(map[string]struct{})
	if strings.TrimSpace(apiKey) != "" {
		key := engine.FirecrawlKey{Name: "primary", APIKey: strings.TrimSpace(apiKey)}
		out = append(out, key)
		seen[key.APIKey] = struct{}{}
	}
	for i, key := range keys {
		key.APIKey = strings.TrimSpace(key.APIKey)
		key.Name = strings.TrimSpace(key.Name)
		if key.APIKey == "" {
			continue
		}
		if _, ok := seen[key.APIKey]; ok {
			continue
		}
		if key.Name == "" {
			key.Name = fmt.Sprintf("key-%d", i)
		}
		out = append(out, key)
		seen[key.APIKey] = struct{}{}
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
