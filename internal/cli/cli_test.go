package cli

import (
	"encoding/json"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
