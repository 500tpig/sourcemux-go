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
	"time"

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

func TestRunPlanTextOutputStillWorks(t *testing.T) {
	out := captureStdout(t, func() {
		if got := Run([]string{"plan", "Evaluate a new open-source project", "--depth", "deep"}); got != 0 {
			t.Fatalf("Run(plan) = %d, want 0", got)
		}
	})
	for _, want := range []string{
		"search_plan",
		"query: Evaluate a new open-source project",
		"depth: deep",
		"web_search query=",
		"final_answer_checklist:",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("plan text missing %q:\n%s", want, out)
		}
	}
}

func TestRunPlanJSONOutputsStructuredPlan(t *testing.T) {
	out := captureStdout(t, func() {
		if got := Run([]string{"plan", "Compare current security risk in SourceMux providers", "--depth", "deep", "--json"}); got != 0 {
			t.Fatalf("Run(plan --json) = %d, want 0", got)
		}
	})
	var parsed tools.StructuredResearchPlan
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, out)
	}
	if parsed.Mode != "deep_research_plan" || parsed.Query == "" || parsed.Depth != "deep" {
		t.Fatalf("plan json metadata = %+v", parsed)
	}
	if parsed.EvidencePolicy != "fetch_before_claim" {
		t.Fatalf("evidence_policy = %q", parsed.EvidencePolicy)
	}
	if parsed.ProfilePolicy.PlannedProfile != "auto" || parsed.ProfilePolicy.EffectiveIfAvailable != "heavy" {
		t.Fatalf("profile_policy = %+v", parsed.ProfilePolicy)
	}
	for _, want := range []string{"deep", "current", "comparison", "high-risk"} {
		if !testContainsString(parsed.ProfilePolicy.HeavyIntentSignals, want) {
			t.Fatalf("heavy intent signals missing %q: %+v", want, parsed.ProfilePolicy.HeavyIntentSignals)
		}
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
	if !strings.Contains(notes, "sourcemux --config") || !strings.Contains(notes, path) {
		t.Fatalf("custom-path setup note missing: %+v", parsed.Notes)
	}
}

func TestConfigListMasksSecrets(t *testing.T) {
	path := writeCLIConfig(t, `{
	  "grokEndpoints": [{"name":"file","baseURL":"https://file.example/v1","apiKey":"sk-super-secret","model":"grok-test","apiType":"responses","profile":"heavy","sendSearchFlag":true,"responseTools":["web_search","x_search"]}],
	  "reasoningEndpoints": [{"name":"deepseek","baseURL":"https://api.deepseek.com/v1","apiKey":"sk-deepseek-secret","model":"deepseek-v4-flash"}],
	  "tavily": {"apiKey": "tvly-secret"},
	  "firecrawl": {"apiKey": "fc-secret"},
	  "exa": {"apiKey": "exa-secret"},
	  "jina": {"apiKey": "jina-secret"},
	  "tinyfish": {"keys": [{"name":"a","apiKey":"tf-secret-a"},{"name":"b","apiKey":"tf-secret-b"}]}
	}`)

	out := captureStdout(t, func() {
		if got := Run([]string{"--config", path, "config", "list", "--json"}); got != 0 {
			t.Fatalf("Run(config list --json) = %d, want 0", got)
		}
	})
	for _, secret := range []string{"sk-super-secret", "sk-deepseek-secret", "tvly-secret", "fc-secret", "exa-secret", "jina-secret", "tf-secret-a", "tf-secret-b"} {
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
	if !parsed.GrokEndpoints[0].Enabled || parsed.GrokEndpoints[0].Profile != "heavy" {
		t.Fatalf("grok endpoint routing fields missing: %+v", parsed.GrokEndpoints[0])
	}
	if len(parsed.ReasoningEndpoints) != 1 || parsed.ReasoningEndpoints[0].KeyStatus == "" {
		t.Fatalf("reasoning endpoint key status missing: %+v", parsed.ReasoningEndpoints)
	}
	if parsed.TavilyKey == "(not set)" || parsed.FirecrawlKey == "(not set)" || len(parsed.TinyFishKeys) != 2 {
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
	for _, want := range []string{`"error"`, `"next_steps"`, "config file not found", "sourcemux --config", path, "setup"} {
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
			"--tinyfish-keys", "tf-setup-a,tf-setup-b",
			"--tinyfish-key-names", "a,b",
			"--json",
		})
		if got != 0 {
			t.Fatalf("Run(setup) = %d, want 0", got)
		}
	})
	for _, secret := range []string{"sk-setup-secret", "tvly-setup-secret", "tf-setup-a", "tf-setup-b"} {
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

func TestRunSearchNoFallbackDisablesProviderFallback(t *testing.T) {
	grok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, `{"error":"try later"}`)
	}))
	defer grok.Close()

	var tinyfishCalls int32
	tinyfish := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&tinyfishCalls, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"results":[{"title":"fallback","url":"https://example.com"}]}}`)
	}))
	defer tinyfish.Close()

	path := writeCLIConfig(t, `{
	  "grokEndpoints": [{"name":"local","baseURL":"`+grok.URL+`","apiKey":"sk-local","model":"grok-test"}],
	  "tinyfish": {"enabled": true, "searchURL": "`+tinyfish.URL+`", "keys": [{"name":"tf","apiKey":"tf-secret"}]}
	}`)

	out := captureStdout(t, func() {
		if got := Run([]string{"--config", path, "search", "q", "--no-fallback", "--json"}); got != 1 {
			t.Fatalf("Run(search --no-fallback) = %d, want 1", got)
		}
	})
	if got := atomic.LoadInt32(&tinyfishCalls); got != 0 {
		t.Fatalf("TinyFish fallback calls = %d, want 0", got)
	}
	var decoded searchOutput
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out)
	}
	if !strings.Contains(decoded.GrokError, "grok API 503") {
		t.Fatalf("grok_error = %q, want upstream failure", decoded.GrokError)
	}
}

func TestRunSearchGrokPoolTimeoutZeroOverrideDisablesConfiguredCap(t *testing.T) {
	grok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(1100 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"slow ok"}}]}`)
	}))
	defer grok.Close()

	path := writeCLIConfig(t, `{
	  "grokPoolTimeoutSec": 1,
	  "grokEndpoints": [{"name":"local","baseURL":"`+grok.URL+`","apiKey":"sk-local","model":"grok-test"}]
	}`)

	out := captureStdout(t, func() {
		if got := Run([]string{
			"--config", path,
			"search", "q",
			"--profile", "default",
			"--grok-pool-timeout", "0",
			"--no-fallback",
			"--timeout", "2s",
			"--json",
		}); got != 0 {
			t.Fatalf("Run(search timeout override) = %d, want 0", got)
		}
	})
	var decoded searchOutput
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out)
	}
	if decoded.Engine != "local" || decoded.Content != "slow ok" {
		t.Fatalf("decoded output = %+v", decoded)
	}
	if decoded.GrokPoolTimeout != "0s" || !decoded.NoFallback {
		t.Fatalf("timeout metadata = pool %q no_fallback %v", decoded.GrokPoolTimeout, decoded.NoFallback)
	}
}

func TestRunSearchUsesPublicSafeDefaultAndPolicyOptInHeavyFirst(t *testing.T) {
	var defaultCalls, heavyCalls int32
	defaultSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&defaultCalls, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"default ok"}}]}`)
	}))
	defer defaultSrv.Close()
	heavySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&heavyCalls, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"heavy ok"}}]}`)
	}))
	defer heavySrv.Close()

	path := writeCLIConfig(t, `{
	  "grokEndpoints": [
	    {"name":"default","baseURL":"`+defaultSrv.URL+`","apiKey":"sk-default","model":"grok-fast"},
	    {"name":"heavy","baseURL":"`+heavySrv.URL+`","apiKey":"sk-heavy","model":"grok-heavy","profile":"heavy"}
	  ]
	}`)

	out := captureStdout(t, func() {
		if got := Run([]string{"--config", path, "search", "quick lookup", "--json"}); got != 0 {
			t.Fatalf("Run(plain search) = %d, want 0", got)
		}
	})
	var plain searchOutput
	if err := json.Unmarshal([]byte(out), &plain); err != nil {
		t.Fatalf("decode plain output: %v\n%s", err, out)
	}
	if plain.Engine != "default" || plain.RequestedProfile != "default" || plain.EffectiveProfile != "default" {
		t.Fatalf("plain output = %+v, want default/default profile", plain)
	}
	if plain.CallerTimeout != "5m0s" || plain.GrokPoolTimeout != "3m0s" {
		t.Fatalf("plain timeouts = caller %q pool %q, want 5m0s/3m0s", plain.CallerTimeout, plain.GrokPoolTimeout)
	}

	out = captureStdout(t, func() {
		if got := Run([]string{"--config", path, "search", "complex current comparison", "--profile", "auto", "--json"}); got != 0 {
			t.Fatalf("Run(auto search) = %d, want 0", got)
		}
	})
	var auto searchOutput
	if err := json.Unmarshal([]byte(out), &auto); err != nil {
		t.Fatalf("decode auto output: %v\n%s", err, out)
	}
	if auto.Engine != "heavy" || auto.RequestedProfile != "auto" || auto.EffectiveProfile != "heavy" {
		t.Fatalf("auto output = %+v, want auto/heavy", auto)
	}
	optInPath := writeCLIConfig(t, `{
	  "searchPolicy": {"defaultProfile":"auto","autoPreference":"heavy-first","fallbackAfterSec":7,"timeoutSec":8},
	  "grokEndpoints": [
	    {"name":"default","baseURL":"`+defaultSrv.URL+`","apiKey":"sk-default","model":"grok-fast"},
	    {"name":"heavy","baseURL":"`+heavySrv.URL+`","apiKey":"sk-heavy","model":"grok-heavy","profile":"heavy"}
	  ]
	}`)
	out = captureStdout(t, func() {
		if got := Run([]string{"--config", optInPath, "search", "quick lookup", "--json"}); got != 0 {
			t.Fatalf("Run(policy opt-in search) = %d, want 0", got)
		}
	})
	var optIn searchOutput
	if err := json.Unmarshal([]byte(out), &optIn); err != nil {
		t.Fatalf("decode opt-in output: %v\n%s", err, out)
	}
	if optIn.Engine != "heavy" || optIn.RequestedProfile != "auto" || optIn.EffectiveProfile != "heavy" {
		t.Fatalf("opt-in output = %+v, want auto/heavy", optIn)
	}
	if optIn.CallerTimeout != "8s" || optIn.GrokPoolTimeout != "7s" {
		t.Fatalf("opt-in timeouts = caller %q pool %q, want 8s/7s", optIn.CallerTimeout, optIn.GrokPoolTimeout)
	}
	if atomic.LoadInt32(&defaultCalls) != 1 || atomic.LoadInt32(&heavyCalls) != 2 {
		t.Fatalf("calls default=%d heavy=%d, want 1/2", defaultCalls, heavyCalls)
	}
}

func TestRunSearchExplicitHeavyErrorsWhenProfileMissing(t *testing.T) {
	path := writeCLIConfig(t, `{
	  "grokEndpoints": [
	    {"name":"default","baseURL":"https://default.example/v1","apiKey":"sk-default","model":"grok-fast"}
	  ],
	  "exa": {"apiURL": "https://exa.example", "apiKey": "exa-test", "enabled": true}
	}`)

	out := captureStdout(t, func() {
		if got := Run([]string{"--config", path, "search", "q", "--profile", "heavy", "--json"}); got != 1 {
			t.Fatalf("Run(search --profile heavy) = %d, want 1", got)
		}
	})
	var decoded searchOutput
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out)
	}
	if !strings.Contains(decoded.GrokError, `profile "heavy"`) {
		t.Fatalf("grok_error = %q, want missing heavy profile", decoded.GrokError)
	}
}

func TestRunSearchAgentOutputIsCompactAndWarnsOnAmbiguousSources(t *testing.T) {
	grok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"Candidate sources: https://apps.apple.com/us/app/cherry-studio/id123 https://www.instagram.com/cherrystudio https://docs.cherry-ai.example/guide"}}]}`)
	}))
	defer grok.Close()

	path := writeCLIConfig(t, `{
	  "grokEndpoints": [{"name":"local","baseURL":"`+grok.URL+`","apiKey":"sk-local","model":"grok-test"}]
	}`)
	out := captureStdout(t, func() {
		if got := Run([]string{"--config", path, "search", "Cherry Studio linux.do", "--agent", "--json"}); got != 0 {
			t.Fatalf("Run(search --agent) = %d, want 0", got)
		}
	})
	var decoded tools.AgentOutput
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out)
	}
	if decoded.Mode != "search" || decoded.AnswerReadiness != "needs_fetch" || len(decoded.SelectedSources) != 3 {
		t.Fatalf("agent search output = %+v", decoded)
	}
	if decoded.SelectedSources[0].ID != "S1" || decoded.SelectedSources[0].URL == "" {
		t.Fatalf("selected sources = %+v", decoded.SelectedSources)
	}
	if strings.Contains(out, `"content":`) {
		t.Fatalf("agent search output should not include full content: %s", out)
	}
	warnings := strings.Join(decoded.Warnings, "\n")
	if !strings.Contains(warnings, "linux.do") || !strings.Contains(warnings, "dispersed") {
		t.Fatalf("warnings = %+v", decoded.Warnings)
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

func TestRunFetchAgentOutputOmitsFullContent(t *testing.T) {
	jina := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("SourceMux agent output fixture. ", 40)))
	}))
	defer jina.Close()

	path := writeCLIConfig(t, `{"jina": {"apiURL": "`+jina.URL+`"}}`)
	out := captureStdout(t, func() {
		if got := Run([]string{"--config", path, "fetch", "https://example.com/page", "--agent", "--json"}); got != 0 {
			t.Fatalf("Run(fetch --agent) = %d, want 0", got)
		}
	})
	var decoded tools.AgentOutput
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out)
	}
	if decoded.Mode != "fetch" || decoded.AnswerReadiness != "ready" || len(decoded.SelectedSources) != 1 {
		t.Fatalf("agent fetch output = %+v", decoded)
	}
	source := decoded.SelectedSources[0]
	if source.ID != "S1" || source.Provider != "Jina Reader" || source.ContentLen == 0 || source.Excerpt == "" {
		t.Fatalf("source = %+v", source)
	}
	if strings.Contains(out, `"content":`) || strings.Contains(out, strings.Repeat("SourceMux agent output fixture. ", 40)) {
		t.Fatalf("agent fetch output should not include full content: %s", out)
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
			"--profile", "heavy",
			"--platform", "GitHub",
			"--domain", "example.com",
			"--domain", "github.com",
			"--max-fetches", "3",
			"--json",
		}, runner); got != 0 {
			t.Fatalf("runResearchWithRunner = %d, want 0", got)
		}
	})

	if runner.opts.Query != "SourceMux MCP" || runner.opts.Depth != "deep" || runner.opts.Profile != "heavy" || runner.opts.Platform != "GitHub" {
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

func TestRunResearchAgentOutputUsesCompressedPack(t *testing.T) {
	runner := &fakeCLIResearchRunner{}
	out := captureStdout(t, func() {
		if got := runResearchWithRunner([]string{
			"SourceMux MCP",
			"--depth", "standard",
			"--max-fetches", "2",
			"--agent",
			"--json",
		}, runner); got != 0 {
			t.Fatalf("runResearchWithRunner --agent = %d, want 0", got)
		}
	})
	var decoded tools.AgentOutput
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out)
	}
	if decoded.Mode != "research" || decoded.AnswerReadiness != "ready_with_gaps" || len(decoded.SelectedSources) != 1 {
		t.Fatalf("agent research output = %+v", decoded)
	}
	if decoded.SelectedSources[0].ID != "S1" || len(decoded.Facts) != 1 || len(decoded.Gaps) != 1 {
		t.Fatalf("agent research details = %+v", decoded)
	}
	if strings.Contains(out, `"confirmed_facts"`) || strings.Contains(out, `"fetched_pages_summary"`) {
		t.Fatalf("agent research output should not include full research pack: %s", out)
	}
}

func TestRunResearchDefaultsProfileAuto(t *testing.T) {
	runner := &fakeCLIResearchRunner{}
	_ = captureStdout(t, func() {
		if got := runResearchWithRunner([]string{
			"SourceMux MCP",
			"--json",
		}, runner); got != 0 {
			t.Fatalf("runResearchWithRunner = %d, want 0", got)
		}
	})
	if runner.opts.Profile != "auto" {
		t.Fatalf("default research profile = %q, want auto", runner.opts.Profile)
	}
	if runner.timeout < 299*time.Second || runner.timeout > 300*time.Second {
		t.Fatalf("default research timeout = %s, want about 300s", runner.timeout)
	}
}

func TestRunSmartAnswerJSONParsesParameters(t *testing.T) {
	runner := &fakeCLISmartAnswerRunner{}
	out := captureStdout(t, func() {
		if got := runSmartAnswerWithRunner([]string{
			"Should I use DeepSeek?",
			"--depth", "quick",
			"--profile", "heavy",
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
	if runner.opts.Profile != "heavy" {
		t.Fatalf("profile opts = %+v", runner.opts)
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

func TestRunSmartAnswerDefaultsProfileAuto(t *testing.T) {
	runner := &fakeCLISmartAnswerRunner{}
	_ = captureStdout(t, func() {
		if got := runSmartAnswerWithRunner([]string{
			"Should I use DeepSeek?",
			"--json",
		}, runner); got != 0 {
			t.Fatalf("runSmartAnswerWithRunner = %d, want 0", got)
		}
	})
	if runner.opts.Profile != "auto" {
		t.Fatalf("default smart-answer profile = %q, want auto", runner.opts.Profile)
	}
	if runner.timeout < 359*time.Second || runner.timeout > 360*time.Second {
		t.Fatalf("default smart-answer timeout = %s, want about 360s", runner.timeout)
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

func TestRunDocsSearchJSONUsesExaConfig(t *testing.T) {
	exa := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" {
			t.Fatalf("unexpected exa path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"requestId":"req","searchType":"auto","results":[{"title":"Docs","url":"https://example.com/docs","highlights":["docs"]}]}`)
	}))
	defer exa.Close()

	path := writeCLIConfig(t, `{
	  "exa": {"apiURL": "`+exa.URL+`", "apiKey": "exa-test", "enabled": true}
	}`)
	out := captureStdout(t, func() {
		if got := Run([]string{"--config", path, "docs-search", "general docs", "--json"}); got != 0 {
			t.Fatalf("Run(docs-search) = %d, want 0", got)
		}
	})
	var decoded docsSearchOutput
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

func TestRunFirecrawlScrapeJSONUsesConfig(t *testing.T) {
	var gotBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/scrape" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer fc-test" {
			t.Fatalf("Authorization = %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"markdown":"# Firecrawl\ncontent","links":["https://example.com/a"],"metadata":{"title":"Firecrawl","sourceURL":"https://example.com/final"}}}`))
	}))
	defer ts.Close()

	path := writeCLIConfig(t, `{
	  "firecrawl": {"apiURL": "`+ts.URL+`", "apiKey": "fc-test", "enabled": true}
	}`)

	out := captureStdout(t, func() {
		if got := Run([]string{
			"--config", path,
			"firecrawl-scrape", "https://example.com",
			"--format", "markdown,links",
			"--include-tags", "article,main",
			"--exclude-tags", "nav,footer",
			"--wait-for", "250",
			"--proxy", "auto",
			"--json",
		}); got != 0 {
			t.Fatalf("Run(firecrawl-scrape) = %d, want 0", got)
		}
	})
	var decoded firecrawlScrapeOutput
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out)
	}
	if decoded.Source != "Firecrawl Scrape" || decoded.ResultURL != "https://example.com/final" || !strings.Contains(decoded.Content, "content") {
		t.Fatalf("decoded = %+v", decoded)
	}
	if len(decoded.RouteDecision) != 1 || decoded.RouteDecision[0].Provider != "firecrawl-scrape" {
		t.Fatalf("route_decision = %+v", decoded.RouteDecision)
	}
	if gotBody["url"] != "https://example.com" || gotBody["waitFor"] != float64(250) || gotBody["proxy"] != "auto" {
		t.Fatalf("request body = %#v", gotBody)
	}
	formats, ok := gotBody["formats"].([]any)
	if !ok || strings.Join([]string{formats[0].(string), formats[1].(string)}, ",") != "markdown,links" {
		t.Fatalf("formats = %#v", gotBody["formats"])
	}
	includeTags, ok := gotBody["includeTags"].([]any)
	if !ok || len(includeTags) != 2 || includeTags[0] != "article" || includeTags[1] != "main" {
		t.Fatalf("includeTags = %#v", gotBody["includeTags"])
	}
	excludeTags, ok := gotBody["excludeTags"].([]any)
	if !ok || len(excludeTags) != 2 || excludeTags[0] != "nav" || excludeTags[1] != "footer" {
		t.Fatalf("excludeTags = %#v", gotBody["excludeTags"])
	}
}

func TestRunFirecrawlMapJSONUsesConfig(t *testing.T) {
	var gotBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/map" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer fc-test" {
			t.Fatalf("Authorization = %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"links":[{"url":"https://example.com/docs","title":"Docs"},{"url":"https://example.com/blog","description":"Blog"}]}`))
	}))
	defer ts.Close()

	path := writeCLIConfig(t, `{
	  "firecrawl": {"apiURL": "`+ts.URL+`", "apiKey": "fc-test", "enabled": true}
	}`)

	out := captureStdout(t, func() {
		if got := Run([]string{
			"--config", path,
			"firecrawl-map", "https://example.com",
			"--search", "docs",
			"--limit", "25",
			"--sitemap", "only",
			"--firecrawl-timeout", "60000",
			"--json",
		}); got != 0 {
			t.Fatalf("Run(firecrawl-map) = %d, want 0", got)
		}
	})
	var decoded firecrawlMapOutput
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out)
	}
	if decoded.Source != "Firecrawl Map" || decoded.Count != 2 || decoded.URLs[0] != "https://example.com/docs" {
		t.Fatalf("decoded = %+v", decoded)
	}
	if len(decoded.RouteDecision) != 1 || decoded.RouteDecision[0].Provider != "firecrawl-map" {
		t.Fatalf("route_decision = %+v", decoded.RouteDecision)
	}
	if gotBody["search"] != "docs" || gotBody["limit"] != float64(25) || gotBody["sitemap"] != "only" || gotBody["timeout"] != float64(60000) {
		t.Fatalf("request body = %#v", gotBody)
	}
}

func TestRunFirecrawlUnavailableDoesNotCallNetwork(t *testing.T) {
	var calls int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	for _, tc := range []struct {
		name string
		body string
		args []string
	}{
		{
			name: "missing key scrape",
			body: `{"firecrawl":{"apiURL":"` + ts.URL + `","enabled":true}}`,
			args: []string{"firecrawl-scrape", "https://example.com", "--json"},
		},
		{
			name: "disabled map",
			body: `{"firecrawl":{"apiURL":"` + ts.URL + `","apiKey":"fc-test","enabled":false}}`,
			args: []string{"firecrawl-map", "https://example.com", "--json"},
		},
		{
			name: "key without explicit enabled scrape",
			body: `{"firecrawl":{"apiURL":"` + ts.URL + `","apiKey":"fc-test"}}`,
			args: []string{"firecrawl-scrape", "https://example.com", "--json"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := writeCLIConfig(t, tc.body)
			out := captureStdout(t, func() {
				args := append([]string{"--config", path}, tc.args...)
				if got := Run(args); got != 1 {
					t.Fatalf("Run(%v) = %d, want 1", args, got)
				}
			})
			if !strings.Contains(out, "Firecrawl is not configured") {
				t.Fatalf("output = %s", out)
			}
		})
	}
	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Fatalf("network calls = %d, want 0", got)
	}
}

func TestRunFirecrawlScrapeErrorCases(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{name: "401", status: http.StatusUnauthorized, body: `bad key`, want: "401"},
		{name: "500", status: http.StatusInternalServerError, body: `upstream down`, want: "500"},
		{name: "empty markdown", status: http.StatusOK, body: `{"success":true,"data":{"markdown":" "}}`, want: "empty markdown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer ts.Close()

			path := writeCLIConfig(t, `{"firecrawl":{"apiURL":"`+ts.URL+`","apiKey":"fc-test","enabled":true}}`)
			out := captureStdout(t, func() {
				if got := Run([]string{"--config", path, "firecrawl-scrape", "https://example.com", "--json"}); got != 1 {
					t.Fatalf("Run(firecrawl-scrape error) = %d, want 1", got)
				}
			})
			if !strings.Contains(out, tt.want) {
				t.Fatalf("output missing %q: %s", tt.want, out)
			}
			if !strings.Contains(out, "route_decision") {
				t.Fatalf("error output missing route_decision: %s", out)
			}
		})
	}
}

func TestRunFetchDefaultUsesFirecrawlWhenConfiguredAndCheapUsesJina(t *testing.T) {
	jina := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/markdown")
		_, _ = w.Write([]byte("# Jina\nunchanged fetch route"))
	}))
	defer jina.Close()

	tavily := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/map" {
			t.Fatalf("unexpected tavily path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":["https://example.com/tavily"]}`))
	}))
	defer tavily.Close()

	var firecrawlCalls int32
	firecrawl := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&firecrawlCalls, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"markdown":"firecrawl"}}`))
	}))
	defer firecrawl.Close()

	path := writeCLIConfig(t, `{
	  "jina": {"apiURL": "`+jina.URL+`"},
	  "tavily": {"apiURL": "`+tavily.URL+`", "apiKey": "tvly-test", "enabled": true},
	  "firecrawl": {"apiURL": "`+firecrawl.URL+`", "apiKey": "fc-test", "enabled": true}
	}`)

	fetchOut := captureStdout(t, func() {
		if got := Run([]string{"--config", path, "fetch", "https://example.com", "--json"}); got != 0 {
			t.Fatalf("Run(fetch) = %d, want 0", got)
		}
	})
	var fetchDecoded fetchOutput
	if err := json.Unmarshal([]byte(fetchOut), &fetchDecoded); err != nil {
		t.Fatalf("decode fetch: %v\n%s", err, fetchOut)
	}
	if !strings.HasPrefix(fetchDecoded.Source, "Firecrawl Scrape") || fetchDecoded.Content != "firecrawl" {
		t.Fatalf("fetch decoded = %+v", fetchDecoded)
	}
	if fetchDecoded.Policy.EffectiveProfile != "auto" || fetchDecoded.Policy.Intent != "ordinary" {
		t.Fatalf("fetch policy = %+v", fetchDecoded.Policy)
	}

	cheapOut := captureStdout(t, func() {
		if got := Run([]string{"--config", path, "fetch", "https://example.com", "--profile", "cheap", "--json"}); got != 0 {
			t.Fatalf("Run(fetch --profile cheap) = %d, want 0", got)
		}
	})
	var cheapDecoded fetchOutput
	if err := json.Unmarshal([]byte(cheapOut), &cheapDecoded); err != nil {
		t.Fatalf("decode cheap fetch: %v\n%s", err, cheapOut)
	}
	if cheapDecoded.Source != "Jina Reader" || !strings.Contains(cheapDecoded.Content, "unchanged fetch route") {
		t.Fatalf("cheap fetch decoded = %+v", cheapDecoded)
	}
	if cheapDecoded.Policy.EffectiveProfile != "cheap" || len(cheapDecoded.RouteDecision) == 0 || cheapDecoded.RouteDecision[0].Provider != "jina-reader" {
		t.Fatalf("cheap route = policy=%+v decisions=%+v", cheapDecoded.Policy, cheapDecoded.RouteDecision)
	}

	mapOut := captureStdout(t, func() {
		if got := Run([]string{"--config", path, "map", "https://example.com", "--json"}); got != 0 {
			t.Fatalf("Run(map) = %d, want 0", got)
		}
	})
	var mapDecoded mapOutput
	if err := json.Unmarshal([]byte(mapOut), &mapDecoded); err != nil {
		t.Fatalf("decode map: %v\n%s", err, mapOut)
	}
	if mapDecoded.URLs[0] != "https://example.com/tavily" {
		t.Fatalf("map decoded = %+v", mapDecoded)
	}
	if got := atomic.LoadInt32(&firecrawlCalls); got != 1 {
		t.Fatalf("firecrawl calls = %d, want 1", got)
	}
}

func TestRunFetchUsesFirecrawlFirstWhenV2WebFetchOrdersItFirst(t *testing.T) {
	var firecrawlCalls int32
	firecrawl := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&firecrawlCalls, 1)
		if r.URL.Path != "/scrape" {
			t.Fatalf("unexpected firecrawl path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer fc-primary" && got != "Bearer fc-backup" {
			t.Fatalf("Authorization = %q", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode firecrawl body: %v", err)
		}
		if body["onlyCleanContent"] != false {
			t.Fatalf("onlyCleanContent = %#v", body["onlyCleanContent"])
		}
		if body["timeout"] != float64(15000) {
			t.Fatalf("timeout = %#v", body["timeout"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"markdown":"clean firecrawl content","metadata":{"sourceURL":"https://example.com/final"}}}`))
	}))
	defer firecrawl.Close()

	var jinaCalls int32
	jina := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&jinaCalls, 1)
		w.Header().Set("Content-Type", "text/markdown")
		_, _ = w.Write([]byte("jina content"))
	}))
	defer jina.Close()

	path := writeCLIConfig(t, `{
	  "version": 2,
	  "minimum_profile": "off",
	  "capabilities": {
	    "main_search": {"providers": []},
	    "docs_search": {"providers": []},
	    "web_fetch": {
	      "providers": [
	        {
	          "type": "firecrawl",
	          "apiURL": "`+firecrawl.URL+`",
	          "keys": [{"name":"primary","apiKey":"fc-primary"},{"name":"backup","apiKey":"fc-backup"}],
	          "enabled": true
	        },
	        {"type": "jina", "apiURL": "`+jina.URL+`"}
	      ]
	    },
	    "web_enhance": {"providers": []}
	  }
	}`)

	out := captureStdout(t, func() {
		if got := Run([]string{"--config", path, "fetch", "https://example.com", "--json"}); got != 0 {
			t.Fatalf("Run(fetch) = %d, want 0", got)
		}
	})
	var decoded fetchOutput
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out)
	}
	if !strings.HasPrefix(decoded.Source, "Firecrawl Scrape") || decoded.URL != "https://example.com/final" || decoded.Content != "clean firecrawl content" {
		t.Fatalf("decoded = %+v", decoded)
	}
	if len(decoded.RouteDecision) != 1 || decoded.RouteDecision[0].Provider != "firecrawl-scrape" {
		t.Fatalf("route_decision = %+v", decoded.RouteDecision)
	}
	if decoded.RouteDecision[0].SubAttempts != 1 || len(decoded.RouteDecision[0].SubAttemptDetails) != 1 {
		t.Fatalf("sub-attempts = %+v", decoded.RouteDecision[0])
	}
	if detail := decoded.RouteDecision[0].SubAttemptDetails[0]; detail.Name != "primary" || detail.Status != "ok" {
		t.Fatalf("sub-attempt detail = %+v", detail)
	}
	if atomic.LoadInt32(&firecrawlCalls) != 1 || atomic.LoadInt32(&jinaCalls) != 0 {
		t.Fatalf("calls firecrawl=%d jina=%d", firecrawlCalls, jinaCalls)
	}
}

func TestRunFetchQualityProfileEnablesFirecrawlCleanContent(t *testing.T) {
	var gotBody map[string]any
	firecrawl := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode firecrawl body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"markdown":"quality firecrawl content"}}`))
	}))
	defer firecrawl.Close()

	path := writeCLIConfig(t, `{
	  "firecrawl": {"apiURL": "`+firecrawl.URL+`", "apiKey": "fc-test", "enabled": true}
	}`)

	out := captureStdout(t, func() {
		if got := Run([]string{"--config", path, "fetch", "https://docs.example.com", "--profile", "quality", "--json"}); got != 0 {
			t.Fatalf("Run(fetch --profile quality) = %d, want 0", got)
		}
	})
	var decoded fetchOutput
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out)
	}
	if decoded.Policy.EffectiveProfile != "quality" || !strings.HasPrefix(decoded.Source, "Firecrawl Scrape") {
		t.Fatalf("decoded = %+v", decoded)
	}
	if gotBody["onlyCleanContent"] != true {
		t.Fatalf("onlyCleanContent = %#v", gotBody["onlyCleanContent"])
	}
	if gotBody["timeout"] != float64(120000) {
		t.Fatalf("timeout = %#v", gotBody["timeout"])
	}
}

func TestDefaultCallerTimeoutForFetchProfile(t *testing.T) {
	if got := tools.DefaultCallerTimeoutForFetchProfile(tools.FetchProfileAuto); got != 60*time.Second {
		t.Fatalf("auto timeout = %s", got)
	}
	if got := tools.DefaultCallerTimeoutForFetchProfile(tools.FetchProfileCheap); got != 60*time.Second {
		t.Fatalf("cheap timeout = %s", got)
	}
	if got := tools.DefaultCallerTimeoutForFetchProfile(tools.FetchProfileQuality); got != 300*time.Second {
		t.Fatalf("quality timeout = %s", got)
	}
}

func TestRunFetchFallsBackFromFirecrawlToJinaInV2Order(t *testing.T) {
	firecrawl := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("bad key fc-primary"))
	}))
	defer firecrawl.Close()

	jina := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/markdown")
		_, _ = w.Write([]byte("jina fallback content"))
	}))
	defer jina.Close()

	path := writeCLIConfig(t, `{
	  "version": 2,
	  "minimum_profile": "off",
	  "capabilities": {
	    "main_search": {"providers": []},
	    "docs_search": {"providers": []},
	    "web_fetch": {
	      "providers": [
	        {"type": "firecrawl", "apiURL": "`+firecrawl.URL+`", "apiKey": "fc-primary", "enabled": true},
	        {"type": "jina", "apiURL": "`+jina.URL+`"}
	      ]
	    },
	    "web_enhance": {"providers": []}
	  }
	}`)

	out := captureStdout(t, func() {
		if got := Run([]string{"--config", path, "fetch", "https://example.com", "--json"}); got != 0 {
			t.Fatalf("Run(fetch) = %d, want 0", got)
		}
	})
	var decoded fetchOutput
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out)
	}
	if decoded.Source != "Jina Reader" || decoded.Content != "jina fallback content" {
		t.Fatalf("decoded = %+v", decoded)
	}
	if len(decoded.RouteDecision) != 2 || decoded.RouteDecision[0].Provider != "firecrawl-scrape" || decoded.RouteDecision[1].Provider != "jina-reader" {
		t.Fatalf("route_decision = %+v", decoded.RouteDecision)
	}
	if strings.Contains(out, "fc-primary") {
		t.Fatalf("output leaked firecrawl key: %s", out)
	}
}

type fakeCLIResearchRunner struct {
	opts    tools.ResearchOptions
	timeout time.Duration
}

func (r *fakeCLIResearchRunner) Run(ctx context.Context, opts tools.ResearchOptions) (tools.ResearchPack, error) {
	r.opts = opts
	if deadline, ok := ctx.Deadline(); ok {
		r.timeout = time.Until(deadline)
	}
	return tools.ResearchPack{
		Query:            opts.Query,
		EffectiveDepth:   opts.Depth,
		RequestedProfile: opts.Profile,
		EffectiveProfile: opts.Profile,
		Platform:         opts.Platform,
		Domains:          opts.Domains,
		MaxFetches:       opts.MaxFetches,
		PlanQueries:      []string{opts.Query},
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
	opts    tools.SmartAnswerOptions
	timeout time.Duration
}

func (r *fakeCLISmartAnswerRunner) Run(ctx context.Context, opts tools.SmartAnswerOptions) (tools.SmartAnswerResult, error) {
	r.opts = opts
	if deadline, ok := ctx.Deadline(); ok {
		r.timeout = time.Until(deadline)
	}
	return tools.SmartAnswerResult{
		Query:             opts.Query,
		Answer:            "Use Grok for search and DeepSeek for synthesis.",
		ReasoningEndpoint: opts.ReasoningEndpoint,
		ReasoningModel:    opts.ReasoningModel,
		Research: tools.ResearchPack{
			Query:            opts.Query,
			EffectiveDepth:   opts.Depth,
			RequestedProfile: opts.Profile,
			EffectiveProfile: opts.Profile,
			MaxFetches:       opts.MaxFetches,
			SourceSummary:    tools.ResearchSourceSummary{UniqueURLs: 1},
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

func testContainsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
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
