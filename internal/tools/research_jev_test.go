package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/500tpig/sourcemux-go/internal/engine"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

type jevFakeSearch struct {
	calls []JEVAction
	urls  []string
}

func (s *jevFakeSearch) Search(_ context.Context, query, _, _ string) (*WebSearchResult, error) {
	return &WebSearchResult{Query: query, SourceURLs: s.urls}, nil
}
func (s *jevFakeSearch) jevProviders(string) []string { return []string{"allowed"} }
func (s *jevFakeSearch) searchJEV(_ context.Context, provider, query, _, _ string) (*WebSearchResult, error) {
	s.calls = append(s.calls, JEVAction{Provider: provider, Query: query})
	return &WebSearchResult{Engine: provider, SourceURLs: s.urls, Content: "discovery is not evidence"}, nil
}

type jevFakeFetch struct {
	calls []string
	empty bool
	url   string
}

func (f *jevFakeFetch) Fetch(_ context.Context, url string) (*WebFetchResult, error) {
	f.calls = append(f.calls, url)
	content := "SourceMux evidence supports the requested technical facts and limitations."
	if f.empty {
		content = ""
	}
	if f.url != "" {
		url = f.url
	}
	return &WebFetchResult{URL: url, Content: content}, nil
}

func jevTestClient(t *testing.T, assessStatus int, sufficient bool) *engine.JEVClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			State struct {
				Available []JEVAction     `json:"available_actions"`
				Evidence  json.RawMessage `json:"evidence"`
			} `json:"state"`
			Questions map[string]engine.JEVQuestion `json:"questions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		answers := map[string]any{"injected_action": map[string]any{"type": "noul", "noul": 1}}
		if _, assessment := body.Questions["sufficient"]; assessment {
			if assessStatus != 200 {
				w.WriteHeader(assessStatus)
				return
			}
			if strings.Contains(string(body.State.Evidence), "discovery is not evidence") {
				t.Error("search snippet sent for sufficiency")
			}
			for id := range body.Questions {
				score := 0.0
				if id == "useful" || (id == "sufficient" && sufficient) {
					score = 0.95
				}
				answers[id] = map[string]any{"type": "noul", "noul": score}
			}
		} else {
			for i, action := range body.State.Available {
				if action.Operation == "search" && action.Provider != "allowed" {
					t.Errorf("unexpected provider: %s", action.Provider)
				}
				score := 0.0
				if i == 0 {
					score = 0.9
				}
				answers[action.ID] = map[string]any{"type": "noul", "noul": score}
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"answers": answers, "usage": map[string]int{"input_tokens": 12}})
	}))
	t.Cleanup(srv.Close)
	return engine.NewJEVClient(engine.JEVConfig{APIURL: srv.URL, APIKey: "test-key"})
}

func TestJEVResearchStopsWithReadEvidence(t *testing.T) {
	search := &jevFakeSearch{urls: []string{"https://example.com/docs", "https://example.com/discovery"}}
	fetch := &jevFakeFetch{}
	executor := &ResearchExecutor{Searcher: search, Fetcher: fetch, JEV: jevTestClient(t, 200, true)}
	pack, err := executor.Run(context.Background(), ResearchOptions{Query: "SourceMux", Mode: "jev", Depth: "deep", MaxFetches: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !pack.JEV.Assessment.Sufficient || pack.JEV.StopReason != "sufficient" {
		t.Fatalf("trace: %+v", pack.JEV)
	}
	if len(search.calls) != 1 || len(fetch.calls) != 1 || len(pack.JEV.Rounds) != 2 {
		t.Fatalf("search=%v fetch=%v rounds=%v", search.calls, fetch.calls, pack.JEV.Rounds)
	}
	if len(pack.JEV.Citations) != 1 || pack.JEV.Citations[0] != fetch.calls[0] {
		t.Fatalf("citations: %v", pack.JEV.Citations)
	}
	if len(pack.JEV.Calls) != 3 || pack.JEV.Calls[0].InputTokens == nil {
		t.Fatalf("calls: %+v", pack.JEV.Calls)
	}
	if len(pack.JEV.Rounds[0].Scores) == 0 {
		t.Fatal("missing selection scores")
	}
	for _, round := range pack.JEV.Rounds {
		if _, ok := round.Scores["injected_action"]; ok {
			t.Fatal("accepted unknown ID")
		}
	}
}

func TestJEVResearchJudgmentFailurePreservesEvidence(t *testing.T) {
	executor := &ResearchExecutor{Searcher: &jevFakeSearch{urls: []string{"https://example.com/docs"}}, Fetcher: &jevFakeFetch{}, JEV: jevTestClient(t, 503, true)}
	pack, err := executor.Run(context.Background(), ResearchOptions{Query: "SourceMux", Mode: "jev", Depth: "deep"})
	if err != nil {
		t.Fatal(err)
	}
	if pack.JEV.Assessment.Status != "unknown" || pack.JEV.StopReason != "judgment_unavailable" || len(pack.JEV.Citations) != 1 || !pack.FetchedPagesSummary[0].Success {
		t.Fatalf("pack: %+v", pack)
	}
	if got := BuildResearchAgentOutput(pack).AnswerReadiness; got != "unknown" {
		t.Fatalf("readiness=%s", got)
	}
}

func TestJEVResearchEmptyAndDisallowedBodiesNeverBecomeCitations(t *testing.T) {
	for _, tc := range []struct {
		name     string
		empty    bool
		redirect string
		domains  []string
	}{
		{name: "empty", empty: true},
		{name: "outside domain", redirect: "https://other.example/docs", domains: []string{"example.com"}},
		{name: "invalid URL", redirect: "javascript:alert(1)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fetch := &jevFakeFetch{empty: tc.empty, url: tc.redirect}
			executor := &ResearchExecutor{Searcher: &jevFakeSearch{urls: []string{"https://example.com/docs"}}, Fetcher: fetch, JEV: jevTestClient(t, 200, true)}
			pack, err := executor.Run(context.Background(), ResearchOptions{Query: "SourceMux", Mode: "jev", Depth: "deep", MaxFetches: 1, Domains: tc.domains})
			if err != nil {
				t.Fatal(err)
			}
			if len(pack.JEV.Citations) != 0 || pack.JEV.Assessment.Sufficient || len(fetch.calls) != 1 {
				t.Fatalf("pack=%+v fetch=%v", pack, fetch.calls)
			}
			for _, call := range pack.JEV.Calls {
				if call.Phase == "assessment" {
					t.Fatal("assessed no body")
				}
			}
		})
	}
}

func TestJEVResearchBudgetAndNoDuplicateActions(t *testing.T) {
	search := &jevFakeSearch{urls: []string{"https://example.com/a", "https://example.com/b"}}
	fetch := &jevFakeFetch{}
	executor := &ResearchExecutor{Searcher: search, Fetcher: fetch, JEV: jevTestClient(t, 200, false)}
	pack, err := executor.Run(context.Background(), ResearchOptions{Query: "SourceMux", Mode: "jev", Depth: "deep", MaxFetches: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(fetch.calls) != 1 || len(pack.JEV.Rounds) > 5 || pack.JEV.StopReason != "no_new_evidence" {
		t.Fatalf("trace=%+v fetch=%v", pack.JEV, fetch.calls)
	}
	seen := map[string]bool{}
	for _, round := range pack.JEV.Rounds {
		for _, a := range round.Selected {
			key := jevActionKey(a)
			if seen[key] {
				t.Fatalf("duplicate: %+v", a)
			}
			seen[key] = true
		}
	}
}

func TestJEVResearchFixedModeAndMissingKey(t *testing.T) {
	client := engine.NewJEVClient(engine.JEVConfig{})
	executor := &ResearchExecutor{Searcher: &jevFakeSearch{}, Fetcher: &jevFakeFetch{}, JEV: client}
	pack, err := executor.Run(context.Background(), ResearchOptions{Query: "SourceMux", Mode: "fixed"})
	if err != nil || pack.JEV != nil {
		t.Fatalf("fixed: %+v %v", pack, err)
	}
	_, err = executor.Run(context.Background(), ResearchOptions{Query: "SourceMux", Mode: "jev"})
	if err == nil || !strings.Contains(err.Error(), "apiKey") {
		t.Fatalf("missing key: %v", err)
	}
	_, err = executor.Run(context.Background(), ResearchOptions{Query: "SourceMux", Mode: "other"})
	if err == nil {
		t.Fatal("accepted unknown mode")
	}
}

func TestJEVResearchCancellationDoesNotStartActions(t *testing.T) {
	search := &jevFakeSearch{}
	executor := &ResearchExecutor{Searcher: search, JEV: engine.NewJEVClient(engine.JEVConfig{APIKey: "test-key"})}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	pack, err := executor.Run(ctx, ResearchOptions{Query: "SourceMux", Mode: "jev"})
	if err != nil || pack.JEV.StopReason != "deadline" || len(search.calls) != 0 || len(pack.JEV.Calls) != 0 {
		t.Fatalf("pack=%+v err=%v", pack, err)
	}
}

func TestJEVEvidencePreviewIsBoundedAndMarksOmissions(t *testing.T) {
	pages := make([]ResearchFetchedPage, 30)
	for i := range pages {
		pages[i] = ResearchFetchedPage{Success: true, URL: fmt.Sprintf("https://example.com/%d", i), Excerpt: strings.Repeat("正文", 600), ContentChars: 2000}
	}
	preview := jevEvidencePreview(pages)
	encoded, _ := json.Marshal(preview)
	if len(encoded) > 20000 || preview["omitted_pages"].(int) == 0 || !strings.Contains(string(encoded), `"truncated":true`) {
		t.Fatalf("len=%d preview=%s", len(encoded), encoded)
	}
}

func TestJEVResearchMCPMode(t *testing.T) {
	s := mcpserver.NewMCPServer("test", "1")
	RegisterResearchRun(s, &ResearchExecutor{Searcher: &jevFakeSearch{}})
	request := json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"research_run","arguments":{"query":"SourceMux","mode":"jev"}}}`)
	response := s.HandleMessage(context.Background(), request)
	encoded, _ := json.Marshal(response)
	if !strings.Contains(string(encoded), "jev.apiKey") {
		t.Fatalf("MCP mode not applied: %s", encoded)
	}
}

func TestJEVResearchTimeouts(t *testing.T) {
	if JEVResearchTimeout("quick") != 45*time.Second || JEVResearchTimeout("standard") != 120*time.Second || JEVResearchTimeout("deep") != 300*time.Second {
		t.Fatal("incorrect budgets")
	}
}

func TestJEVProviderCandidatesRespectProfileAndDisabledChannels(t *testing.T) {
	disabled := false
	p := webSearchResearchProvider{clients: WebSearchClients{
		Pool: engine.NewGrokPool([]engine.GrokEndpoint{
			{Name: "heavy", Profile: "heavy", BaseURL: "https://example.invalid", APIKey: "test"},
			{Name: "disabled", Profile: "default", Enabled: &disabled, BaseURL: "https://example.invalid", APIKey: "test"},
		}),
		Exa: engine.NewExaClient("https://example.invalid", "test"),
	}}
	if got := strings.Join(p.jevProviders("default"), ","); got != "exa-search" {
		t.Fatalf("default providers = %s", got)
	}
	if got := strings.Join(p.jevProviders("heavy"), ","); got != "grok-pool,exa-search" {
		t.Fatalf("heavy providers = %s", got)
	}
}

func TestJEVSearchExecutesOnlySelectedConfiguredProvider(t *testing.T) {
	var exaCalls, tavilyCalls atomic.Int32
	exa := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		exaCalls.Add(1)
		_, _ = w.Write([]byte(`{"results":[{"url":"https://example.com/docs","title":"Docs","text":"SourceMux documentation"}]}`))
	}))
	defer exa.Close()
	tavily := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { tavilyCalls.Add(1); w.WriteHeader(500) }))
	defer tavily.Close()
	p := webSearchResearchProvider{clients: WebSearchClients{Exa: engine.NewExaClient(exa.URL, "test"), Tavily: engine.NewTavilyClient(tavily.URL, "test")}}
	result, err := p.searchJEV(context.Background(), "exa-search", "SourceMux", "", "default")
	if err != nil || len(result.SourceURLs) != 1 || exaCalls.Load() != 1 || tavilyCalls.Load() != 0 {
		t.Fatalf("result=%+v err=%v calls=%d/%d", result, err, exaCalls.Load(), tavilyCalls.Load())
	}
	_, err = p.searchJEV(context.Background(), "unconfigured", "SourceMux", "", "default")
	if err == nil || exaCalls.Load() != 1 || tavilyCalls.Load() != 0 {
		t.Fatal("unconfigured provider executed")
	}
}

func TestJEVNoSuitableActionsDoesNotChooseHighestBelowThreshold(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Questions map[string]engine.JEVQuestion `json:"questions"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		answers := map[string]any{}
		for id := range request.Questions {
			answers[id] = map[string]any{"type": "noul", "noul": 0.49}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"answers": answers})
	}))
	defer srv.Close()
	search := &jevFakeSearch{}
	executor := &ResearchExecutor{Searcher: search, JEV: engine.NewJEVClient(engine.JEVConfig{APIURL: srv.URL, APIKey: "test"})}
	pack, err := executor.Run(context.Background(), ResearchOptions{Query: "SourceMux", Mode: "jev"})
	if err != nil || len(search.calls) != 0 || pack.JEV.StopReason != "no_suitable_actions" {
		t.Fatalf("pack=%+v err=%v", pack, err)
	}
}

func TestJEVCandidatesIncludeActionBoundariesAndExclusions(t *testing.T) {
	actions := jevCandidates(
		ResearchPack{Query: "SourceMux", MaxFetches: 2, JEV: &JEVResearchTrace{}},
		[]string{"grok-pool", "tinyfish-search", "exa-search", "tavily-search"},
		[]ResearchSource{{URL: "https://example.com/doc"}},
		map[string]bool{},
	)
	byProvider := make(map[string]JEVAction, len(actions))
	for _, action := range actions {
		if _, exists := byProvider[action.Provider]; !exists {
			byProvider[action.Provider] = action
		}
	}
	expectations := map[string][]string{
		"web_fetch":       {"citations", "unsuitable", "max_fetches"},
		"grok-pool":       {"model answer", "not read-page evidence", "domain allow-lists"},
		"tinyfish-search": {"only the query", "without server-side domain", "cannot fetch pages"},
		"exa-search":      {"highlights only", "no full-page text", "require fetch"},
		"tavily-search":   {"without server-side domain", "not return full page bodies", "not read-page citation evidence"},
	}
	for provider, wants := range expectations {
		action, ok := byProvider[provider]
		if !ok {
			t.Fatalf("missing candidate for %s", provider)
		}
		lower := strings.ToLower(action.Guidance)
		for _, want := range wants {
			if !strings.Contains(lower, strings.ToLower(want)) {
				t.Fatalf("%s guidance %q missing boundary %q", provider, action.Guidance, want)
			}
		}
	}
}

func TestResearchAndSmartAnswerSkipSearchAnswerCheck(t *testing.T) {
	grokSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"I am about to search."}}],"citations":["https://example.com/doc"]}`))
	}))
	defer grokSrv.Close()

	fetchSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("Detailed documentation about SourceMux capabilities."))
	}))
	defer fetchSrv.Close()

	var answerCheckCalls atomic.Int32
	jevSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Questions map[string]engine.JEVQuestion `json:"questions"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if _, isAnswerCheck := body.Questions["substantive"]; isAnswerCheck {
			answerCheckCalls.Add(1)
		}
		answers := map[string]any{}
		for id := range body.Questions {
			score := 0.0
			if id == "action_0" || id == "useful" || id == "sufficient" {
				score = 0.95
			}
			answers[id] = map[string]any{"type": "noul", "noul": score}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"answers": answers})
	}))
	defer jevSrv.Close()

	jevClient := engine.NewJEVClient(engine.JEVConfig{APIURL: jevSrv.URL, APIKey: "jev-key", AnswerCheck: true})
	executor := NewResearchExecutor(ResearchExecutorDeps{
		JEV: jevClient,
		Search: WebSearchClients{
			Pool:        engine.NewGrokPool([]engine.GrokEndpoint{{Name: "primary", BaseURL: grokSrv.URL, APIKey: "sk-g", Model: "grok-test"}}),
			JEV:         jevClient,
			AnswerCheck: true,
		},
		Fetch: WebFetchClients{
			Jina:        engine.NewJinaClient(fetchSrv.URL, ""),
			Order:       []string{"jina"},
			StrictOrder: true,
		},
	})

	if _, err := executor.Run(context.Background(), ResearchOptions{Query: "SourceMux", Mode: "fixed", Depth: "quick", MaxFetches: 1}); err != nil {
		t.Fatalf("fixed research failed: %v", err)
	}
	if _, err := executor.Run(context.Background(), ResearchOptions{Query: "SourceMux", Mode: "jev", Depth: "quick", MaxFetches: 1}); err != nil {
		t.Fatalf("jev research failed: %v", err)
	}
	answerer := &SmartAnswerer{
		Researcher: executor,
		Reasoner: &fakeSmartReasoner{
			result: &engine.PoolReasoningResult{
				ReasoningResult: &engine.ReasoningResult{Content: "Synthesized answer."},
				EndpointName:    "deepseek",
				EndpointModel:   "deepseek-v4-flash",
			},
		},
	}
	if _, err := answerer.Run(context.Background(), SmartAnswerOptions{Query: "SourceMux", Mode: "jev", Depth: "quick", MaxFetches: 1}); err != nil {
		t.Fatalf("smart-answer failed: %v", err)
	}
	if got := answerCheckCalls.Load(); got != 0 {
		t.Fatalf("research/smart-answer triggered search answer_check %d times, want 0", got)
	}
}
