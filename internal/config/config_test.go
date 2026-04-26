package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
		"JINA_API_URL", "JINA_API_KEY",
		"GROK_DEBUG", "GROK_LOG_LEVEL",
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
