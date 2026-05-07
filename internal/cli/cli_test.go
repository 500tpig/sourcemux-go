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
	"testing"

	"github.com/bettas/grok-search-go/internal/engine"
	"github.com/bettas/grok-search-go/internal/tools"
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
			"Grok Search MCP",
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

	if runner.opts.Query != "Grok Search MCP" || runner.opts.Depth != "deep" || runner.opts.Platform != "GitHub" {
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
	if decoded.Query != "Grok Search MCP" || decoded.EffectiveDepth != "deep" || decoded.Platform != "GitHub" {
		t.Fatalf("decoded metadata = %+v", decoded)
	}
	if decoded.SourceSummary.SelectedForFetch != 3 {
		t.Fatalf("source summary = %+v", decoded.SourceSummary)
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

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("GROK_ENDPOINTS_JSON", `[{"name":"test","baseURL":"https://grok.invalid/v1","apiKey":"sk-test","model":"grok-test"}]`)
	t.Setenv("TAVILY_API_URL", ts.URL)
	t.Setenv("TAVILY_API_KEY", "tvly-test")
	t.Setenv("TAVILY_ENABLED", "true")

	out := captureStdout(t, func() {
		if got := Run([]string{"crawl", "https://example.com", "--instructions", "Find docs", "--limit", "1", "--json"}); got != 0 {
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

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("GROK_ENDPOINTS_JSON", `[{"name":"test","baseURL":"https://grok.invalid/v1","apiKey":"sk-test","model":"grok-test"}]`)
	t.Setenv("EXA_API_URL", ts.URL)
	t.Setenv("EXA_API_KEY", "exa-test")
	t.Setenv("EXA_ENABLED", "true")

	out := captureStdout(t, func() {
		if got := Run([]string{
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

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("GROK_ENDPOINTS_JSON", `[{"name":"test","baseURL":"https://grok.invalid/v1","apiKey":"sk-test","model":"grok-test"}]`)
	t.Setenv("EXA_API_URL", ts.URL)
	t.Setenv("EXA_API_KEY", "exa-test")
	t.Setenv("EXA_ENABLED", "true")

	out := captureStdout(t, func() {
		if got := Run([]string{
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
			Excerpt: "Grok Search MCP is test content.",
		}},
		HighSignalSources: []tools.ResearchSource{{
			URL:         "https://example.com/a",
			Domain:      "example.com",
			Score:       1,
			Occurrences: 1,
		}},
		ConfirmedFacts:   []string{"Grok Search MCP is test content. (source: https://example.com/a)"},
		LikelyInferences: []string{"fake inference"},
		OpenQuestions:    []string{"fake question"},
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
