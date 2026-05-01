package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// resetConfigEnv clears every env var Load() reads, so each test starts from a
// clean slate regardless of the host environment. Uses t.Setenv so values are
// restored automatically at test end.
func resetConfigEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"GROK_ENDPOINTS_JSON", "GROK_ENDPOINTS_FILE",
		"GROK_API_URL", "GROK_API_KEY", "GROK_NAME", "GROK_MODEL", "GROK_SEND_SEARCH_FLAG",
		"TAVILY_API_URL", "TAVILY_API_KEY", "TAVILY_ENABLED",
		"EXA_API_URL", "EXA_API_KEY", "EXA_ENABLED",
		"JINA_API_URL", "JINA_API_KEY",
		"GROK_DEBUG", "GROK_LOG_LEVEL",
		"GROK_POOL_TIMEOUT_SEC",
	} {
		t.Setenv(k, "")
	}
	// Point the user's home + XDG config dir at an empty tempdir so the
	// default-config-file fallback never picks up a real file from the host.
	isolated := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", isolated)
	t.Setenv("HOME", isolated)
}

func TestLoad_NoEndpointsErrors(t *testing.T) {
	resetConfigEnv(t)
	_, err := Load()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no Grok endpoints configured") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLoad_LegacySingleEndpointDefaults(t *testing.T) {
	resetConfigEnv(t)
	t.Setenv("GROK_API_URL", "https://example.com/v1")
	t.Setenv("GROK_API_KEY", "sk-test")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(cfg.GrokEndpoints) != 1 {
		t.Fatalf("len = %d, want 1", len(cfg.GrokEndpoints))
	}
	ep := cfg.GrokEndpoints[0]
	if ep.BaseURL != "https://example.com/v1" || ep.APIKey != "sk-test" {
		t.Errorf("endpoint = %+v", ep)
	}
	if ep.Name != "default" {
		t.Errorf("default Name = %q, want default", ep.Name)
	}
	if ep.Model != "grok-3-mini" {
		t.Errorf("default Model = %q, want grok-3-mini", ep.Model)
	}
	if !ep.SendSearchFlag {
		t.Error("legacy default SendSearchFlag = false, want true")
	}
}

func TestLoad_LegacySingleEndpointSendSearchFlagFalse(t *testing.T) {
	resetConfigEnv(t)
	t.Setenv("GROK_API_URL", "https://example.com/v1")
	t.Setenv("GROK_API_KEY", "sk-test")
	t.Setenv("GROK_SEND_SEARCH_FLAG", "false")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.GrokEndpoints[0].SendSearchFlag {
		t.Error("SendSearchFlag = true, want false")
	}
}

func TestLoad_EndpointsJSONFillsDefaults(t *testing.T) {
	resetConfigEnv(t)
	t.Setenv("GROK_ENDPOINTS_JSON", `[
		{"name":"a","baseURL":"https://a/v1","apiKey":"k1","model":"grok-x"},
		{"baseURL":"https://b/v1","apiKey":"k2"}
	]`)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(cfg.GrokEndpoints) != 2 {
		t.Fatalf("len = %d, want 2", len(cfg.GrokEndpoints))
	}
	if cfg.GrokEndpoints[0].Model != "grok-x" {
		t.Errorf("explicit Model = %q", cfg.GrokEndpoints[0].Model)
	}
	if cfg.GrokEndpoints[1].Name != "endpoint-1" {
		t.Errorf("auto Name = %q, want endpoint-1", cfg.GrokEndpoints[1].Name)
	}
	if cfg.GrokEndpoints[1].Model != "grok-3-mini" {
		t.Errorf("default Model = %q, want grok-3-mini", cfg.GrokEndpoints[1].Model)
	}
}

func TestLoad_EndpointsJSONNormalizesBaseURL(t *testing.T) {
	resetConfigEnv(t)
	t.Setenv("GROK_ENDPOINTS_JSON", `[
		{"name":"root","baseURL":"https://example.com","apiKey":"k1"},
		{"name":"slash","baseURL":"https://example.org/","apiKey":"k2"},
		{"name":"already","baseURL":"https://example.net/v1","apiKey":"k3"}
	]`)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	got := []string{
		cfg.GrokEndpoints[0].BaseURL,
		cfg.GrokEndpoints[1].BaseURL,
		cfg.GrokEndpoints[2].BaseURL,
	}
	want := []string{
		"https://example.com/v1",
		"https://example.org/v1",
		"https://example.net/v1",
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("BaseURL[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestLoad_EndpointsJSONMissingFieldsErrors(t *testing.T) {
	resetConfigEnv(t)
	t.Setenv("GROK_ENDPOINTS_JSON", `[{"name":"bad","baseURL":"https://b","apiKey":""}]`)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "missing baseURL or apiKey") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLoad_EndpointsJSONInvalidJSONErrors(t *testing.T) {
	resetConfigEnv(t)
	t.Setenv("GROK_ENDPOINTS_JSON", `not json`)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "parse GROK_ENDPOINTS_JSON") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLoad_EndpointsFile(t *testing.T) {
	resetConfigEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "endpoints.json")
	body := `[{"name":"file-ep","baseURL":"https://x/v1","apiKey":"kf"}]`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write tmp file: %v", err)
	}
	t.Setenv("GROK_ENDPOINTS_FILE", path)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(cfg.GrokEndpoints) != 1 || cfg.GrokEndpoints[0].Name != "file-ep" {
		t.Errorf("loaded = %+v", cfg.GrokEndpoints)
	}
}

func TestLoad_EndpointsFileMissingErrors(t *testing.T) {
	resetConfigEnv(t)
	t.Setenv("GROK_ENDPOINTS_FILE", "/nonexistent/endpoints.json")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "read GROK_ENDPOINTS_FILE") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLoad_TavilyEnabledDefaultsTrue(t *testing.T) {
	resetConfigEnv(t)
	t.Setenv("GROK_API_URL", "https://x/v1")
	t.Setenv("GROK_API_KEY", "k")
	t.Setenv("TAVILY_API_KEY", "tvly-x")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if !cfg.TavilyEnabled {
		t.Error("TavilyEnabled = false, want true (default)")
	}
}

func TestLoad_TavilyDisabledExplicit(t *testing.T) {
	resetConfigEnv(t)
	t.Setenv("GROK_API_URL", "https://x/v1")
	t.Setenv("GROK_API_KEY", "k")
	t.Setenv("TAVILY_ENABLED", "false")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.TavilyEnabled {
		t.Error("TavilyEnabled = true, want false")
	}
}

func TestLoad_DefaultBaseURLs(t *testing.T) {
	resetConfigEnv(t)
	t.Setenv("GROK_API_URL", "https://x/v1")
	t.Setenv("GROK_API_KEY", "k")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.TavilyAPIURL != "https://api.tavily.com" {
		t.Errorf("default TavilyAPIURL = %q", cfg.TavilyAPIURL)
	}
	if cfg.ExaAPIURL != "https://api.exa.ai" {
		t.Errorf("default ExaAPIURL = %q", cfg.ExaAPIURL)
	}
	if !cfg.ExaEnabled {
		t.Error("default ExaEnabled = false, want true")
	}
	if cfg.JinaAPIURL != "https://r.jina.ai" {
		t.Errorf("default JinaAPIURL = %q", cfg.JinaAPIURL)
	}
	if cfg.LogLevel != "INFO" {
		t.Errorf("default LogLevel = %q, want INFO", cfg.LogLevel)
	}
	if cfg.Debug {
		t.Error("default Debug = true, want false")
	}
}

func TestLoad_DebugTrue(t *testing.T) {
	resetConfigEnv(t)
	t.Setenv("GROK_API_URL", "https://x/v1")
	t.Setenv("GROK_API_KEY", "k")
	t.Setenv("GROK_DEBUG", "TRUE")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if !cfg.Debug {
		t.Error("Debug = false, want true (case-insensitive)")
	}
}

func TestLoad_DefaultXDGFile(t *testing.T) {
	resetConfigEnv(t)
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, "grok-search")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := `[{"name":"xdg","baseURL":"https://x/v1","apiKey":"kx"}]`
	if err := os.WriteFile(filepath.Join(cfgDir, "endpoints.json"), []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(cfg.GrokEndpoints) != 1 || cfg.GrokEndpoints[0].Name != "xdg" {
		t.Errorf("loaded = %+v", cfg.GrokEndpoints)
	}
}

func TestLoad_DefaultAppConfigFile(t *testing.T) {
	resetConfigEnv(t)
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, "grok-search")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := `{
		"grokEndpoints": [
			{"name":"app","baseURL":"https://app","apiKey":"ka","model":"grok-4.20-fast","sendSearchFlag":false}
		],
		"tavily": {
			"apiURL": "https://tavily.example",
			"apiKey": "tvly-file",
			"enabled": false
		},
		"exa": {
			"apiURL": "https://exa.example",
			"apiKey": "exa-file",
			"enabled": false
		},
		"jina": {
			"apiURL": "https://jina.example",
			"apiKey": "jina-file"
		},
		"debug": true,
		"logLevel": "DEBUG",
		"grokPoolTimeoutSec": 45
	}`
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(cfg.GrokEndpoints) != 1 || cfg.GrokEndpoints[0].Name != "app" {
		t.Fatalf("loaded endpoints = %+v", cfg.GrokEndpoints)
	}
	if cfg.GrokEndpoints[0].BaseURL != "https://app/v1" {
		t.Errorf("BaseURL = %q, want normalized /v1", cfg.GrokEndpoints[0].BaseURL)
	}
	if cfg.TavilyAPIURL != "https://tavily.example" || cfg.TavilyAPIKey != "tvly-file" || cfg.TavilyEnabled {
		t.Errorf("tavily config = url:%q key:%q enabled:%v", cfg.TavilyAPIURL, cfg.TavilyAPIKey, cfg.TavilyEnabled)
	}
	if cfg.ExaAPIURL != "https://exa.example" || cfg.ExaAPIKey != "exa-file" || cfg.ExaEnabled {
		t.Errorf("exa config = url:%q key:%q enabled:%v", cfg.ExaAPIURL, cfg.ExaAPIKey, cfg.ExaEnabled)
	}
	if cfg.JinaAPIURL != "https://jina.example" || cfg.JinaAPIKey != "jina-file" {
		t.Errorf("jina config = url:%q key:%q", cfg.JinaAPIURL, cfg.JinaAPIKey)
	}
	if !cfg.Debug || cfg.LogLevel != "DEBUG" || cfg.GrokPoolTimeout != 45*time.Second {
		t.Errorf("general config = debug:%v log:%q timeout:%v", cfg.Debug, cfg.LogLevel, cfg.GrokPoolTimeout)
	}
}

func TestLoad_DefaultAppConfigSupportsEndpointsAlias(t *testing.T) {
	resetConfigEnv(t)
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, "grok-search")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := `{"endpoints":[{"name":"alias","baseURL":"https://alias/v1","apiKey":"ka"}]}`
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(cfg.GrokEndpoints) != 1 || cfg.GrokEndpoints[0].Name != "alias" {
		t.Errorf("loaded endpoints = %+v", cfg.GrokEndpoints)
	}
}

func TestLoad_DefaultAppConfigWithoutEndpointsFallsBackToEndpointsFile(t *testing.T) {
	resetConfigEnv(t)
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, "grok-search")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	appBody := `{"tavily":{"apiKey":"tvly-file","enabled":true}}`
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte(appBody), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	endpointsBody := `[{"name":"legacy","baseURL":"https://legacy/v1","apiKey":"kl"}]`
	if err := os.WriteFile(filepath.Join(cfgDir, "endpoints.json"), []byte(endpointsBody), 0o644); err != nil {
		t.Fatalf("write endpoints: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(cfg.GrokEndpoints) != 1 || cfg.GrokEndpoints[0].Name != "legacy" {
		t.Errorf("loaded endpoints = %+v", cfg.GrokEndpoints)
	}
	if cfg.TavilyAPIKey != "tvly-file" {
		t.Errorf("TavilyAPIKey = %q, want tvly-file", cfg.TavilyAPIKey)
	}
}

func TestLoad_EnvOverridesDefaultAppConfig(t *testing.T) {
	resetConfigEnv(t)
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, "grok-search")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := `{
		"grokEndpoints": [{"name":"file","baseURL":"https://file/v1","apiKey":"kf"}],
		"tavily": {"apiURL":"https://file-tavily","apiKey":"tvly-file","enabled":true},
		"exa": {"apiURL":"https://file-exa","apiKey":"exa-file","enabled":true},
		"jina": {"apiURL":"https://file-jina","apiKey":"jina-file"},
		"debug": true,
		"logLevel": "DEBUG",
		"grokPoolTimeoutSec": 45
	}`
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("GROK_ENDPOINTS_JSON", `[{"name":"env","baseURL":"https://env/v1","apiKey":"ke"}]`)
	t.Setenv("TAVILY_API_URL", "https://env-tavily")
	t.Setenv("TAVILY_API_KEY", "tvly-env")
	t.Setenv("TAVILY_ENABLED", "false")
	t.Setenv("EXA_API_URL", "https://env-exa")
	t.Setenv("EXA_API_KEY", "exa-env")
	t.Setenv("EXA_ENABLED", "false")
	t.Setenv("JINA_API_URL", "https://env-jina")
	t.Setenv("JINA_API_KEY", "jina-env")
	t.Setenv("GROK_DEBUG", "false")
	t.Setenv("GROK_LOG_LEVEL", "WARN")
	t.Setenv("GROK_POOL_TIMEOUT_SEC", "10")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.GrokEndpoints[0].Name != "env" {
		t.Errorf("endpoint name = %q, want env", cfg.GrokEndpoints[0].Name)
	}
	if cfg.TavilyAPIURL != "https://env-tavily" || cfg.TavilyAPIKey != "tvly-env" || cfg.TavilyEnabled {
		t.Errorf("tavily env override failed: url:%q key:%q enabled:%v", cfg.TavilyAPIURL, cfg.TavilyAPIKey, cfg.TavilyEnabled)
	}
	if cfg.ExaAPIURL != "https://env-exa" || cfg.ExaAPIKey != "exa-env" || cfg.ExaEnabled {
		t.Errorf("exa env override failed: url:%q key:%q enabled:%v", cfg.ExaAPIURL, cfg.ExaAPIKey, cfg.ExaEnabled)
	}
	if cfg.JinaAPIURL != "https://env-jina" || cfg.JinaAPIKey != "jina-env" {
		t.Errorf("jina env override failed: url:%q key:%q", cfg.JinaAPIURL, cfg.JinaAPIKey)
	}
	if cfg.Debug || cfg.LogLevel != "WARN" || cfg.GrokPoolTimeout != 10*time.Second {
		t.Errorf("general env override failed: debug:%v log:%q timeout:%v", cfg.Debug, cfg.LogLevel, cfg.GrokPoolTimeout)
	}
}

func TestLoad_DefaultAppConfigInvalidJSONErrors(t *testing.T) {
	resetConfigEnv(t)
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, "grok-search")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte("not json"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", dir)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "parse ") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLoad_DefaultHomeFile(t *testing.T) {
	resetConfigEnv(t)
	home := t.TempDir()
	cfgDir := filepath.Join(home, ".config", "grok-search")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := `[{"name":"home","baseURL":"https://h/v1","apiKey":"kh"}]`
	if err := os.WriteFile(filepath.Join(cfgDir, "endpoints.json"), []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	// Unset XDG_CONFIG_HOME so the HOME-based branch runs.
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", home)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(cfg.GrokEndpoints) != 1 || cfg.GrokEndpoints[0].Name != "home" {
		t.Errorf("loaded = %+v", cfg.GrokEndpoints)
	}
}

func TestLoad_DefaultFileInvalidJSONErrors(t *testing.T) {
	resetConfigEnv(t)
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, "grok-search")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "endpoints.json"), []byte("not json"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", dir)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "parse ") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLoad_EnvOverridesDefaultFile(t *testing.T) {
	resetConfigEnv(t)
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, "grok-search")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := `[{"name":"file","baseURL":"https://file/v1","apiKey":"kf"}]`
	if err := os.WriteFile(filepath.Join(cfgDir, "endpoints.json"), []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", dir)
	// Env legacy vars should still win over the default file.
	t.Setenv("GROK_API_URL", "https://env/v1")
	t.Setenv("GROK_API_KEY", "kenv")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.GrokEndpoints[0].BaseURL != "https://env/v1" {
		t.Errorf("env should override default file, got BaseURL=%q", cfg.GrokEndpoints[0].BaseURL)
	}
}

func TestLoad_PoolTimeout(t *testing.T) {
	resetConfigEnv(t)
	t.Setenv("GROK_API_URL", "https://x/v1")
	t.Setenv("GROK_API_KEY", "k")
	t.Setenv("GROK_POOL_TIMEOUT_SEC", "45")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.GrokPoolTimeout != 45*time.Second {
		t.Errorf("GrokPoolTimeout = %v, want 45s", cfg.GrokPoolTimeout)
	}
}

func TestLoad_PoolTimeoutInvalidIgnored(t *testing.T) {
	resetConfigEnv(t)
	t.Setenv("GROK_API_URL", "https://x/v1")
	t.Setenv("GROK_API_KEY", "k")
	t.Setenv("GROK_POOL_TIMEOUT_SEC", "abc")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.GrokPoolTimeout != 0 {
		t.Errorf("GrokPoolTimeout = %v, want 0 (invalid input ignored)", cfg.GrokPoolTimeout)
	}
}

func TestLoad_PoolTimeoutZeroDisabled(t *testing.T) {
	resetConfigEnv(t)
	t.Setenv("GROK_API_URL", "https://x/v1")
	t.Setenv("GROK_API_KEY", "k")
	t.Setenv("GROK_POOL_TIMEOUT_SEC", "0")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.GrokPoolTimeout != 0 {
		t.Errorf("GrokPoolTimeout = %v, want 0", cfg.GrokPoolTimeout)
	}
}
