package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestExtractPlanQueries(t *testing.T) {
	plan := BuildSearchPlan("Grok Search MCP", "standard", "GitHub")
	got := ExtractPlanQueries(plan)
	if len(got) != 4 {
		t.Fatalf("queries len = %d, want 4: %#v", len(got), got)
	}
	if got[0] != "Grok Search MCP GitHub" {
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
		{URL: "https://example.com/blog", Success: true, Excerpt: "Grok Search MCP is useful source text."},
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
		Query:      "Grok Search MCP",
		Depth:      "quick",
		MaxFetches: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if pack.Query != "Grok Search MCP" || pack.EffectiveDepth != "quick" {
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
		`"query":"Grok Search MCP"`,
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

func (fakeResearchSearcher) Search(ctx context.Context, query, platform string) (*WebSearchResult, error) {
	if strings.Contains(query, "fail") {
		return nil, errors.New("search failed")
	}
	return &WebSearchResult{
		Query:        query,
		Engine:       "fake",
		SessionID:    "session-" + strings.ReplaceAll(query, " ", "-"),
		Content:      "fake search result for " + query,
		SourceURLs:   []string{"https://docs.example.com/docs/grok-search-mcp", "https://example.com/blog"},
		SourcesCount: 2,
	}, nil
}

type emptyResearchSearcher struct{}

func (emptyResearchSearcher) Search(ctx context.Context, query, platform string) (*WebSearchResult, error) {
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
		Content: "# Grok Search MCP\nGrok Search MCP is a source-backed MCP server for search workflows.\nMore implementation details follow.",
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

func (s *slowSearcher) Search(ctx context.Context, query, platform string) (*WebSearchResult, error) {
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
		Content: "Grok Search MCP excerpt for " + rawURL,
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
		Content: "Grok Search MCP page " + rawURL,
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
		Query:      "Grok Search MCP",
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
