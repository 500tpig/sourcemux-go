package tools

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/500tpig/sourcemux-go/internal/config"
	"github.com/500tpig/sourcemux-go/internal/engine"
)

func TestExtractPlanQueries(t *testing.T) {
	plan := BuildSearchPlan("SourceMux MCP", "standard", "GitHub")
	got := ExtractPlanQueries(plan)
	if len(got) != 4 {
		t.Fatalf("queries len = %d, want 4: %#v", len(got), got)
	}
	if got[0] != "SourceMux MCP GitHub" {
		t.Fatalf("first query = %q", got[0])
	}

	manual := `1. web_search query="quoted \"topic\"" platform="GitHub"`
	got = ExtractPlanQueries(manual)
	if len(got) != 1 || got[0] != `quoted "topic"` {
		t.Fatalf("escaped query parse = %#v", got)
	}
}

func TestDeduplicateResearchSources(t *testing.T) {
	got := deduplicateResearchSources([]researchSourceInput{
		{URL: "https://Example.com/docs/?utm_source=x#intro", Query: "q1"},
		{URL: "https://example.com/docs", Query: "q2"},
		{URL: "https://example.com/docs?ref=keep", Query: "q2"},
		{URL: "not a url", Query: "bad"},
	})

	if len(got) != 2 {
		t.Fatalf("dedup len = %d, want 2: %#v", len(got), got)
	}
	if got[0].URL != "https://example.com/docs" || got[0].Occurrences != 2 {
		t.Fatalf("first source = %+v", got[0])
	}
	if strings.Join(got[0].SearchQueries, ",") != "q1,q2" {
		t.Fatalf("queries = %#v", got[0].SearchQueries)
	}
}

func TestRankResearchSourcesFiltersAndPrioritizes(t *testing.T) {
	sources := deduplicateResearchSources([]researchSourceInput{
		{URL: "https://docs.example.com/docs/release-2026", Query: "example release docs"},
		{URL: "https://docs.example.com/docs/release-2026?utm_campaign=x", Query: "example release docs"},
		{URL: "https://forum.example.net/thread", Query: "example release docs"},
		{URL: "https://github.com/acme/example/releases", Query: "example release docs"},
	})

	ranked, filteredOut := rankResearchSources(sources, "example release docs", []string{"example.com", "github.com"})
	if filteredOut != 1 {
		t.Fatalf("filteredOut = %d, want 1", filteredOut)
	}
	if len(ranked) != 2 {
		t.Fatalf("ranked len = %d, want 2: %#v", len(ranked), ranked)
	}
	if ranked[0].URL != "https://docs.example.com/docs/release-2026" {
		t.Fatalf("top ranked = %+v", ranked[0])
	}
	if !strings.Contains(strings.Join(ranked[0].Reasons, ","), "official_or_primary") {
		t.Fatalf("top reasons = %#v", ranked[0].Reasons)
	}
}

func TestApplyFetchSignalsDownranksFailures(t *testing.T) {
	sources := []ResearchSource{
		{URL: "https://docs.example.com/docs", Domain: "docs.example.com", Score: 10, Occurrences: 1},
		{URL: "https://example.com/blog", Domain: "example.com", Score: 8, Occurrences: 1},
	}
	got := applyFetchSignals(sources, []ResearchFetchedPage{
		{URL: "https://docs.example.com/docs", Success: false, Error: "blocked"},
		{URL: "https://example.com/blog", Success: true, Excerpt: "SourceMux MCP is useful source text."},
	})
	if got[0].URL != "https://example.com/blog" {
		t.Fatalf("ranking after fetch signals = %+v", got)
	}
	if !strings.Contains(strings.Join(got[1].Reasons, ","), "fetch_failed_downrank") {
		t.Fatalf("failed source reasons = %#v", got[1].Reasons)
	}
}

func TestResearchExecutorBuildsStablePack(t *testing.T) {
	executor := &ResearchExecutor{
		Searcher: fakeResearchSearcher{},
		Fetcher:  fakeResearchFetcher{},
	}

	pack, err := executor.Run(context.Background(), ResearchOptions{
		Query:      "SourceMux MCP",
		Depth:      "quick",
		MaxFetches: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if pack.Query != "SourceMux MCP" || pack.EffectiveDepth != "quick" {
		t.Fatalf("metadata = %+v", pack)
	}
	if len(pack.ExecutedSearches) != 2 {
		t.Fatalf("executed searches len = %d", len(pack.ExecutedSearches))
	}
	if pack.SourceSummary.SelectedForFetch != 1 {
		t.Fatalf("selected_for_fetch = %d", pack.SourceSummary.SelectedForFetch)
	}
	if len(pack.FetchedPagesSummary) != 1 || !pack.FetchedPagesSummary[0].Success {
		t.Fatalf("fetched pages = %+v", pack.FetchedPagesSummary)
	}

	data, err := json.Marshal(pack)
	if err != nil {
		t.Fatal(err)
	}
	gotJSON := string(data)
	for _, want := range []string{
		`"query":"SourceMux MCP"`,
		`"effective_depth":"quick"`,
		`"executed_searches":[`,
		`"source_summary":`,
		`"fetched_pages_summary":[`,
		`"high_signal_sources":[`,
		`"confirmed_facts":[`,
		`"likely_inferences":[`,
		`"open_questions":[`,
	} {
		if !strings.Contains(gotJSON, want) {
			t.Fatalf("json missing %q in %s", want, gotJSON)
		}
	}

	human := FormatResearchPack(pack)
	for _, want := range []string{"research_pack", "executed_searches:", "confirmed_facts:", "fetched_pages_summary:"} {
		if !strings.Contains(human, want) {
			t.Fatalf("human output missing %q:\n%s", want, human)
		}
	}
}

func TestResearchExecutorPassesProfileToSearch(t *testing.T) {
	searcher := &profileRecordingSearcher{}
	executor := &ResearchExecutor{
		Searcher: searcher,
		Fetcher:  fakeResearchFetcher{},
	}

	_, err := executor.Run(context.Background(), ResearchOptions{
		Query:   "SourceMux MCP",
		Depth:   "quick",
		Profile: "heavy",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(searcher.profiles) == 0 {
		t.Fatal("expected searcher to be called")
	}
	for i, profile := range searcher.profiles {
		if profile != "heavy" {
			t.Fatalf("searcher profile[%d] = %q, want heavy", i, profile)
		}
	}
}

func TestResearchExecutorDefaultsToAutoAndUsesResolvedProfile(t *testing.T) {
	searcher := &resolvingProfileSearcher{
		resolution: SearchProfileResolution{
			RequestedProfile: "auto",
			EffectiveProfile: "heavy",
			ProfileReason:    "auto selected heavy: research flow",
		},
	}
	executor := &ResearchExecutor{
		Searcher: searcher,
		Fetcher:  fakeResearchFetcher{},
	}

	pack, err := executor.Run(context.Background(), ResearchOptions{
		Query: "SourceMux MCP",
		Depth: "standard",
	})
	if err != nil {
		t.Fatal(err)
	}
	if searcher.requested != "auto" {
		t.Fatalf("requested profile = %q, want auto", searcher.requested)
	}
	if pack.RequestedProfile != "auto" || pack.EffectiveProfile != "heavy" {
		t.Fatalf("pack profiles = requested %q effective %q", pack.RequestedProfile, pack.EffectiveProfile)
	}
	for i, profile := range searcher.profiles {
		if profile != "heavy" {
			t.Fatalf("searcher profile[%d] = %q, want heavy", i, profile)
		}
	}
}

func TestResearchExecutorUsesLongerTimeoutForHeavySearchProfile(t *testing.T) {
	searcher := &deadlineRecordingSearcher{
		resolution: SearchProfileResolution{
			RequestedProfile: "auto",
			EffectiveProfile: "heavy",
			ProfileReason:    "auto selected heavy: research flow",
		},
	}
	executor := &ResearchExecutor{
		Searcher: searcher,
		Fetcher:  fakeResearchFetcher{},
	}

	_, err := executor.Run(context.Background(), ResearchOptions{Query: "SourceMux MCP"})
	if err != nil {
		t.Fatal(err)
	}
	if len(searcher.timeouts) == 0 {
		t.Fatal("expected searcher calls")
	}
	for _, timeout := range searcher.timeouts {
		if timeout < time.Minute {
			t.Fatalf("heavy search timeout = %s, want at least 1m", timeout)
		}
	}
	if researchSearchTimeout("default") >= researchSearchTimeout("heavy") {
		t.Fatalf("default timeout should remain shorter than heavy timeout")
	}
	if researchGrokFallbackAfter("default") >= researchSearchTimeout("default") {
		t.Fatalf("default Grok fallback cap should leave fallback budget")
	}
}

func TestWithResearchGrokPoolTimeoutSetsBoundedFallbackCap(t *testing.T) {
	pool := engine.NewGrokPool([]engine.GrokEndpoint{
		{Name: "heavy", BaseURL: "http://heavy", APIKey: "k", Model: "m", Profile: "heavy"},
	})
	pool.OverallTimeout = time.Second
	clients := WebSearchClients{Pool: pool}

	got := withResearchGrokPoolTimeout(clients, researchHeavyFallbackAfter)
	if got.Pool.OverallTimeout != researchHeavyFallbackAfter {
		t.Fatalf("timeout = %s, want %s", got.Pool.OverallTimeout, researchHeavyFallbackAfter)
	}
	if clients.Pool.OverallTimeout != time.Second {
		t.Fatalf("original pool timeout mutated to %s", clients.Pool.OverallTimeout)
	}

	pool.OverallTimeout = 0
	got = withResearchGrokPoolTimeout(clients, researchHeavyFallbackAfter)
	if got.Pool.OverallTimeout != researchHeavyFallbackAfter {
		t.Fatalf("zero timeout override = %s, want %s", got.Pool.OverallTimeout, researchHeavyFallbackAfter)
	}

	pool.OverallTimeout = 90 * time.Second
	got = withResearchGrokPoolTimeout(clients, researchHeavyFallbackAfter)
	if got.Pool.OverallTimeout != researchHeavyFallbackAfter {
		t.Fatalf("long timeout override = %s, want %s", got.Pool.OverallTimeout, researchHeavyFallbackAfter)
	}
}

func TestWebSearchResearchProviderAutoDefaultLeavesTimeForFallbackAfterGrokTimeout(t *testing.T) {
	var grokCalls int32
	grok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&grokCalls, 1)
		time.Sleep(200 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"late grok"}}]}`)
	}))
	defer grok.Close()

	var tinyfishCalls int32
	tinyfish := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&tinyfishCalls, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"results":[{"title":"fallback","url":"https://example.com/fallback","snippet":"fallback result"}]}`)
	}))
	defer tinyfish.Close()

	pool := engine.NewGrokPool([]engine.GrokEndpoint{
		{Name: "default", BaseURL: grok.URL, APIKey: "sk-local", Model: "grok-test", Profile: "default"},
	})
	var fallbackProfile string
	provider := webSearchResearchProvider{
		clients: WebSearchClients{
			Pool:     pool,
			TinyFish: engine.NewTinyFishPool([]engine.TinyFishKey{{Name: "tf", APIKey: "tf-secret"}}, tinyfish.URL, ""),
		},
		grokFallbackAfter: func(profile string) time.Duration {
			fallbackProfile = profile
			return 20 * time.Millisecond
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	res, err := provider.Search(ctx, "research fallback query", "", SearchProfileAuto)
	if err != nil {
		t.Fatal(err)
	}
	if fallbackProfile != engine.DefaultGrokEndpointProfile {
		t.Fatalf("fallback profile = %q, want default", fallbackProfile)
	}
	if res.Fallback != "tinyfish" || res.Engine != "TinyFish Search" {
		t.Fatalf("result = %+v, want TinyFish fallback", res)
	}
	if atomic.LoadInt32(&grokCalls) == 0 || atomic.LoadInt32(&tinyfishCalls) == 0 {
		t.Fatalf("calls: grok=%d tinyfish=%d, want both", grokCalls, tinyfishCalls)
	}
}

func TestWebSearchResearchProviderAutoResolvedDefaultKeepsDefaultResearchFallbackWindow(t *testing.T) {
	var grokCalls int32
	grok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&grokCalls, 1)
		time.Sleep(200 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"grok default ok"}}]}`)
	}))
	defer grok.Close()

	var tinyfishCalls int32
	tinyfish := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&tinyfishCalls, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"results":[{"title":"fallback","url":"https://example.com/fallback","snippet":"fallback result"}]}`)
	}))
	defer tinyfish.Close()

	provider := webSearchResearchProvider{
		clients: WebSearchClients{
			Pool: engine.NewGrokPool([]engine.GrokEndpoint{
				{Name: "default", BaseURL: grok.URL, APIKey: "sk-local", Model: "grok-test", Profile: "default"},
			}),
			TinyFish: engine.NewTinyFishPool([]engine.TinyFishKey{{Name: "tf", APIKey: "tf-secret"}}, tinyfish.URL, ""),
			SearchPolicy: config.SearchPolicy{
				DefaultProfile: config.SearchProfileDefault,
				AgentProfile:   config.SearchProfileAuto,
				AutoPreference: config.SearchAutoPreferenceIntentBased,
				FallbackAfter:  20 * time.Millisecond,
				Timeout:        300 * time.Second,
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	res, err := provider.Search(ctx, "routine research topic", "", SearchProfileAuto)
	if err != nil {
		t.Fatal(err)
	}
	if res.Fallback != "" || res.Engine == "TinyFish Search" {
		t.Fatalf("result = %+v, want Grok result without fallback", res)
	}
	if atomic.LoadInt32(&grokCalls) == 0 {
		t.Fatal("expected grok call")
	}
	if atomic.LoadInt32(&tinyfishCalls) != 0 {
		t.Fatalf("tinyfishCalls = %d, want 0", tinyfishCalls)
	}
}

func TestResearchExecutorNoSourcesUsesStableEmptyArrays(t *testing.T) {
	executor := &ResearchExecutor{
		Searcher: emptyResearchSearcher{},
	}

	pack, err := executor.Run(context.Background(), ResearchOptions{
		Query:      "No source query",
		Depth:      "quick",
		MaxFetches: 99,
	})
	if err != nil {
		t.Fatal(err)
	}
	if pack.MaxFetches != researchMaxFetches {
		t.Fatalf("max_fetches = %d, want cap %d", pack.MaxFetches, researchMaxFetches)
	}

	data, err := json.Marshal(pack)
	if err != nil {
		t.Fatal(err)
	}
	gotJSON := string(data)
	for _, want := range []string{
		`"fetched_pages_summary":[]`,
		`"high_signal_sources":[]`,
		`"domains":[]`,
		`"selected_source_urls":[]`,
		`"high_signal_source_urls":[]`,
	} {
		if !strings.Contains(gotJSON, want) {
			t.Fatalf("json missing stable empty array %q in %s", want, gotJSON)
		}
	}
}

func TestFormatResearchPackMCPStaysThinWhileFullPackRemainsDetailed(t *testing.T) {
	pack := ResearchPack{
		Query:          "SourceMux MCP",
		EffectiveDepth: "deep",
		MaxFetches:     6,
		PlanQueries: []string{
			"plan-1", "plan-2", "plan-3", "plan-4", "plan-5-only",
		},
		ExecutedSearches: []ResearchSearchSummary{
			{Query: "search-1", Engine: "grok-a", SessionID: "s1", SourcesCount: 2, Snippet: "snippet-1"},
			{Query: "search-2", Engine: "grok-b", SessionID: "s2", SourcesCount: 2, Snippet: "snippet-2"},
			{Query: "search-3", Engine: "grok-c", SessionID: "s3", SourcesCount: 2, Snippet: "snippet-3"},
			{Query: "search-4", Engine: "grok-d", SessionID: "s4", SourcesCount: 2, Snippet: "snippet-4"},
			{Query: "search-5-only", Engine: "grok-e", SessionID: "s5", SourcesCount: 2, Snippet: "snippet-5"},
		},
		SourceSummary: ResearchSourceSummary{
			TotalURLs:        12,
			UniqueURLs:       8,
			SelectedForFetch: 6,
		},
		HighSignalSources: []ResearchSource{
			{URL: "https://example.com/source-1", Score: 10, Occurrences: 3},
			{URL: "https://example.com/source-2", Score: 9, Occurrences: 2},
			{URL: "https://example.com/source-3", Score: 8, Occurrences: 2},
			{URL: "https://example.com/source-4", Score: 7, Occurrences: 1},
			{URL: "https://example.com/source-5", Score: 6, Occurrences: 1},
			{URL: "https://example.com/source-6", Score: 5, Occurrences: 1},
			{URL: "https://example.com/source-7-only", Score: 4, Occurrences: 1},
		},
		FetchedPagesSummary: []ResearchFetchedPage{
			{URL: "https://example.com/page-1", Source: "Jina", Success: true, ContentChars: 100, Excerpt: "page-1 excerpt"},
			{URL: "https://example.com/page-2", Source: "Jina", Success: true, ContentChars: 100, Excerpt: "page-2 excerpt"},
			{URL: "https://example.com/page-3", Source: "Jina", Success: true, ContentChars: 100, Excerpt: "page-3 excerpt"},
			{URL: "https://example.com/page-4", Source: "Jina", Success: true, ContentChars: 100, Excerpt: "page-4 excerpt"},
			{URL: "https://example.com/page-5-only", Source: "Jina", Success: true, ContentChars: 100, Excerpt: "page-5 excerpt"},
		},
		ConfirmedFacts: []string{
			"fact-1", "fact-2", "fact-3", "fact-4", "fact-5-only",
		},
		LikelyInferences: []string{
			"inference-1", "inference-2", "inference-3", "inference-4", "inference-5-only",
		},
		OpenQuestions: []string{
			"question-1", "question-2", "question-3", "question-4", "question-5-only",
		},
	}

	full := FormatResearchPack(pack)
	thin := FormatResearchPackMCP(pack)

	for _, want := range []string{"plan-5-only", "search-5-only", "https://example.com/source-7-only", "https://example.com/page-5-only", "fact-5-only"} {
		if !strings.Contains(full, want) {
			t.Fatalf("full output should keep detailed item %q:\n%s", want, full)
		}
		if strings.Contains(thin, want) {
			t.Fatalf("thin MCP output should omit overflow item %q:\n%s", want, thin)
		}
	}
	for _, want := range []string{
		"search_rounds: 5",
		"fetched_pages: 5",
		"unique_sources: 8",
		"sources: call get_sources(session_id) for URLs",
		"- ... (1 more search rounds)",
		"- ... (1 more sources)",
		"- ... (1 more fetched pages)",
		"- ... (1 more)",
	} {
		if !strings.Contains(thin, want) {
			t.Fatalf("thin output missing %q:\n%s", want, thin)
		}
	}
}

func TestFormatResearchPackMCPKeepsStableCoreSectionsWhenEmpty(t *testing.T) {
	thin := FormatResearchPackMCP(ResearchPack{
		Query:          "No sources",
		EffectiveDepth: "quick",
		MaxFetches:     2,
		SourceSummary:  ResearchSourceSummary{},
	})

	for _, want := range []string{
		"plan_queries:",
		"executed_searches:",
		"source_summary:",
		"high_signal_sources:",
		"fetched_pages_summary:",
		"confirmed_facts:",
		"likely_inferences:",
		"open_questions:",
	} {
		if !strings.Contains(thin, want) {
			t.Fatalf("thin output missing stable section %q:\n%s", want, thin)
		}
	}
}

func TestExtractConfirmedFactsFiltersBoilerplate(t *testing.T) {
	pages := []ResearchFetchedPage{
		{
			URL:     "https://docs.example.com/crawl-vs-map",
			Success: true,
			Excerpt: strings.Join([]string{
				"![light logo](https://example.com/logo.svg)",
				"Navigation",
				"Ctrl K",
				"Title: Best Practices for Crawl - Example Docs",
				"Crawl vs Map",
				"response = tavily_client.crawl(\"https://docs.example.com\")",
				"Feature comparison: Crawl extracts full content while Map returns URLs only for site structure discovery.",
				"Privacy Policy",
			}, "\n"),
		},
	}

	facts := extractConfirmedFacts(pages, "Tavily Map vs Crawl official docs difference", 3)
	if len(facts) == 0 {
		t.Fatal("expected at least one fact")
	}
	if strings.Contains(strings.ToLower(facts[0]), "logo") || strings.Contains(strings.ToLower(facts[0]), "privacy") {
		t.Fatalf("boilerplate fact leaked through: %q", facts[0])
	}
	if !strings.Contains(facts[0], "Crawl extracts full content while Map returns URLs only") {
		t.Fatalf("expected comparison fact, got %q", facts[0])
	}
}

func TestExtractConfirmedFactsRequiresQueryRelevance(t *testing.T) {
	pages := []ResearchFetchedPage{
		{
			URL:     "https://docs.example.com/about",
			Success: true,
			Excerpt: strings.Join([]string{
				"Traditional search APIs return links and snippets.",
				"General platform overview without the target terms.",
			}, "\n"),
		},
	}

	facts := extractConfirmedFacts(pages, "Tavily Map vs Crawl official docs difference", 3)
	if len(facts) != 1 || !strings.Contains(facts[0], "No source-backed facts") {
		t.Fatalf("expected no extracted facts for irrelevant lines, got %#v", facts)
	}
}

type fakeResearchSearcher struct{}

func (fakeResearchSearcher) Search(ctx context.Context, query, platform, profile string) (*WebSearchResult, error) {
	if strings.Contains(query, "fail") {
		return nil, errors.New("search failed")
	}
	return &WebSearchResult{
		Query:        query,
		Engine:       "fake",
		SessionID:    "session-" + strings.ReplaceAll(query, " ", "-"),
		Content:      "fake search result for " + query,
		SourceURLs:   []string{"https://docs.example.com/docs/sourcemux-mcp", "https://example.com/blog"},
		SourcesCount: 2,
	}, nil
}

type emptyResearchSearcher struct{}

func (emptyResearchSearcher) Search(ctx context.Context, query, platform, profile string) (*WebSearchResult, error) {
	return &WebSearchResult{
		Query:      query,
		Engine:     "fake",
		SessionID:  "empty-" + strings.ReplaceAll(query, " ", "-"),
		Content:    "fake search result with no source URLs",
		SourceURLs: []string{},
	}, nil
}

type fakeResearchFetcher struct{}

func (fakeResearchFetcher) Fetch(ctx context.Context, rawURL string) (*WebFetchResult, error) {
	return &WebFetchResult{
		Source:  "fake-fetch",
		URL:     rawURL,
		Content: "# SourceMux MCP\nSourceMux MCP is a source-backed MCP server for search workflows.\nMore implementation details follow.",
	}, nil
}

// slowSearcher delays each Search call by `delay`. It records active call
// counts so the test can assert overlap (i.e., concurrency > 1).
type slowSearcher struct {
	delay      time.Duration
	maxActive  int32
	curActive  int32
	queryCount int32
}

func (s *slowSearcher) Search(ctx context.Context, query, platform, profile string) (*WebSearchResult, error) {
	atomic.AddInt32(&s.queryCount, 1)
	cur := atomic.AddInt32(&s.curActive, 1)
	for {
		prev := atomic.LoadInt32(&s.maxActive)
		if cur <= prev || atomic.CompareAndSwapInt32(&s.maxActive, prev, cur) {
			break
		}
	}
	defer atomic.AddInt32(&s.curActive, -1)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(s.delay):
	}
	return &WebSearchResult{
		Query:      query,
		Engine:     "fake",
		SessionID:  "slow-" + strings.ReplaceAll(query, " ", "-"),
		Content:    "fake slow result for " + query,
		SourceURLs: []string{"https://example.com/" + strings.ReplaceAll(query, " ", "-")},
	}, nil
}

// indexFetcher returns a result with a deterministic excerpt; it also records
// the order in which URLs are received to verify position is preserved on output.
type indexFetcher struct {
	mu       sync.Mutex
	received []string
}

func (f *indexFetcher) Fetch(ctx context.Context, rawURL string) (*WebFetchResult, error) {
	f.mu.Lock()
	f.received = append(f.received, rawURL)
	f.mu.Unlock()
	return &WebFetchResult{
		Source:  "fake-fetch",
		URL:     rawURL,
		Content: "SourceMux MCP excerpt for " + rawURL,
	}, nil
}

// timeoutFetcher returns ctx.Err on the first URL only, succeeds for the rest.
type timeoutFetcher struct {
	failURL string
}

func (t timeoutFetcher) Fetch(ctx context.Context, rawURL string) (*WebFetchResult, error) {
	if rawURL == t.failURL {
		return nil, context.DeadlineExceeded
	}
	return &WebFetchResult{
		Source:  "fake-fetch",
		URL:     rawURL,
		Content: "SourceMux MCP page " + rawURL,
	}, nil
}

func TestResearchExecutorRunsSearchesConcurrentlyPreservingOrder(t *testing.T) {
	searcher := &slowSearcher{delay: 80 * time.Millisecond}
	executor := &ResearchExecutor{
		Searcher: searcher,
		Fetcher:  fakeResearchFetcher{},
	}

	start := time.Now()
	pack, err := executor.Run(context.Background(), ResearchOptions{
		Query:      "SourceMux MCP",
		Depth:      "standard",
		MaxFetches: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)

	if len(pack.PlanQueries) < 2 {
		t.Fatalf("expected >=2 plan queries, got %d", len(pack.PlanQueries))
	}
	if atomic.LoadInt32(&searcher.maxActive) < 2 {
		t.Fatalf("expected concurrent searches (>=2 active), max active = %d", searcher.maxActive)
	}
	// With 4 plan queries × 80ms serial = 320ms; concurrency=3 should land well below.
	if elapsed > 250*time.Millisecond {
		t.Fatalf("search loop appears serial: elapsed = %s", elapsed)
	}
	if len(pack.ExecutedSearches) != len(pack.PlanQueries) {
		t.Fatalf("executed_searches len = %d, want %d", len(pack.ExecutedSearches), len(pack.PlanQueries))
	}
	for i, q := range pack.PlanQueries {
		if pack.ExecutedSearches[i].Query != q {
			t.Fatalf("executed_searches[%d].Query = %q, want %q", i, pack.ExecutedSearches[i].Query, q)
		}
	}
}

type profileRecordingSearcher struct {
	mu       sync.Mutex
	profiles []string
}

func (s *profileRecordingSearcher) Search(ctx context.Context, query, platform, profile string) (*WebSearchResult, error) {
	s.mu.Lock()
	s.profiles = append(s.profiles, profile)
	s.mu.Unlock()
	return (&fakeResearchSearcher{}).Search(ctx, query, platform, profile)
}

type resolvingProfileSearcher struct {
	mu         sync.Mutex
	requested  string
	profiles   []string
	resolution SearchProfileResolution
}

func (s *resolvingProfileSearcher) ResolveSearchProfile(requested string, opts ResearchOptions) (SearchProfileResolution, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requested = requested
	return s.resolution, nil
}

func (s *resolvingProfileSearcher) Search(ctx context.Context, query, platform, profile string) (*WebSearchResult, error) {
	s.mu.Lock()
	s.profiles = append(s.profiles, profile)
	s.mu.Unlock()
	return (&fakeResearchSearcher{}).Search(ctx, query, platform, profile)
}

type deadlineRecordingSearcher struct {
	mu         sync.Mutex
	timeouts   []time.Duration
	resolution SearchProfileResolution
}

func (s *deadlineRecordingSearcher) ResolveSearchProfile(requested string, opts ResearchOptions) (SearchProfileResolution, error) {
	return s.resolution, nil
}

func (s *deadlineRecordingSearcher) Search(ctx context.Context, query, platform, profile string) (*WebSearchResult, error) {
	if deadline, ok := ctx.Deadline(); ok {
		s.mu.Lock()
		s.timeouts = append(s.timeouts, time.Until(deadline))
		s.mu.Unlock()
	}
	return (&fakeResearchSearcher{}).Search(ctx, query, platform, profile)
}

func TestFetchSelectedSourcesPreservesURLOrder(t *testing.T) {
	executor := &ResearchExecutor{
		Fetcher: &indexFetcher{},
	}
	urls := []string{
		"https://a.example.com/one",
		"https://b.example.com/two",
		"https://c.example.com/three",
		"https://d.example.com/four",
	}

	pages := executor.fetchSelectedSources(context.Background(), urls, 4)
	if len(pages) != len(urls) {
		t.Fatalf("pages len = %d, want %d", len(pages), len(urls))
	}
	for i, want := range urls {
		if pages[i].URL != want {
			t.Fatalf("pages[%d].URL = %q, want %q", i, pages[i].URL, want)
		}
		if !pages[i].Success {
			t.Fatalf("pages[%d] not successful", i)
		}
	}
}

func TestFetchSelectedSourcesPerCallTimeoutIsolated(t *testing.T) {
	urls := []string{
		"https://timeout.example.com/slow",
		"https://ok.example.com/page",
	}
	executor := &ResearchExecutor{
		Fetcher: timeoutFetcher{failURL: urls[0]},
	}
	pages := executor.fetchSelectedSources(context.Background(), urls, 2)
	if pages[0].Success {
		t.Fatalf("expected timeout URL to be marked failed: %+v", pages[0])
	}
	if pages[0].Error == "" {
		t.Fatalf("expected non-empty error on timeout URL")
	}
	if !pages[1].Success {
		t.Fatalf("expected sibling URL to succeed even when peer timed out: %+v", pages[1])
	}
}
