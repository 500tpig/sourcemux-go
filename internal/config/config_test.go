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
	if !strings.Contains(err.Error(), "config file not found: grok-search.json") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoad_ReadsOnlyDefaultLocalFile(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	t.Setenv("GROK_API_URL", "https://env.example/v1")
	t.Setenv("GROK_API_KEY", "sk-env")

	hiddenDir := filepath.Join(dir, ".config", "grok-search")
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
	      "apiType": "responses"
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

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "grok-search.json")
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
