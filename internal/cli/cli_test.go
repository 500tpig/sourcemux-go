package cli

import (
	"context"
	"encoding/json"
	"flag"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	cfgpkg "github.com/500tpig/sourcemux-go/internal/config"
	"github.com/500tpig/sourcemux-go/internal/engine"
	"github.com/500tpig/sourcemux-go/internal/tools"
)

func TestRunUsageOnEmpty(t *testing.T) {
	if got := Run([]string{}); got != 0 {
		t.Fatalf("Run([]) = %d, want 0 (usage shown)", got)
	}
}

func TestRunHelpFlags(t *testing.T) {
	for _, flag := range []string{"-h", "--help"} {
		if got := Run([]string{flag}); got != 0 {
			t.Errorf("Run([%q]) = %d, want 0", flag, got)
		}
	}
}

func TestRunUnknownSubcommand(t *testing.T) {
	if got := Run([]string{"frobnicate"}); got != 2 {
		t.Errorf("Run([frobnicate]) = %d, want 2", got)
	}
}

func TestRunDoctorHelp(t *testing.T) {
	if got := Run([]string{"doctor", "--help"}); got != 0 {
		t.Fatalf("Run(doctor --help) = %d, want 0", got)
	}
}

func TestRunDoctorDoesNotProbeEndpoints(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"data":[]}`)
	}))
	t.Cleanup(srv.Close)

	path := writeCLIConfig(t, `{
	  "grokEndpoints": [{"name":"local","baseURL":"`+srv.URL+`/v1","apiKey":"sk-local"}],
	  "exa": {"apiKey": "exa-test"}
	}`)
	out := captureStdout(t, func() {
		if got := Run([]string{"--config", path, "doctor", "--json"}); got != 0 {
			t.Fatalf("Run(doctor --json) = %d, want 0", got)
		}
	})
	if atomic.LoadInt32(&calls) != 0 {
		t.Fatalf("doctor made %d live request(s), want 0", calls)
	}
	if strings.Contains(out, "sk-local") || strings.Contains(out, "exa-test") {
		t.Fatalf("doctor leaked secret in %s", out)
	}
	var parsed doctorOutput
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatal(err)
	}
	if !parsed.OK {
		t.Fatalf("doctor ok = false: %+v", parsed)
	}
}

func TestMinimumProfileStandardFailsSearchAndFetchWithExit3(t *testing.T) {
	path := writeCLIConfig(t, `{
	  "version": 2,
	  "minimum_profile": "standard",
	  "capabilities": {
	    "main_search": {"providers": []},
	    "docs_search": {"providers": []},
	    "web_fetch": {"providers": []},
	    "web_enhance": {"providers": []}
	  }
	}`)

	searchOut := captureStdout(t, func() {
		if got := Run([]string{"--config", path, "search", "hello", "--json"}); got != 3 {
			t.Fatalf("Run(search --json) = %d, want 3", got)
		}
	})
	for _, want := range []string{"minimum_profile=standard", "main_search", "docs_search", "web_fetch"} {
		if !strings.Contains(searchOut, want) {
			t.Fatalf("search output missing %q in %s", want, searchOut)
		}
	}

	fetchOut := captureStdout(t, func() {
		if got := Run([]string{"--config", path, "fetch", "https://example.com", "--json"}); got != 3 {
			t.Fatalf("Run(fetch --json) = %d, want 3", got)
		}
	})
	for _, want := range []string{"minimum_profile=standard", "main_search", "docs_search", "web_fetch"} {
		if !strings.Contains(fetchOut, want) {
			t.Fatalf("fetch output missing %q in %s", want, fetchOut)
		}
	}
}

func TestRunSetupHelp(t *testing.T) {
	if got := Run([]string{"setup", "--help"}); got != 0 {
		t.Fatalf("Run(setup --help) = %d, want 0", got)
	}
}

func TestRunSmokeMock(t *testing.T) {
	out := captureStdout(t, func() {
		if got := Run([]string{"smoke", "--mock", "--json"}); got != 0 {
			t.Fatalf("Run(smoke --mock --json) = %d, want 0", got)
		}
	})
	var parsed smokeOutput
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatal(err)
	}
	if !parsed.OK || parsed.RouteTrace.FinalProvider != "mock-ok" || !parsed.RouteTrace.FallbackTriggered {
		t.Fatalf("smoke output = %+v", parsed)
	}
}

func TestKeyStatusMasking(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", "(not set)"},
		{"short", "****"},
		{"abcd1234efgh5678", "abcd...5678"},
	}
	for _, c := range cases {
		if got := keyStatus(c.in); got != c.want {
			t.Errorf("keyStatus(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestConfigPathOutputJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "custom.sourcemux.json")
	out := captureStdout(t, func() {
		if got := Run([]string{"--config", path, "config", "path", "--json"}); got != 0 {
			t.Fatalf("Run(config path --json) = %d, want 0", got)
		}
	})

	var parsed configPathOutput
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.ConfigFile != path {
		t.Fatalf("config_file = %q, want %q", parsed.ConfigFile, path)
	}
	if parsed.AbsConfigFile == "" || !filepath.IsAbs(parsed.AbsConfigFile) {
		t.Fatalf("abs_config_file = %q, want absolute path", parsed.AbsConfigFile)
	}
	if parsed.Exists {
		t.Fatalf("exists = true, want false for missing temp path")
	}
}

func TestRunBlankConfigPathIsUsageError(t *testing.T) {
	cases := [][]string{
		{"--config", "", "config", "path"},
		{"--config=", "config", "path"},
		{"-c", "  ", "config", "path"},
	}
	for _, args := range cases {
		if got := Run(args); got != 2 {
			t.Fatalf("Run(%#v) = %d, want 2", args, got)
		}
	}
}

func TestConfigFilesOutputShowsSingleActiveFileOnly(t *testing.T) {
	path := writeCLIConfig(t, `{"grokEndpoints":[{"name":"a","baseURL":"https://a/v1","apiKey":"sk-a"}]}`)

	out := captureStdout(t, func() {
		if got := Run([]string{"--config", path, "config", "files", "--json"}); got != 0 {
			t.Fatalf("Run(config files --json) = %d, want 0", got)
		}
	})
	if strings.Contains(out, "sk-a") {
		t.Fatalf("config files leaked secret in %s", out)
	}
	var parsed configFilesOutput
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.ConfigFile.Path != path || !parsed.ConfigFile.Exists {
		t.Fatalf("config file status = %+v", parsed.ConfigFile)
	}
	if len(parsed.Notes) == 0 || !strings.Contains(strings.Join(parsed.Notes, "\n"), "Only this file is loaded") {
		t.Fatalf("notes = %+v", parsed.Notes)
	}
	notes := strings.Join(parsed.Notes, "\n")
	if !strings.Contains(notes, "sourcemux cli --config") || !strings.Contains(notes, path) {
		t.Fatalf("custom-path setup note missing: %+v", parsed.Notes)
	}
}

func TestConfigListMasksSecrets(t *testing.T) {
	path := writeCLIConfig(t, `{
	  "grokEndpoints": [{"name":"file","baseURL":"https://file.example/v1","apiKey":"sk-super-secret","model":"grok-test","apiType":"responses","sendSearchFlag":true,"responseTools":["web_search","x_search"]}],
	  "reasoningEndpoints": [{"name":"deepseek","baseURL":"https://api.deepseek.com/v1","apiKey":"sk-deepseek-secret","model":"deepseek-v4-flash"}],
	  "tavily": {"apiKey": "tvly-secret"},
	  "exa": {"apiKey": "exa-secret"},
	  "jina": {"apiKey": "jina-secret"},
	  "tinyfish": {"keys": [{"name":"a","apiKey":"tf-secret-a"},{"name":"b","apiKey":"tf-secret-b"}]}
	}`)

	out := captureStdout(t, func() {
		if got := Run([]string{"--config", path, "config", "list", "--json"}); got != 0 {
			t.Fatalf("Run(config list --json) = %d, want 0", got)
		}
	})
	for _, secret := range []string{"sk-super-secret", "sk-deepseek-secret", "tvly-secret", "exa-secret", "jina-secret", "tf-secret-a", "tf-secret-b"} {
		if strings.Contains(out, secret) {
			t.Fatalf("config list leaked secret %q in %s", secret, out)
		}
	}

	var parsed configListOutput
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.GrokEndpoints) != 1 || parsed.GrokEndpoints[0].KeyStatus == "" {
		t.Fatalf("grok endpoint key status missing: %+v", parsed.GrokEndpoints)
	}
	if strings.Join(parsed.GrokEndpoints[0].ResponseTools, ",") != "web_search,x_search" {
		t.Fatalf("grok endpoint response tools missing: %+v", parsed.GrokEndpoints[0])
	}
	if len(parsed.ReasoningEndpoints) != 1 || parsed.ReasoningEndpoints[0].KeyStatus == "" {
		t.Fatalf("reasoning endpoint key status missing: %+v", parsed.ReasoningEndpoints)
	}
	if parsed.TavilyKey == "(not set)" || len(parsed.TinyFishKeys) != 2 {
		t.Fatalf("masked provider keys missing: %+v", parsed)
	}
}

func TestConfigListAllowsProviderOnlyConfig(t *testing.T) {
	path := writeCLIConfig(t, `{"exa":{"apiKey":"exa-only-secret"}}`)

	out := captureStdout(t, func() {
		if got := Run([]string{"--config", path, "config", "list", "--json"}); got != 0 {
			t.Fatalf("Run(config list provider-only --json) = %d, want 0", got)
		}
	})
	if strings.Contains(out, "exa-only-secret") {
		t.Fatalf("config list leaked provider-only secret in %s", out)
	}

	var parsed configListOutput
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.GrokEndpoints) != 0 {
		t.Fatalf("grok_endpoints len = %d, want 0", len(parsed.GrokEndpoints))
	}
	if parsed.ExaKey == "(not set)" {
		t.Fatalf("exa key status was not reported: %+v", parsed)
	}
}

func TestConfigMigrateWritesV2WithBackupAndMaskedOutput(t *testing.T) {
	path := writeCLIConfig(t, `{
	  "grokEndpoints": [{"name":"file","baseURL":"https://file.example/v1","apiKey":"sk-migrate-secret","model":"grok-test"}],
	  "exa": {"apiKey": "exa-migrate-secret"}
	}`)

	out := captureStdout(t, func() {
		if got := Run([]string{"--config", path, "config", "migrate", "--json"}); got != 0 {
			t.Fatalf("Run(config migrate --json) = %d, want 0", got)
		}
	})
	for _, secret := range []string{"sk-migrate-secret", "exa-migrate-secret"} {
		if strings.Contains(out, secret) {
			t.Fatalf("config migrate output leaked secret %q in %s", secret, out)
		}
	}
	var parsed configMigrateOutput
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatal(err)
	}
	if !parsed.Changed || parsed.BackupFile == "" {
		t.Fatalf("migrate output = %+v", parsed)
	}
	if _, err := os.Stat(parsed.BackupFile); err != nil {
		t.Fatalf("backup missing: %v", err)
	}
	cfg, err := cfgpkg.LoadFile(path)
	if err != nil {
		t.Fatalf("Load migrated config failed: %v", err)
	}
	if cfg.Version != 2 || cfg.MinimumProfile != "off" {
		t.Fatalf("version/profile = %d/%q", cfg.Version, cfg.MinimumProfile)
	}
	if len(cfg.GrokEndpoints) != 1 || cfg.GrokEndpoints[0].APIKey != "sk-migrate-secret" {
		t.Fatalf("migrated endpoints = %+v", cfg.GrokEndpoints)
	}
	if cfg.ExaAPIKey != "exa-migrate-secret" {
		t.Fatalf("migrated exa key = %q", cfg.ExaAPIKey)
	}
}

func TestConfigListMissingConfigReportsNextSteps(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.json")
	out := captureStdout(t, func() {
		if got := Run([]string{"--config", path, "config", "list", "--json"}); got != 1 {
			t.Fatalf("Run(config list --json) = %d, want 1", got)
		}
	})
	for _, want := range []string{`"error"`, `"next_steps"`, "config file not found", "sourcemux cli --config", path, "setup"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %s", want, out)
		}
	}
}

func TestSetupNonInteractiveWritesConfigAndMasksOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sourcemux.json")
	out := captureStdout(t, func() {
		got := Run([]string{
			"--config", path,
			"setup",
			"--non-interactive",
			"--api-url", "https://setup.example/v1",
			"--api-key", "sk-setup-secret",
			"--model", "grok-setup",
			"--api-type", "responses",
			"--send-search-flag",
			"--response-tools", "web_search,x_search",
			"--tavily-key", "tvly-setup-secret",
			"--context7-key", "ctx7-setup-secret",
			"--tinyfish-keys", "tf-setup-a,tf-setup-b",
			"--tinyfish-key-names", "a,b",
			"--json",
		})
		if got != 0 {
			t.Fatalf("Run(setup) = %d, want 0", got)
		}
	})
	for _, secret := range []string{"sk-setup-secret", "tvly-setup-secret", "ctx7-setup-secret", "tf-setup-a", "tf-setup-b"} {
		if strings.Contains(out, secret) {
			t.Fatalf("setup output leaked secret %q in %s", secret, out)
		}
	}

	var parsed setupOutput
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.ConfigFile != path {
		t.Fatalf("config_file = %q, want %q", parsed.ConfigFile, path)
	}
	if strings.Join(parsed.Endpoint.ResponseTools, ",") != "web_search,x_search" {
		t.Fatalf("setup output response tools = %+v", parsed.Endpoint)
	}
	if _, err := os.Stat(parsed.ConfigFile); err != nil {
		t.Fatalf("config file not written: %v", err)
	}

	cfg, err := cfgpkg.LoadFile(path)
	if err != nil {
		t.Fatalf("Load after setup failed: %v", err)
	}
	if len(cfg.GrokEndpoints) != 1 || cfg.GrokEndpoints[0].BaseURL != "https://setup.example/v1" {
		t.Fatalf("loaded endpoints = %+v", cfg.GrokEndpoints)
	}
	if cfg.GrokEndpoints[0].APIKey != "sk-setup-secret" {
		t.Fatalf("setup did not write endpoint key")
	}
	if cfg.GrokEndpoints[0].APIType != "responses" || !cfg.GrokEndpoints[0].SendSearchFlag || strings.Join(cfg.GrokEndpoints[0].ResponseTools, ",") != "web_search,x_search" {
		t.Fatalf("setup did not write responses tools: %+v", cfg.GrokEndpoints[0])
	}
	if cfg.TavilyAPIKey != "tvly-setup-secret" || len(cfg.TinyFishKeys) != 2 {
		t.Fatalf("provider keys not loaded: tavily=%q tinyfish=%+v", cfg.TavilyAPIKey, cfg.TinyFishKeys)
	}
	if len(cfg.Context7Endpoints) != 1 || cfg.Context7Endpoints[0].APIKey != "ctx7-setup-secret" {
		t.Fatalf("context7 endpoints = %+v", cfg.Context7Endpoints)
	}
}

func TestSetupRefusesExistingConfigWithoutForce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sourcemux.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"grokEndpoints":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		got := Run([]string{
			"--config", path,
			"setup",
			"--non-interactive",
			"--api-url", "https://setup.example/v1",
			"--api-key", "sk-setup-secret",
			"--json",
		})
		if got != 1 {
			t.Fatalf("Run(setup existing) = %d, want 1", got)
		}
	})
	if !strings.Contains(out, "already exists") || strings.Contains(out, "sk-setup-secret") {
		t.Fatalf("unexpected setup existing output: %s", out)
	}
}

func TestSetupRejectsInvalidResponseTools(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sourcemux.json")
	out := captureStdout(t, func() {
		got := Run([]string{
			"--config", path,
			"setup",
			"--non-interactive",
			"--api-url", "https://setup.example/v1",
			"--api-key", "sk-setup-secret",
			"--api-type", "responses",
			"--response-tools", "bad",
			"--json",
		})
		if got != 1 {
			t.Fatalf("Run(setup invalid response tools) = %d, want 1", got)
		}
	})
	if !strings.Contains(out, "--response-tools") || !strings.Contains(out, "unsupported") {
		t.Fatalf("unexpected invalid response tools output: %s", out)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("config should not be written, stat err=%v", err)
	}
}

func TestSetupResponseToolsRequireResponsesAPI(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sourcemux.json")
	out := captureStdout(t, func() {
		got := Run([]string{
			"--config", path,
			"setup",
			"--non-interactive",
			"--api-url", "https://setup.example/v1",
			"--api-key", "sk-setup-secret",
			"--response-tools", "web_search",
			"--json",
		})
		if got != 1 {
			t.Fatalf("Run(setup response tools with chat API) = %d, want 1", got)
		}
	})
	if !strings.Contains(out, "--response-tools requires --api-type responses") {
		t.Fatalf("unexpected response tools API type output: %s", out)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("config should not be written, stat err=%v", err)
	}
}

func TestSetupForceOverwritesExistingConfig(t *testing.T) {
	path := writeCLIConfig(t, `{"grokEndpoints":[{"name":"old","baseURL":"https://old.example/v1","apiKey":"sk-old"}]}`)

	out := captureStdout(t, func() {
		got := Run([]string{
			"--config", path,
			"setup",
			"--non-interactive",
			"--api-url", "https://new.example/v1",
			"--api-key", "sk-new-secret",
			"--force",
			"--json",
		})
		if got != 0 {
			t.Fatalf("Run(setup --force) = %d, want 0", got)
		}
	})
	if strings.Contains(out, "sk-new-secret") {
		t.Fatalf("setup --force output leaked secret in %s", out)
	}

	cfg, err := cfgpkg.LoadFile(path)
	if err != nil {
		t.Fatalf("Load after force setup failed: %v", err)
	}
	if len(cfg.GrokEndpoints) != 1 {
		t.Fatalf("endpoint len = %d, want 1", len(cfg.GrokEndpoints))
	}
	ep := cfg.GrokEndpoints[0]
	if ep.BaseURL != "https://new.example/v1" || ep.APIKey != "sk-new-secret" {
		t.Fatalf("force setup endpoint = %+v", ep)
	}
}

func TestSearchOutputJSONShape(t *testing.T) {
	s := searchOutput{
		Engine:       "grok",
		Query:        "q",
		Content:      "c",
		SourceURLs:   []string{"u1"},
		SourcesCount: 1,
	}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	for _, want := range []string{
		`"engine":"grok"`,
		`"query":"q"`,
		`"content":"c"`,
		`"source_urls":["u1"]`,
		`"sources_count":1`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

func TestFetchOutputJSONShape(t *testing.T) {
	f := fetchOutput{Source: "jina", URL: "https://x", Content: "md"}
	b, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	for _, want := range []string{`"source":"jina"`, `"url":"https://x"`, `"content":"md"`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

func TestExaSearchOutputJSONShape(t *testing.T) {
	s := exaSearchOutput{
		Source:      "exa-search-advanced",
		Query:       "q",
		SearchType:  "deep",
		ResultCount: 2,
		SourceURLs:  []string{"u1"},
		Content:     "c",
		StructuredOutput: map[string]any{
			"answer": "ok",
		},
	}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	for _, want := range []string{
		`"source":"exa-search-advanced"`,
		`"query":"q"`,
		`"search_type":"deep"`,
		`"result_count":2`,
		`"source_urls":["u1"]`,
		`"structured_output":{"answer":"ok"}`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

func TestExaContentsOutputJSONShape(t *testing.T) {
	f := exaContentsOutput{
		Source:    "exa-contents-advanced",
		URL:       "https://x",
		ResultURL: "https://x",
		Content:   "md",
		Subpages: []exaContentsSubpageOutput{{
			URL:     "https://x/a",
			Title:   "A",
			Content: "sub",
		}},
	}
	b, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	for _, want := range []string{
		`"source":"exa-contents-advanced"`,
		`"url":"https://x"`,
		`"result_url":"https://x"`,
		`"content":"md"`,
		`"subpages":[{"url":"https://x/a","title":"A","content":"sub"}]`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

func TestMapOutputJSONShape(t *testing.T) {
	m := mapOutput{URL: "https://x", URLs: []string{"a", "b"}, Count: 2}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	for _, want := range []string{`"url":"https://x"`, `"urls":["a","b"]`, `"count":2`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

func TestCrawlOutputJSONShape(t *testing.T) {
	c := crawlOutput{
		Source:  "tavily",
		URL:     "https://x",
		BaseURL: "x",
		Results: []engine.TavilyCrawlPage{{
			URL:        "https://x/a",
			RawContent: "md",
		}},
		Count:        1,
		ResponseTime: 0.25,
	}
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	for _, want := range []string{
		`"source":"tavily"`,
		`"url":"https://x"`,
		`"base_url":"x"`,
		`"raw_content":"md"`,
		`"count":1`,
		`"response_time":0.25`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

func TestRunResearchJSONParsesParameters(t *testing.T) {
	runner := &fakeCLIResearchRunner{}
	out := captureStdout(t, func() {
		if got := runResearchWithRunner([]string{
			"SourceMux MCP",
			"--depth", "deep",
			"--platform", "GitHub",
			"--domain", "example.com",
			"--domain", "github.com",
			"--max-fetches", "3",
			"--json",
		}, runner); got != 0 {
			t.Fatalf("runResearchWithRunner = %d, want 0", got)
		}
	})

	if runner.opts.Query != "SourceMux MCP" || runner.opts.Depth != "deep" || runner.opts.Platform != "GitHub" {
		t.Fatalf("runner opts = %+v", runner.opts)
	}
	if strings.Join(runner.opts.Domains, ",") != "example.com,github.com" {
		t.Fatalf("domains = %#v", runner.opts.Domains)
	}
	if runner.opts.MaxFetches != 3 {
		t.Fatalf("max_fetches = %d", runner.opts.MaxFetches)
	}

	var decoded tools.ResearchPack
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out)
	}
	if decoded.Query != "SourceMux MCP" || decoded.EffectiveDepth != "deep" || decoded.Platform != "GitHub" {
		t.Fatalf("decoded metadata = %+v", decoded)
	}
	if decoded.SourceSummary.SelectedForFetch != 3 {
		t.Fatalf("source summary = %+v", decoded.SourceSummary)
	}
}

func TestRunSmartAnswerJSONParsesParameters(t *testing.T) {
	runner := &fakeCLISmartAnswerRunner{}
	out := captureStdout(t, func() {
		if got := runSmartAnswerWithRunner([]string{
			"Should I use DeepSeek?",
			"--depth", "quick",
			"--platform", "GitHub",
			"--domain", "example.com",
			"--max-fetches", "2",
			"--reasoning-endpoint", "deepseek",
			"--reasoning-model", "deepseek-v4-pro",
			"--json",
		}, runner); got != 0 {
			t.Fatalf("runSmartAnswerWithRunner = %d, want 0", got)
		}
	})

	if runner.opts.Query != "Should I use DeepSeek?" || runner.opts.Depth != "quick" || runner.opts.Platform != "GitHub" {
		t.Fatalf("runner opts = %+v", runner.opts)
	}
	if strings.Join(runner.opts.Domains, ",") != "example.com" || runner.opts.MaxFetches != 2 {
		t.Fatalf("domain/max_fetches opts = %+v", runner.opts)
	}
	if runner.opts.ReasoningEndpoint != "deepseek" || runner.opts.ReasoningModel != "deepseek-v4-pro" {
		t.Fatalf("reasoning opts = %+v", runner.opts)
	}

	var decoded tools.SmartAnswerResult
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out)
	}
	if decoded.Answer == "" || decoded.ReasoningModel != "deepseek-v4-pro" {
		t.Fatalf("decoded = %+v", decoded)
	}
}

func TestRunCrawlJSONUsesTavilyConfig(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/crawl" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tvly-test" {
			t.Fatalf("Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"base_url":"example.com","results":[{"url":"https://example.com/a","raw_content":"# A"}],"response_time":0.25}`))
	}))
	defer ts.Close()

	path := writeCLIConfig(t, `{
	  "tavily": {"apiURL": "`+ts.URL+`", "apiKey": "tvly-test", "enabled": true}
	}`)

	out := captureStdout(t, func() {
		if got := Run([]string{"--config", path, "crawl", "https://example.com", "--instructions", "Find docs", "--limit", "1", "--json"}); got != 0 {
			t.Fatalf("Run(crawl) = %d, want 0", got)
		}
	})
	var decoded crawlOutput
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out)
	}
	if decoded.Source != "tavily" || decoded.Count != 1 || decoded.Results[0].RawContent != "# A" {
		t.Fatalf("decoded output = %+v", decoded)
	}
}

func TestRunExaSearchJSONUsesConfig(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["type"] != "deep" {
			t.Fatalf("type = %v", body["type"])
		}
		if body["numResults"] != float64(6) {
			t.Fatalf("numResults = %v", body["numResults"])
		}
		if body["systemPrompt"] != "Prefer official sources" {
			t.Fatalf("systemPrompt = %v", body["systemPrompt"])
		}
		if _, ok := body["outputSchema"].(map[string]any); !ok {
			t.Fatalf("outputSchema missing: %#v", body["outputSchema"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"searchType":"deep",
			"results":[{"title":"Docs","url":"https://example.com/docs","highlights":["official docs"]}],
			"output":{"content":{"answer":"grounded"}}
		}`))
	}))
	defer ts.Close()

	path := writeCLIConfig(t, `{
	  "exa": {"apiURL": "`+ts.URL+`", "apiKey": "exa-test", "enabled": true}
	}`)

	out := captureStdout(t, func() {
		if got := Run([]string{
			"--config", path,
			"exa-search", "hello world",
			"--type", "deep",
			"--num-results", "6",
			"--text",
			"--text-max-characters", "500",
			"--highlights-query", "main points",
			"--system-prompt", "Prefer official sources",
			"--output-schema-json", `{"type":"object","properties":{"answer":{"type":"string"}}}`,
			"--json",
		}); got != 0 {
			t.Fatalf("Run(exa-search) = %d, want 0", got)
		}
	})
	var decoded exaSearchOutput
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out)
	}
	if decoded.Source != "exa-search-advanced" || decoded.SearchType != "deep" || decoded.ResultCount != 1 {
		t.Fatalf("decoded output = %+v", decoded)
	}
}

func TestRunContext7DocsJSONUsesConfig(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/context" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer ctx7-test" {
			t.Fatalf("Authorization = %q", got)
		}
		if r.URL.Query().Get("libraryId") != "/vercel/next.js" {
			t.Fatalf("libraryId = %q", r.URL.Query().Get("libraryId"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"codeSnippets": [{"codeTitle":"Middleware","codeId":"https://example.com/code","codeList":[{"language":"typescript","code":"export function middleware() {}"}]}],
			"infoSnippets": [{"pageId":"https://example.com/docs","breadcrumb":"Routing","content":"Middleware docs"}]
		}`)
	}))
	defer ts.Close()

	path := writeCLIConfig(t, `{
	  "context7": {"apiURL": "`+ts.URL+`", "apiKey": "ctx7-test", "enabled": true}
	}`)
	out := captureStdout(t, func() {
		if got := Run([]string{"--config", path, "context7-docs", "/vercel/next.js", "middleware auth", "--json"}); got != 0 {
			t.Fatalf("Run(context7-docs) = %d, want 0", got)
		}
	})
	if strings.Contains(out, "ctx7-test") {
		t.Fatalf("context7 output leaked secret: %s", out)
	}
	var decoded context7Output
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out)
	}
	if decoded.EndpointName != "context7-main" || decoded.LibraryID != "/vercel/next.js" || decoded.SourcesCount != 2 {
		t.Fatalf("decoded = %+v", decoded)
	}
}

func TestRunDocsSearchSkipsContext7WithoutExplicitLibrary(t *testing.T) {
	var context7Calls int32
	ctx7 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&context7Calls, 1)
		t.Fatalf("context7 should not be called")
	}))
	defer ctx7.Close()
	exa := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" {
			t.Fatalf("unexpected exa path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"requestId":"req","searchType":"auto","results":[{"title":"Docs","url":"https://example.com/docs","highlights":["docs"]}]}`)
	}))
	defer exa.Close()

	path := writeCLIConfig(t, `{
	  "context7": {"apiURL": "`+ctx7.URL+`", "apiKey": "ctx7-test", "enabled": true},
	  "exa": {"apiURL": "`+exa.URL+`", "apiKey": "exa-test", "enabled": true}
	}`)
	out := captureStdout(t, func() {
		if got := Run([]string{"--config", path, "docs-search", "general docs", "--json"}); got != 0 {
			t.Fatalf("Run(docs-search) = %d, want 0", got)
		}
	})
	if atomic.LoadInt32(&context7Calls) != 0 {
		t.Fatalf("context7 calls = %d, want 0", context7Calls)
	}
	var decoded context7Output
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out)
	}
	if decoded.Engine != "Exa Search" || decoded.SourcesCount != 1 {
		t.Fatalf("decoded = %+v", decoded)
	}
}

func TestRunExaContentsJSONUsesConfig(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/contents" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["subpages"] != float64(2) {
			t.Fatalf("subpages = %v", body["subpages"])
		}
		if body["maxAgeHours"] != float64(0) {
			t.Fatalf("maxAgeHours = %v", body["maxAgeHours"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"results":[{"url":"https://example.com/root","text":"root content","subpages":[{"url":"https://example.com/a","title":"A","text":"sub page"}]}],
			"statuses":[{"id":"https://example.com/root","status":"success"}]
		}`))
	}))
	defer ts.Close()

	path := writeCLIConfig(t, `{
	  "exa": {"apiURL": "`+ts.URL+`", "apiKey": "exa-test", "enabled": true}
	}`)

	out := captureStdout(t, func() {
		if got := Run([]string{
			"--config", path,
			"exa-contents", "https://example.com/root",
			"--subpages", "2",
			"--subpage-target", "api",
			"--subpage-target", "docs",
			"--max-age-hours", "0",
			"--json",
		}); got != 0 {
			t.Fatalf("Run(exa-contents) = %d, want 0", got)
		}
	})
	var decoded exaContentsOutput
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out)
	}
	if decoded.Source != "exa-contents-advanced" || len(decoded.Subpages) != 1 || decoded.Subpages[0].URL != "https://example.com/a" {
		t.Fatalf("decoded output = %+v", decoded)
	}
}

type fakeCLIResearchRunner struct {
	opts tools.ResearchOptions
}

func (r *fakeCLIResearchRunner) Run(ctx context.Context, opts tools.ResearchOptions) (tools.ResearchPack, error) {
	r.opts = opts
	return tools.ResearchPack{
		Query:          opts.Query,
		EffectiveDepth: opts.Depth,
		Platform:       opts.Platform,
		Domains:        opts.Domains,
		MaxFetches:     opts.MaxFetches,
		PlanQueries:    []string{opts.Query},
		ExecutedSearches: []tools.ResearchSearchSummary{{
			Query:        opts.Query,
			Engine:       "fake",
			SourcesCount: 1,
		}},
		SourceSummary: tools.ResearchSourceSummary{
			TotalURLs:          3,
			UniqueURLs:         3,
			SelectedForFetch:   opts.MaxFetches,
			SelectedSourceURLs: []string{"https://example.com/a"},
		},
		FetchedPagesSummary: []tools.ResearchFetchedPage{{
			URL:     "https://example.com/a",
			Source:  "fake",
			Success: true,
			Excerpt: "SourceMux MCP is test content.",
		}},
		HighSignalSources: []tools.ResearchSource{{
			URL:         "https://example.com/a",
			Domain:      "example.com",
			Score:       1,
			Occurrences: 1,
		}},
		ConfirmedFacts:   []string{"SourceMux MCP is test content. (source: https://example.com/a)"},
		LikelyInferences: []string{"fake inference"},
		OpenQuestions:    []string{"fake question"},
	}, nil
}

type fakeCLISmartAnswerRunner struct {
	opts tools.SmartAnswerOptions
}

func (r *fakeCLISmartAnswerRunner) Run(ctx context.Context, opts tools.SmartAnswerOptions) (tools.SmartAnswerResult, error) {
	r.opts = opts
	return tools.SmartAnswerResult{
		Query:             opts.Query,
		Answer:            "Use Grok for search and DeepSeek for synthesis.",
		ReasoningEndpoint: opts.ReasoningEndpoint,
		ReasoningModel:    opts.ReasoningModel,
		Research: tools.ResearchPack{
			Query:          opts.Query,
			EffectiveDepth: opts.Depth,
			MaxFetches:     opts.MaxFetches,
			SourceSummary:  tools.ResearchSourceSummary{UniqueURLs: 1},
		},
	}, nil
}

func TestProbeOutputJSONShape(t *testing.T) {
	p := probeOutput{
		TavilyEnabled: true,
		TavilyAPIURL:  "https://api.tavily.com",
		TavilyKey:     "abcd...wxyz",
		ExaEnabled:    true,
		ExaAPIURL:     "https://api.exa.ai",
		ExaKey:        "exa_...1234",
		JinaAPIURL:    "https://r.jina.ai",
		JinaKey:       "(not set)",
		Endpoints: []probeEndpoint{
			{Name: "e1", BaseURL: "https://x/v1", Model: "grok-3", OK: true, ModelsCount: 5, Models: []string{"a"}},
		},
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	for _, want := range []string{
		`"tavily_enabled":true`,
		`"exa_enabled":true`,
		`"exa_key_status":"exa_...1234"`,
		`"jina_key_status":"(not set)"`,
		`"endpoints":[`,
		`"name":"e1"`,
		`"ok":true`,
		`"models_count":5`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()

	fn()

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func writeCLIConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sourcemux.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestParsePositionalInterleaved(t *testing.T) {
	fs := flag.NewFlagSet("x", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	depth := fs.String("depth", "standard", "")
	jsonOut := fs.Bool("json", false, "")

	pos, err := parsePositional(fs, []string{"hello world", "--depth", "deep", "--json"})
	if err != nil {
		t.Fatal(err)
	}
	if len(pos) != 1 || pos[0] != "hello world" {
		t.Errorf("positional = %#v, want [hello world]", pos)
	}
	if *depth != "deep" {
		t.Errorf("depth = %q, want deep", *depth)
	}
	if !*jsonOut {
		t.Errorf("json flag did not get set")
	}
}

func TestParsePositionalFlagsBeforeAndMultiPositional(t *testing.T) {
	fs := flag.NewFlagSet("x", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	platform := fs.String("platform", "", "")

	pos, err := parsePositional(fs, []string{"--platform", "Twitter", "a", "b", "c"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(pos, " ") != "a b c" {
		t.Errorf("positional = %#v, want [a b c]", pos)
	}
	if *platform != "Twitter" {
		t.Errorf("platform = %q, want Twitter", *platform)
	}
}

func TestParsePositionalUnknownFlag(t *testing.T) {
	fs := flag.NewFlagSet("x", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	_, err := parsePositional(fs, []string{"q", "--no-such-flag"})
	if err == nil {
		t.Fatal("expected error for unknown flag, got nil")
	}
}

func TestTinyFishLoadKeysFromEnv(t *testing.T) {
	t.Setenv("TINYFISH_API_KEY", "")
	t.Setenv("TINYFISH_KEYS_FILE", "")
	t.Setenv("TINYFISH_API_KEYS", "tf_key_one, tf_key_two\n")
	t.Setenv("TINYFISH_KEY_NAMES", "first,second")

	keys, err := loadTinyFishKeys("")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 {
		t.Fatalf("keys len = %d, want 2", len(keys))
	}
	if keys[0].Name != "first" || keys[1].Name != "second" {
		t.Fatalf("keys = %+v", keys)
	}
	if got := keyStatus(keys[0].APIKey); got != "tf_k..._one" {
		t.Fatalf("masked key = %q", got)
	}
}

func TestTinyFishLoadKeysFromFile(t *testing.T) {
	t.Setenv("TINYFISH_API_KEYS", "")
	t.Setenv("TINYFISH_API_KEY", "")
	t.Setenv("TINYFISH_KEYS_FILE", "")
	path := filepath.Join(t.TempDir(), "tinyfish-keys.json")
	if err := os.WriteFile(path, []byte(`{"keys":[{"name":"acct-a","apiKey":"tf_secret_a"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	keys, err := loadTinyFishKeys(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys[0].Name != "acct-a" || keys[0].APIKey != "tf_secret_a" {
		t.Fatalf("keys = %+v", keys)
	}
}

func TestTinyFishBenchmarkCasesValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cases.json")
	data := `{
		"search":[{"name":"s","query":"TinyFish docs"}],
		"fetch":[{"name":"f","urls":["https://example.com"],"format":"markdown"}],
		"agent":[{"name":"a","url":"https://example.com","goal":"summarize"}]
	}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}

	cases, err := loadTinyFishBenchmarkCases(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cases.Search) != 1 || len(cases.Fetch) != 1 || len(cases.Agent) != 1 {
		t.Fatalf("cases = %+v", cases)
	}
}

func TestTinyFishSurfaceParsing(t *testing.T) {
	got, err := parseTinyFishSurfaces("search, fetch")
	if err != nil {
		t.Fatal(err)
	}
	if !got["search"] || !got["fetch"] || got["agent"] {
		t.Fatalf("surfaces = %+v", got)
	}
	if _, err := parseTinyFishSurfaces("search,nope"); err == nil {
		t.Fatal("expected error for unknown surface")
	}
}

func TestTinyFishBenchmarkOutputJSONShapeMasksKey(t *testing.T) {
	out := tinyfishBenchmarkOutput{
		GeneratedAt: "2026-05-06T00:00:00Z",
		CasesFile:   "cases.json",
		KeyCount:    1,
		Results: []tinyfishKeyBenchmarkRun{{
			Name:      "acct",
			KeyStatus: keyStatus("tf_1234567890"),
			Search: []tinyfishSearchMeasurement{{
				Case:        "s",
				Query:       "q",
				OK:          true,
				ResultCount: 1,
				SourceURLs:  []string{"https://example.com"},
			}},
		}},
	}
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	for _, want := range []string{
		`"key_status":"tf_1...7890"`,
		`"case":"s"`,
		`"source_urls":["https://example.com"]`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
	if strings.Contains(got, "tf_1234567890") {
		t.Fatalf("full key leaked in JSON: %s", got)
	}
}
