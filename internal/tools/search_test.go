package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/500tpig/sourcemux-go/internal/config"
	"github.com/500tpig/sourcemux-go/internal/engine"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

type testSourceCache struct {
	sessionID string
	urls      []string
}

func (c *testSourceCache) CacheSources(sessionID string, urls []string) {
	c.sessionID = sessionID
	c.urls = urls
}

func TestFormatTinyFishResponseCachesSources(t *testing.T) {
	cache := &testSourceCache{}
	res := webSearchTinyFishResult("query", &engine.TinyFishPoolSearchResult{
		KeyName: "acct-a",
		TinyFishSearchResponse: &engine.TinyFishSearchResponse{
			Results: []engine.TinyFishSearchResult{
				{Title: "A", URL: "https://example.com/a"},
				{Title: "B", URL: "https://example.com/b"},
			},
		},
	}, nil, cache)
	body := FormatWebSearchResult(res)

	if !strings.Contains(body, "engine: TinyFish Search (acct-a; no Grok endpoint configured)") {
		t.Fatalf("body = %s", body)
	}
	if cache.sessionID == "" {
		t.Fatal("session id was not cached")
	}
	if strings.Join(cache.urls, ",") != "https://example.com/a,https://example.com/b" {
		t.Fatalf("cached urls = %v", cache.urls)
	}
}

func TestFormatWebSearchResultIsCompactForMCP(t *testing.T) {
	content := strings.Repeat("A", mcpSearchExcerptRunes) + "TAIL"
	body := FormatWebSearchResult(&WebSearchResult{
		Engine:       "grok-prod",
		Model:        "grok-4.20-fast",
		SessionID:    "session-123",
		Content:      content,
		SourcesCount: 2,
	})

	for _, want := range []string{
		"engine: grok-prod (grok-4.20-fast)",
		"session_id: session-123",
		"sources: call get_sources(session_id) for URLs",
		"content_chars:",
		"summary:",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "TAIL") {
		t.Fatalf("expected clipped MCP output, got tail in:\n%s", body)
	}
}

func TestInjectTimeContextDoesNotDuplicateExplicitPrefix(t *testing.T) {
	query := "[Current date: 2026-08-10] What is the latest release?"
	if got := injectTimeContext(query); got != query {
		t.Fatalf("injectTimeContext = %q, want unchanged query", got)
	}

	got := injectTimeContext("What is the latest release?")
	if !strings.HasPrefix(got, "[Current time:") {
		t.Fatalf("injectTimeContext = %q, want generated time prefix", got)
	}
}

func TestBuildSearchAgentOutputWarnsForRegexOnlySources(t *testing.T) {
	out := BuildSearchAgentOutput("latest release", &WebSearchResult{
		SourceURLs:   []string{"https://example.com/release"},
		SourceOrigin: engine.SourceOriginRegex,
	})
	if len(out.Warnings) == 0 || !strings.Contains(out.Warnings[0], "generated answer text") {
		t.Fatalf("warnings = %v", out.Warnings)
	}
}

func TestBuildSearchAgentOutputWarnsForUnverifiedAnswerCheck(t *testing.T) {
	for _, tc := range []struct {
		reason string
		want   string
	}{
		{reason: "jev_unconfigured", want: "without a configured JEV key"},
		{reason: "jev_error", want: "check failed"},
	} {
		out := BuildSearchAgentOutput("q", &WebSearchResult{
			SourceURLs:  []string{"https://example.com/a"},
			AnswerCheck: &SearchAnswerCheck{Status: "unverified", Reason: tc.reason},
		})
		joined := strings.Join(out.Warnings, "\n")
		if !strings.Contains(joined, tc.want) || !strings.Contains(joined, "unverified") {
			t.Fatalf("warnings for %s = %+v", tc.reason, out.Warnings)
		}
	}

	passed := BuildSearchAgentOutput("q", &WebSearchResult{
		SourceURLs:  []string{"https://example.com/a"},
		AnswerCheck: &SearchAnswerCheck{Status: "passed"},
	})
	if strings.Contains(strings.Join(passed.Warnings, "\n"), "unverified") {
		t.Fatalf("unexpected unverified warning: %+v", passed.Warnings)
	}
}

func TestMCPWebSearchAnswerCheckOverridesAndFallbackFormatting(t *testing.T) {
	grokSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices":[{"message":{"content":"There is no release yet.","annotations":[{"type":"url_citation","url_citation":{"url":"https://example.com/release","title":"Release 2.0 Published"}}]}}],
			"citations":["https://example.com/release"]
		}`))
	}))
	defer grokSrv.Close()

	var tavilyCalls atomic.Int32
	tavilySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tavilyCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"answer":"Release 2.0 is out.","results":[{"title":"Release 2.0 Published","url":"https://example.com/release","content":"Release notes","score":0.9}]}`))
	}))
	defer tavilySrv.Close()

	var jevCalls atomic.Int32
	var rejectNext atomic.Bool
	jevSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jevCalls.Add(1)
		contradicted := 0.05
		if rejectNext.Load() {
			contradicted = 0.9
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"answers": map[string]any{
				"substantive":  map[string]any{"type": "noul", "noul": 0.9},
				"contradicted": map[string]any{"type": "noul", "noul": contradicted},
			},
		})
	}))
	defer jevSrv.Close()

	pool := engine.NewGrokPool([]engine.GrokEndpoint{{Name: "primary", BaseURL: grokSrv.URL, APIKey: "sk-g", Model: "grok-test"}})
	tavily := engine.NewTavilyClient(tavilySrv.URL, "tvly-test")
	jev := engine.NewJEVClient(engine.JEVConfig{APIURL: jevSrv.URL, APIKey: "jev-test"})

	t.Run("default_false_and_explicit_true_override", func(t *testing.T) {
		jevCalls.Store(0)
		tavilyCalls.Store(0)
		rejectNext.Store(true)
		s := mcpserver.NewMCPServer("test", "1")
		RegisterSearchWithClients(s, WebSearchClients{
			Pool:         pool,
			Tavily:       tavily,
			JEV:          jev,
			AnswerCheck:  false,
			SearchPolicy: config.DefaultSearchPolicy(),
		})

		resp := s.HandleMessage(context.Background(), json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"web_search","arguments":{"query":"latest release"}}}`))
		encoded, _ := json.Marshal(resp)
		if jevCalls.Load() != 0 || tavilyCalls.Load() != 0 || strings.Contains(string(encoded), "grok_answer_check") {
			t.Fatalf("expected default answerCheck=false to skip JEV, got jev=%d tavily=%d body=%s", jevCalls.Load(), tavilyCalls.Load(), encoded)
		}

		resp = s.HandleMessage(context.Background(), json.RawMessage(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"web_search","arguments":{"query":"latest release","answer_check":true}}}`))
		encoded, _ = json.Marshal(resp)
		if jevCalls.Load() != 1 || tavilyCalls.Load() != 1 {
			t.Fatalf("expected explicit answer_check=true to trigger JEV and fallback, got jev=%d tavily=%d", jevCalls.Load(), tavilyCalls.Load())
		}
		if !strings.Contains(string(encoded), "grok_answer_check: rejected (contradicted_by_sources; switched to fallback tavily)") {
			t.Fatalf("missing clear fallback wording in MCP output: %s", encoded)
		}
	})

	t.Run("config_true_and_explicit_false_override", func(t *testing.T) {
		jevCalls.Store(0)
		tavilyCalls.Store(0)
		rejectNext.Store(false)
		s := mcpserver.NewMCPServer("test", "1")
		RegisterSearchWithClients(s, WebSearchClients{
			Pool:         pool,
			Tavily:       tavily,
			JEV:          jev,
			AnswerCheck:  true,
			SearchPolicy: config.DefaultSearchPolicy(),
		})

		resp := s.HandleMessage(context.Background(), json.RawMessage(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"web_search","arguments":{"query":"latest release"}}}`))
		encoded, _ := json.Marshal(resp)
		if jevCalls.Load() != 1 || !strings.Contains(string(encoded), "grok_answer_check: passed") {
			t.Fatalf("expected configured answerCheck=true to pass, jev=%d body=%s", jevCalls.Load(), encoded)
		}

		resp = s.HandleMessage(context.Background(), json.RawMessage(`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"web_search","arguments":{"query":"latest release","answer_check":false}}}`))
		encoded, _ = json.Marshal(resp)
		if jevCalls.Load() != 1 || strings.Contains(string(encoded), "grok_answer_check") {
			t.Fatalf("expected explicit answer_check=false to override config, jev=%d body=%s", jevCalls.Load(), encoded)
		}
	})
}

func TestMCPWebSearchRejectsNonBooleanAnswerCheckWithoutNetworkCalls(t *testing.T) {
	var grokCalls, jevCalls, tavilyCalls atomic.Int32
	grokSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		grokCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Answer"}}],"citations":["https://example.com"]}`))
	}))
	defer grokSrv.Close()

	tavilySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tavilyCalls.Add(1)
	}))
	defer tavilySrv.Close()

	jevSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jevCalls.Add(1)
	}))
	defer jevSrv.Close()

	s := mcpserver.NewMCPServer("test", "1")
	RegisterSearchWithClients(s, WebSearchClients{
		Pool:         engine.NewGrokPool([]engine.GrokEndpoint{{Name: "primary", BaseURL: grokSrv.URL, APIKey: "sk-g", Model: "grok-test"}}),
		Tavily:       engine.NewTavilyClient(tavilySrv.URL, "tvly-test"),
		JEV:          engine.NewJEVClient(engine.JEVConfig{APIURL: jevSrv.URL, APIKey: "jev-test", AnswerCheck: true}),
		AnswerCheck:  true,
		SearchPolicy: config.DefaultSearchPolicy(),
	})

	for _, rawVal := range []string{`"false"`, `"true"`, `1`, `null`} {
		req := json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"web_search","arguments":{"query":"latest release","answer_check":` + rawVal + `}}}`)
		resp := s.HandleMessage(context.Background(), req)
		encoded, _ := json.Marshal(resp)
		if !strings.Contains(string(encoded), `"isError":true`) || !strings.Contains(string(encoded), "answer_check must be a boolean") {
			t.Fatalf("expected boolean type error for %s, got %s", rawVal, encoded)
		}
	}
	if grokCalls.Load() != 0 || jevCalls.Load() != 0 || tavilyCalls.Load() != 0 {
		t.Fatalf("expected zero network calls, got grok=%d jev=%d tavily=%d", grokCalls.Load(), jevCalls.Load(), tavilyCalls.Load())
	}
}

func TestRunWebSearchPreservesPreCanceledAndTimedOutReasonWithoutCallingProviders(t *testing.T) {
	var grokCalls, tavilyCalls atomic.Int32
	grokSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		grokCalls.Add(1)
	}))
	defer grokSrv.Close()

	tavilySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tavilyCalls.Add(1)
	}))
	defer tavilySrv.Close()

	clients := WebSearchClients{
		Pool:         engine.NewGrokPool([]engine.GrokEndpoint{{Name: "primary", BaseURL: grokSrv.URL, APIKey: "sk-g", Model: "grok-test"}}),
		Tavily:       engine.NewTavilyClient(tavilySrv.URL, "tvly-test"),
		SearchPolicy: config.DefaultSearchPolicy(),
	}

	t.Run("pre_canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		res, err := RunWebSearchWithOptions(ctx, clients, WebSearchOptions{Query: "q"})
		if err == nil || !strings.Contains(err.Error(), "context canceled") || strings.Contains(err.Error(), "no fallback configured") {
			t.Fatalf("err = %v, want context canceled", err)
		}
		if res == nil || res.RouteTrace.Error != "context canceled" {
			t.Fatalf("trace = %+v", res)
		}
	})

	t.Run("pre_timed_out", func(t *testing.T) {
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		defer cancel()

		res, err := RunWebSearchWithOptions(ctx, clients, WebSearchOptions{Query: "q"})
		if err == nil || !strings.Contains(err.Error(), "context deadline exceeded") || strings.Contains(err.Error(), "no fallback configured") {
			t.Fatalf("err = %v, want context deadline exceeded", err)
		}
		if res == nil || res.RouteTrace.Error != "context deadline exceeded" {
			t.Fatalf("trace = %+v", res)
		}
	})

	if grokCalls.Load() != 0 || tavilyCalls.Load() != 0 {
		t.Fatalf("expected 0 provider calls, got grok=%d tavily=%d", grokCalls.Load(), tavilyCalls.Load())
	}
}
