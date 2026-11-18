package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/500tpig/sourcemux-go/internal/capability"
	"github.com/500tpig/sourcemux-go/internal/engine"
	"github.com/500tpig/sourcemux-go/internal/router"
)

func testGrokPool(t *testing.T, body string) *engine.GrokPool {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return engine.NewGrokPool([]engine.GrokEndpoint{{
		Name:    "primary",
		BaseURL: srv.URL,
		APIKey:  "sk-test",
		Model:   "grok-test",
	}})
}

func testTavilyClient(t *testing.T, calls *atomic.Int32, answer string) *engine.TavilyClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls != nil {
			calls.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"answer": answer,
			"results": []map[string]any{
				{"title": "Fallback source", "url": "https://fallback.example/doc", "content": "verified", "score": 0.9},
			},
		})
	}))
	t.Cleanup(srv.Close)
	return engine.NewTavilyClient(srv.URL, "tvly-test")
}

func TestGrokSearchProvider_AnswerCheckDisabledDoesNotCallJEV(t *testing.T) {
	var jevCalls atomic.Int32
	jevSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jevCalls.Add(1)
	}))
	defer jevSrv.Close()

	pool := testGrokPool(t, `{"choices":[{"message":{"content":"Substantive answer"}}],"citations":["https://example.com/a"]}`)
	provider := NewGrokSearchWithAnswerCheck(pool, engine.NewJEVClient(engine.JEVConfig{APIURL: jevSrv.URL, APIKey: "jev-test"}), false)

	r := router.New(provider)
	res, trace := r.Run(context.Background(), capability.MainSearch, capability.Request{Query: "what is Go?"})
	if res.Content != "Substantive answer" || trace.FinalProvider != "grok-pool" {
		t.Fatalf("unexpected result/trace: %+v %+v", res, trace)
	}
	if jevCalls.Load() != 0 || provider.LastAnswerCheck() != nil {
		t.Fatalf("expected 0 JEV calls and nil check, got calls=%d check=%+v", jevCalls.Load(), provider.LastAnswerCheck())
	}
}

func TestGrokSearchProvider_AnswerCheckPassesWithSourceTitleAndTokenUsage(t *testing.T) {
	var seenTitle string
	var seenSnippet string
	var jevCalls atomic.Int32
	jevSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jevCalls.Add(1)
		var payload struct {
			State struct {
				Question     string `json:"question"`
				Answer       string `json:"answer"`
				CitedSources []struct {
					URL     string `json:"url"`
					Title   string `json:"title"`
					Snippet string `json:"snippet"`
				} `json:"cited_sources"`
			} `json:"state"`
			Questions map[string]engine.JEVQuestion `json:"questions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if len(payload.State.CitedSources) != 1 {
			t.Fatalf("cited_sources = %+v", payload.State.CitedSources)
		}
		seenTitle = payload.State.CitedSources[0].Title
		seenSnippet = payload.State.CitedSources[0].Snippet
		_ = json.NewEncoder(w).Encode(map[string]any{
			"answers": map[string]any{
				"substantive":  map[string]any{"type": "noul", "noul": 0.92},
				"contradicted": map[string]any{"type": "noul", "noul": 0.08},
			},
			"usage": map[string]int{"input_tokens": 55, "output_tokens": 9},
		})
	}))
	defer jevSrv.Close()

	pool := testGrokPool(t, `{
		"choices":[{"message":{
			"content":"Go 1.24 is released.",
			"annotations":[{"type":"url_citation","url_citation":{"url":"https://go.dev/doc/go1.24","title":"Go 1.24 Release Notes"}}]
		}}]
	}`)
	provider := NewGrokSearchWithAnswerCheck(pool, engine.NewJEVClient(engine.JEVConfig{APIURL: jevSrv.URL, APIKey: "jev-test"}), true)

	r := router.New(provider)
	res, trace := r.Run(context.Background(), capability.MainSearch, capability.Request{Query: "latest Go release"})
	if res.Content != "Go 1.24 is released." || trace.FinalProvider != "grok-pool" {
		t.Fatalf("unexpected result/trace: %+v %+v", res, trace)
	}
	if jevCalls.Load() != 1 {
		t.Fatalf("jevCalls = %d, want 1", jevCalls.Load())
	}
	if seenTitle != "Go 1.24 Release Notes" || seenSnippet != "" {
		t.Fatalf("cited source metadata = title:%q snippet:%q", seenTitle, seenSnippet)
	}
	check := provider.LastAnswerCheck()
	if check == nil || check.Status != "passed" || check.Reason != "" {
		t.Fatalf("answer check = %+v", check)
	}
	if check.InputTokens == nil || *check.InputTokens != 55 || check.OutputTokens == nil || *check.OutputTokens != 9 {
		t.Fatalf("tokens = %+v", check)
	}
	if !reflect.DeepEqual(check.Scores, map[string]float64{"substantive": 0.92, "contradicted": 0.08}) {
		t.Fatalf("scores = %+v", check.Scores)
	}
}

func TestGrokSearchProvider_AnswerCheckWorksWithoutTitlesOrSnippets(t *testing.T) {
	var seenCitedCount int
	var seenTitle, seenSnippet string
	jevSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			State struct {
				CitedSources []struct {
					URL     string `json:"url"`
					Title   string `json:"title"`
					Snippet string `json:"snippet"`
				} `json:"cited_sources"`
			} `json:"state"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		seenCitedCount = len(payload.State.CitedSources)
		if seenCitedCount > 0 {
			seenTitle = payload.State.CitedSources[0].Title
			seenSnippet = payload.State.CitedSources[0].Snippet
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"answers": map[string]any{
				"substantive":  map[string]any{"type": "noul", "noul": 0.85},
				"contradicted": map[string]any{"type": "noul", "noul": 0.05},
			},
		})
	}))
	defer jevSrv.Close()

	pool := testGrokPool(t, `{"choices":[{"message":{"content":"Detailed explanation."}}],"citations":["https://example.com/only-url"]}`)
	provider := NewGrokSearchWithAnswerCheck(pool, engine.NewJEVClient(engine.JEVConfig{APIURL: jevSrv.URL, APIKey: "jev-test"}), true)

	r := router.New(provider)
	_, trace := r.Run(context.Background(), capability.MainSearch, capability.Request{Query: "explain"})
	if trace.FinalProvider != "grok-pool" || seenCitedCount != 1 || seenTitle != "" || seenSnippet != "" {
		t.Fatalf("unexpected behavior: trace=%+v count=%d title=%q snippet=%q", trace, seenCitedCount, seenTitle, seenSnippet)
	}
	check := provider.LastAnswerCheck()
	if check == nil || check.Status != "passed" || check.InputTokens != nil || check.OutputTokens != nil {
		t.Fatalf("check = %+v", check)
	}
}

func TestGrokSearchProvider_RejectionTriggersFallback(t *testing.T) {
	cases := []struct {
		name         string
		substantive  float64
		contradicted float64
		wantReason   string
	}{
		{name: "not_substantive", substantive: 0.2, contradicted: 0.05, wantReason: "not_substantive"},
		{name: "contradicted_by_sources", substantive: 0.9, contradicted: 0.85, wantReason: "contradicted_by_sources"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var jevCalls atomic.Int32
			jevSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				jevCalls.Add(1)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"answers": map[string]any{
						"substantive":  map[string]any{"type": "noul", "noul": tc.substantive},
						"contradicted": map[string]any{"type": "noul", "noul": tc.contradicted},
					},
				})
			}))
			defer jevSrv.Close()

			var tavilyCalls atomic.Int32
			tavily := testTavilyClient(t, &tavilyCalls, "Fallback verified content")
			pool := testGrokPool(t, `{
				"choices":[{"message":{"content":"I will search for that.","annotations":[{"type":"url_citation","url_citation":{"url":"https://example.com/doc","title":"Official Doc"}}]}}],
				"citations":["https://example.com/doc"]
			}`)
			provider := NewGrokSearchWithAnswerCheck(pool, engine.NewJEVClient(engine.JEVConfig{APIURL: jevSrv.URL, APIKey: "jev-test"}), true)

			r := router.New(provider, NewTavilySearch(tavily))
			res, trace := r.Run(context.Background(), capability.MainSearch, capability.Request{Query: "q"})
			if trace.FinalProvider != "tavily-search" || !trace.FallbackTriggered {
				t.Fatalf("trace = %+v", trace)
			}
			if !strings.Contains(res.Content, "Fallback verified content") {
				t.Fatalf("content = %q", res.Content)
			}
			if len(trace.Decisions) != 2 || trace.Decisions[0].Status != "empty" || trace.Decisions[0].FallbackReason != capability.ReasonAnswerRejected {
				t.Fatalf("decisions = %+v", trace.Decisions)
			}
			if jevCalls.Load() != 1 || tavilyCalls.Load() != 1 {
				t.Fatalf("calls jev=%d tavily=%d", jevCalls.Load(), tavilyCalls.Load())
			}
			check := provider.LastAnswerCheck()
			if check == nil || check.Status != "rejected" || check.Reason != tc.wantReason {
				t.Fatalf("check = %+v, want reason %q", check, tc.wantReason)
			}
		})
	}
}

func TestGrokSearchProvider_JEVFailureAndUnconfiguredKeepOriginalResultUnverified(t *testing.T) {
	t.Run("jev_http_error", func(t *testing.T) {
		jevSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("secret-jev-key internal failure"))
		}))
		defer jevSrv.Close()

		var tavilyCalls atomic.Int32
		tavily := testTavilyClient(t, &tavilyCalls, "should not run")
		pool := testGrokPool(t, `{"choices":[{"message":{"content":"Original Grok answer"}}],"citations":["https://example.com/a"]}`)
		provider := NewGrokSearchWithAnswerCheck(pool, engine.NewJEVClient(engine.JEVConfig{APIURL: jevSrv.URL, APIKey: "secret-jev-key"}), true)

		r := router.New(provider, NewTavilySearch(tavily))
		res, trace := r.Run(context.Background(), capability.MainSearch, capability.Request{Query: "q"})
		if res.Content != "Original Grok answer" || trace.FinalProvider != "grok-pool" || tavilyCalls.Load() != 0 {
			t.Fatalf("unexpected fallback or dropped answer: res=%+v trace=%+v tavily=%d", res, trace, tavilyCalls.Load())
		}
		check := provider.LastAnswerCheck()
		if check == nil || check.Status != "unverified" || check.Reason != "jev_error" || strings.Contains(check.Error, "secret-jev-key") {
			t.Fatalf("check = %+v", check)
		}
	})

	t.Run("jev_client_own_timeout", func(t *testing.T) {
		stop := make(chan struct{})
		jevSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.Copy(io.Discard, r.Body)
			select {
			case <-r.Context().Done():
			case <-stop:
			}
		}))
		defer jevSrv.Close()
		defer close(stop)

		pool := testGrokPool(t, `{"choices":[{"message":{"content":"Original Grok answer"}}],"citations":["https://example.com/a"]}`)
		provider := NewGrokSearchWithAnswerCheck(pool, engine.NewJEVClient(engine.JEVConfig{APIURL: jevSrv.URL, APIKey: "jev-key", TimeoutSec: 1}), true)

		r := router.New(provider)
		res, trace := r.Run(context.Background(), capability.MainSearch, capability.Request{Query: "q"})
		if res.Content != "Original Grok answer" || trace.FinalProvider != "grok-pool" {
			t.Fatalf("res=%+v trace=%+v", res, trace)
		}
		check := provider.LastAnswerCheck()
		if check == nil || check.Status != "unverified" || check.Reason != "jev_error" {
			t.Fatalf("check = %+v", check)
		}
	})

	t.Run("jev_unconfigured", func(t *testing.T) {
		pool := testGrokPool(t, `{"choices":[{"message":{"content":"Original Grok answer"}}],"citations":["https://example.com/a"]}`)
		provider := NewGrokSearchWithAnswerCheck(pool, engine.NewJEVClient(engine.JEVConfig{}), true)

		r := router.New(provider)
		res, trace := r.Run(context.Background(), capability.MainSearch, capability.Request{Query: "q"})
		if res.Content != "Original Grok answer" || trace.FinalProvider != "grok-pool" {
			t.Fatalf("res=%+v trace=%+v", res, trace)
		}
		check := provider.LastAnswerCheck()
		if check == nil || check.Status != "unverified" || check.Reason != "jev_unconfigured" {
			t.Fatalf("check = %+v", check)
		}
	})
}

func TestGrokSearchProvider_CallerCancellationAndTotalDeadlineDoNotRecoverToSuccess(t *testing.T) {
	t.Run("caller_canceled_during_jev", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		stop := make(chan struct{})
		jevSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.Copy(io.Discard, r.Body)
			cancel()
			select {
			case <-r.Context().Done():
			case <-stop:
			}
		}))
		defer jevSrv.Close()
		defer close(stop)

		var tavilyCalls atomic.Int32
		tavily := testTavilyClient(t, &tavilyCalls, "should not run")
		pool := testGrokPool(t, `{"choices":[{"message":{"content":"Original Grok answer"}}],"citations":["https://example.com/a"]}`)
		provider := NewGrokSearchWithAnswerCheck(pool, engine.NewJEVClient(engine.JEVConfig{APIURL: jevSrv.URL, APIKey: "jev-key", TimeoutSec: 5}), true)

		r := router.New(provider, NewTavilySearch(tavily))
		res, trace := r.Run(ctx, capability.MainSearch, capability.Request{Query: "q"})
		if res.Content != "" || trace.FinalProvider != "" || tavilyCalls.Load() != 0 {
			t.Fatalf("expected canceled failure without recovery, got res=%+v trace=%+v tavily=%d", res, trace, tavilyCalls.Load())
		}
		if len(trace.Decisions) != 1 || trace.Decisions[0].Status != "timeout" {
			t.Fatalf("decisions = %+v", trace.Decisions)
		}
	})

	t.Run("outer_deadline_exceeded_during_jev", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
		defer cancel()
		stop := make(chan struct{})
		jevSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.Copy(io.Discard, r.Body)
			select {
			case <-r.Context().Done():
			case <-stop:
			}
		}))
		defer jevSrv.Close()
		defer close(stop)

		var tavilyCalls atomic.Int32
		tavily := testTavilyClient(t, &tavilyCalls, "should not run")
		pool := testGrokPool(t, `{"choices":[{"message":{"content":"Original Grok answer"}}],"citations":["https://example.com/a"]}`)
		provider := NewGrokSearchWithAnswerCheck(pool, engine.NewJEVClient(engine.JEVConfig{APIURL: jevSrv.URL, APIKey: "jev-key", TimeoutSec: 15}), true)

		r := router.New(provider, NewTavilySearch(tavily))
		res, trace := r.Run(ctx, capability.MainSearch, capability.Request{Query: "q"})
		if res.Content != "" || trace.FinalProvider != "" || tavilyCalls.Load() != 0 {
			t.Fatalf("expected deadline failure without recovery, got res=%+v trace=%+v tavily=%d", res, trace, tavilyCalls.Load())
		}
		if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
			t.Fatalf("expected context deadline exceeded, got %v", ctx.Err())
		}
	})
}

func TestGrokSearchProvider_SkipAnswerCheckOptionSkipsJEV(t *testing.T) {
	var jevCalls atomic.Int32
	jevSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jevCalls.Add(1)
	}))
	defer jevSrv.Close()

	pool := testGrokPool(t, `{"choices":[{"message":{"content":"Grok answer"}}],"citations":["https://example.com/a"]}`)
	provider := NewGrokSearchWithAnswerCheck(pool, engine.NewJEVClient(engine.JEVConfig{APIURL: jevSrv.URL, APIKey: "jev-key"}), true)

	r := router.New(provider)
	res, trace := r.Run(context.Background(), capability.MainSearch, capability.Request{
		Query:   "q",
		Options: map[string]any{"skip_answer_check": true},
	})
	if res.Content != "Grok answer" || trace.FinalProvider != "grok-pool" || jevCalls.Load() != 0 || provider.LastAnswerCheck() != nil {
		t.Fatalf("expected skip_answer_check to bypass JEV: calls=%d check=%+v", jevCalls.Load(), provider.LastAnswerCheck())
	}
}
