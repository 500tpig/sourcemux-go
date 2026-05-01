package cli

import (
	"encoding/json"
	"flag"
	"io"
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
