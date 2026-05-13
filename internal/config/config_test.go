package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoad_MissingDefaultConfigErrors(t *testing.T) {
	chdir(t, t.TempDir())

	_, err := Load()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "config file not found: sourcemux.json") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoad_ReadsOnlyDefaultLocalFile(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	t.Setenv("GROK_API_URL", "https://env.example/v1")
	t.Setenv("GROK_API_KEY", "sk-env")

	hiddenDir := filepath.Join(dir, ".config", "sourcemux")
	if err := os.MkdirAll(hiddenDir, 0o755); err != nil {
		t.Fatalf("mkdir hidden config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(hiddenDir, "config.json"), []byte(`{"grokEndpoints":[{"name":"hidden","baseURL":"https://hidden/v1","apiKey":"sk-hidden"}]}`), 0o600); err != nil {
		t.Fatalf("write hidden config: %v", err)
	}
	if err := os.WriteFile(DefaultConfigPath(), []byte(`{"grokEndpoints":[{"name":"local","baseURL":"https://local","apiKey":"sk-local"}]}`), 0o600); err != nil {
		t.Fatalf("write local config: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(cfg.GrokEndpoints) != 1 {
		t.Fatalf("endpoint len = %d, want 1", len(cfg.GrokEndpoints))
	}
	ep := cfg.GrokEndpoints[0]
	if ep.Name != "local" || ep.BaseURL != "https://local/v1" || ep.APIKey != "sk-local" {
		t.Fatalf("loaded endpoint = %+v, want local file only", ep)
	}
}

func TestLoadFile_LoadsSingleConfig(t *testing.T) {
	path := writeConfig(t, `{
	  "grokEndpoints": [
	    {
	      "name": "primary",
	      "baseURL": "https://grok.example",
	      "apiKey": "sk-primary",
	      "model": "grok-4.20-fast",
	      "sendSearchFlag": false,
	      "apiType": "responses",
	      "responseTools": [" web_search ", "x_search", "web_search"]
	    }
	  ],
	  "reasoningEndpoints": [
	    {
	      "name": "deepseek",
	      "baseURL": "https://api.deepseek.com",
	      "apiKey": "sk-deepseek",
	      "model": "deepseek-v4-flash"
	    }
	  ],
	  "tavily": {"apiURL": "https://tavily.test", "apiKey": "tvly-test", "enabled": false},
	  "exa": {"apiURL": "https://exa.test", "apiKey": "exa-test", "enabled": true},
	  "jina": {"apiURL": "https://jina.test", "apiKey": "jina-test"},
	  "tinyfish": {
	    "enabled": true,
	    "searchURL": "https://tinyfish-search.test",
	    "fetchURL": "https://tinyfish-fetch.test",
	    "keys": [{"name": "acct-a", "apiKey": "tf-key"}]
	  },
	  "debug": true,
	  "logLevel": "DEBUG",
	  "grokPoolTimeoutSec": 45
	}`)

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile failed: %v", err)
	}
	if len(cfg.GrokEndpoints) != 1 {
		t.Fatalf("endpoint len = %d, want 1", len(cfg.GrokEndpoints))
	}
	ep := cfg.GrokEndpoints[0]
	if ep.BaseURL != "https://grok.example/v1" || ep.APIType != "responses" || ep.Model != "grok-4.20-fast" {
		t.Fatalf("endpoint = %+v", ep)
	}
	if got := strings.Join(ep.ResponseTools, ","); got != "web_search,x_search" {
		t.Fatalf("responseTools = %v", ep.ResponseTools)
	}
	if len(cfg.ReasoningEndpoints) != 1 {
		t.Fatalf("reasoning endpoints len = %d, want 1", len(cfg.ReasoningEndpoints))
	}
	rep := cfg.ReasoningEndpoints[0]
	if rep.Name != "deepseek" || rep.BaseURL != "https://api.deepseek.com/v1" || rep.APIKey != "sk-deepseek" || rep.Model != "deepseek-v4-flash" {
		t.Fatalf("reasoning endpoint = %+v", rep)
	}
	if cfg.TavilyEnabled || cfg.TavilyAPIURL != "https://tavily.test" || cfg.TavilyAPIKey != "tvly-test" {
		t.Fatalf("tavily config = enabled=%v url=%q key=%q", cfg.TavilyEnabled, cfg.TavilyAPIURL, cfg.TavilyAPIKey)
	}
	if !cfg.ExaEnabled || cfg.ExaAPIURL != "https://exa.test" || cfg.ExaAPIKey != "exa-test" {
		t.Fatalf("exa config = enabled=%v url=%q key=%q", cfg.ExaEnabled, cfg.ExaAPIURL, cfg.ExaAPIKey)
	}
	if cfg.JinaAPIURL != "https://jina.test" || cfg.JinaAPIKey != "jina-test" {
		t.Fatalf("jina config = url=%q key=%q", cfg.JinaAPIURL, cfg.JinaAPIKey)
	}
	if len(cfg.TinyFishKeys) != 1 || cfg.TinyFishKeys[0].Name != "acct-a" || cfg.TinyFishKeys[0].APIKey != "tf-key" {
		t.Fatalf("tinyfish keys = %+v", cfg.TinyFishKeys)
	}
	if cfg.TinyFishSearchURL != "https://tinyfish-search.test" || cfg.TinyFishFetchURL != "https://tinyfish-fetch.test" {
		t.Fatalf("tinyfish urls = %q %q", cfg.TinyFishSearchURL, cfg.TinyFishFetchURL)
	}
	if !cfg.Debug || cfg.LogLevel != "DEBUG" || cfg.GrokPoolTimeout != 45*time.Second {
		t.Fatalf("debug/log/timeout = %v %q %s", cfg.Debug, cfg.LogLevel, cfg.GrokPoolTimeout)
	}
}

func TestLoadFile_AllowsProviderOnlyConfig(t *testing.T) {
	path := writeConfig(t, `{"exa":{"apiKey":"exa-test"}}`)

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile failed: %v", err)
	}
	if len(cfg.GrokEndpoints) != 0 {
		t.Fatalf("endpoints len = %d, want 0", len(cfg.GrokEndpoints))
	}
	if cfg.ExaAPIKey != "exa-test" || !cfg.ExaEnabled {
		t.Fatalf("exa config = enabled=%v key=%q", cfg.ExaEnabled, cfg.ExaAPIKey)
	}
	if cfg.TavilyAPIURL != "https://api.tavily.com" || !cfg.TavilyEnabled {
		t.Fatalf("default tavily = enabled=%v url=%q", cfg.TavilyEnabled, cfg.TavilyAPIURL)
	}
	if cfg.JinaAPIURL != "https://r.jina.ai" {
		t.Fatalf("default jina url = %q", cfg.JinaAPIURL)
	}
}

func TestLoadFile_InvalidJSONErrors(t *testing.T) {
	path := writeConfig(t, `not json`)

	_, err := LoadFile(path)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "parse config file") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadFile_InvalidEndpointErrors(t *testing.T) {
	path := writeConfig(t, `{"grokEndpoints":[{"name":"bad","baseURL":"https://bad","apiKey":""}]}`)

	_, err := LoadFile(path)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "missing baseURL or apiKey") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadFile_InvalidAPITypeErrors(t *testing.T) {
	path := writeConfig(t, `{"grokEndpoints":[{"name":"bad","baseURL":"https://bad","apiKey":"sk","apiType":"other"}]}`)

	_, err := LoadFile(path)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid apiType") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadFile_InvalidResponseToolsErrors(t *testing.T) {
	path := writeConfig(t, `{"grokEndpoints":[{"name":"bad","baseURL":"https://bad","apiKey":"sk","apiType":"responses","responseTools":["web_search","bad"]}]}`)

	_, err := LoadFile(path)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "responseTools") || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadFile_ResponseToolsRequireResponsesAPI(t *testing.T) {
	path := writeConfig(t, `{"grokEndpoints":[{"name":"bad","baseURL":"https://bad","apiKey":"sk","apiType":"chat","responseTools":["web_search"]}]}`)

	_, err := LoadFile(path)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "responseTools require apiType") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadFile_InvalidReasoningEndpointErrors(t *testing.T) {
	path := writeConfig(t, `{"reasoningEndpoints":[{"name":"bad","baseURL":"https://deepseek","apiKey":""}]}`)

	_, err := LoadFile(path)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "reasoningEndpoints") || !strings.Contains(err.Error(), "missing baseURL or apiKey") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadFile_ReasoningEndpointDefaults(t *testing.T) {
	path := writeConfig(t, `{"reasoningEndpoints":[{"baseURL":"https://api.deepseek.com","apiKey":"sk-test"}]}`)

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile failed: %v", err)
	}
	if len(cfg.ReasoningEndpoints) != 1 {
		t.Fatalf("reasoning endpoints len = %d, want 1", len(cfg.ReasoningEndpoints))
	}
	ep := cfg.ReasoningEndpoints[0]
	if ep.Name != "reasoning-0" || ep.Model != "deepseek-v4-flash" || ep.BaseURL != "https://api.deepseek.com/v1" {
		t.Fatalf("reasoning endpoint defaults = %+v", ep)
	}
}

func TestLoadFile_TinyFishKeyNormalization(t *testing.T) {
	path := writeConfig(t, `{
	  "tinyfish": {
	    "keys": [
	      {"apiKey": "tf-a"},
	      {"name": "blank", "apiKey": "   "},
	      {"name": "named", "apiKey": "tf-b"}
	    ]
	  }
	}`)

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile failed: %v", err)
	}
	if len(cfg.TinyFishKeys) != 2 {
		t.Fatalf("keys len = %d, want 2: %+v", len(cfg.TinyFishKeys), cfg.TinyFishKeys)
	}
	if cfg.TinyFishKeys[0].Name != "key-0" || cfg.TinyFishKeys[0].APIKey != "tf-a" {
		t.Fatalf("first key = %+v", cfg.TinyFishKeys[0])
	}
	if cfg.TinyFishKeys[1].Name != "named" || cfg.TinyFishKeys[1].APIKey != "tf-b" {
		t.Fatalf("second key = %+v", cfg.TinyFishKeys[1])
	}
}

func TestLoadFile_PoolTimeoutZeroDisables(t *testing.T) {
	path := writeConfig(t, `{"grokPoolTimeoutSec":0}`)

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile failed: %v", err)
	}
	if cfg.GrokPoolTimeout != 0 {
		t.Fatalf("timeout = %s, want 0", cfg.GrokPoolTimeout)
	}
}

func TestLoadFile_V2CapabilitiesDeriveFlatView(t *testing.T) {
	path := writeConfig(t, `{
	  "version": 2,
	  "minimum_profile": "standard",
	  "capabilities": {
	    "main_search": {
	      "providers": [
	        {"type": "openai-compatible", "name": "primary", "baseURL": "https://grok.example", "apiKey": "sk-primary", "model": "grok-test"}
	      ]
	    },
	    "docs_search": {
	      "providers": [
	        {"type": "exa", "name": "exa-main", "apiURL": "https://exa.test", "apiKey": "exa-test"},
	        {"type": "context7", "name": "context7-main", "apiURL": "https://context7.test", "apiKey": "ctx7-test", "library_scopes": ["/vercel/*"]},
	        {"type": "context7", "name": "context7-empty", "apiURL": "https://context7-empty.test", "apiKey": ""}
	      ]
	    },
	    "web_fetch": {
	      "providers": [
	        {"type": "jina", "apiURL": "https://jina.test"},
	        {"type": "tinyfish", "keys": [{"name": "tf-a", "apiKey": "tf-key"}], "searchURL": "https://tf-search.test", "fetchURL": "https://tf-fetch.test"},
	        {"type": "tavily", "apiURL": "https://tavily.test", "apiKey": "tvly-test"}
	      ]
	    }
	  },
	  "reasoning_endpoints": [{"name":"deepseek","baseURL":"https://reason.example","apiKey":"sk-reason"}]
	}`)

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile failed: %v", err)
	}
	if cfg.Version != 2 || cfg.MinimumProfile != "standard" {
		t.Fatalf("version/profile = %d/%q", cfg.Version, cfg.MinimumProfile)
	}
	if len(cfg.GrokEndpoints) != 1 || cfg.GrokEndpoints[0].BaseURL != "https://grok.example/v1" {
		t.Fatalf("grok endpoints = %+v", cfg.GrokEndpoints)
	}
	if cfg.ExaAPIURL != "https://exa.test" || cfg.ExaAPIKey != "exa-test" {
		t.Fatalf("exa = %q %q", cfg.ExaAPIURL, cfg.ExaAPIKey)
	}
	if len(cfg.Context7Endpoints) != 1 || cfg.Context7Endpoints[0].Name != "context7-main" || cfg.Context7Endpoints[0].APIKey != "ctx7-test" {
		t.Fatalf("context7 = %+v", cfg.Context7Endpoints)
	}
	if len(cfg.Context7Endpoints[0].LibraryScopes) != 1 || cfg.Context7Endpoints[0].LibraryScopes[0] != "/vercel/*" {
		t.Fatalf("context7 scopes = %+v", cfg.Context7Endpoints[0].LibraryScopes)
	}
	if cfg.JinaAPIURL != "https://jina.test" {
		t.Fatalf("jina = %q", cfg.JinaAPIURL)
	}
	if len(cfg.TinyFishKeys) != 1 || cfg.TinyFishKeys[0].Name != "tf-a" || cfg.TinyFishFetchURL != "https://tf-fetch.test" {
		t.Fatalf("tinyfish = keys=%+v fetch=%q", cfg.TinyFishKeys, cfg.TinyFishFetchURL)
	}
	if cfg.TavilyAPIURL != "https://tavily.test" || cfg.TavilyAPIKey != "tvly-test" {
		t.Fatalf("tavily = %q %q", cfg.TavilyAPIURL, cfg.TavilyAPIKey)
	}
	if len(cfg.ReasoningEndpoints) != 1 || cfg.ReasoningEndpoints[0].BaseURL != "https://reason.example/v1" {
		t.Fatalf("reasoning = %+v", cfg.ReasoningEndpoints)
	}
}

func TestLoadFile_V2RejectsMixedLegacyFields(t *testing.T) {
	path := writeConfig(t, `{
	  "version": 2,
	  "capabilities": {"main_search": {"providers": []}},
	  "grokEndpoints": [{"baseURL":"https://legacy","apiKey":"sk"}]
	}`)

	_, err := LoadFile(path)
	if err == nil {
		t.Fatal("expected mixed config error, got nil")
	}
	if !strings.Contains(err.Error(), "mixes v2 capabilities with legacy") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sourcemux.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(old); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})
}
