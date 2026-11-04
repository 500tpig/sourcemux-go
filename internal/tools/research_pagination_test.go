package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/500tpig/sourcemux-go/internal/engine"
)

type paginationFetcher struct {
	content map[string]string
	calls   []string
}

func (f *paginationFetcher) Fetch(_ context.Context, rawURL string) (*WebFetchResult, error) {
	f.calls = append(f.calls, rawURL)
	return &WebFetchResult{URL: rawURL, Content: f.content[rawURL]}, nil
}

func paginationJudge(t *testing.T, assess func(string, int) (float64, int)) *engine.JEVClient {
	t.Helper()
	assessments := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			State struct {
				Available []JEVAction     `json:"available_actions"`
				Evidence  json.RawMessage `json:"evidence"`
			} `json:"state"`
			Questions map[string]engine.JEVQuestion `json:"questions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		answers := map[string]any{}
		for id := range request.Questions {
			answers[id] = map[string]any{"type": "noul", "noul": 0.0}
		}
		if _, ok := request.Questions["sufficient"]; ok {
			assessments++
			score, status := assess(string(request.State.Evidence), assessments)
			if status != http.StatusOK {
				w.WriteHeader(status)
				return
			}
			answers["useful"] = map[string]any{"type": "noul", "noul": 0.9}
			answers["sufficient"] = map[string]any{"type": "noul", "noul": score}
		} else {
			for i, action := range request.State.Available {
				if action.Operation == "fetch" || i == 0 {
					answers[action.ID] = map[string]any{"type": "noul", "noul": 0.9}
				}
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"answers": answers})
	}))
	t.Cleanup(srv.Close)
	return engine.NewJEVClient(engine.JEVConfig{APIURL: srv.URL, APIKey: "test-key"})
}

func TestJEVPaginationFindsLaterEvidenceWithoutRefetch(t *testing.T) {
	const rawURL = "https://example.com/a"
	const answer = "targetfact: the answer is in the later page."
	content := strings.Repeat("Unrelated context.\n", 450) + "\n" + answer + "\n\n" + strings.Repeat("Tail context.\n", 1000)
	fetcher := &paginationFetcher{content: map[string]string{rawURL: content}}
	judge := paginationJudge(t, func(evidence string, _ int) (float64, int) {
		if strings.Contains(evidence, answer) {
			return 0.9, 200
		}
		return 0.1, 200
	})
	executor := &ResearchExecutor{Searcher: &jevFakeSearch{urls: []string{rawURL}}, Fetcher: fetcher, JEV: judge}
	pack, err := executor.Run(context.Background(), ResearchOptions{Query: "targetfact", Mode: "jev", Depth: "standard", MaxFetches: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !pack.JEV.Assessment.Sufficient || pack.JEV.ReadPages != 2 || len(fetcher.calls) != 1 {
		t.Fatalf("trace=%+v fetches=%v", pack.JEV, fetcher.calls)
	}
	page := pack.FetchedPagesSummary[0]
	if page.Reading.Complete || !strings.Contains(page.Excerpt, answer) || len([]rune(page.Excerpt)) > 2000 {
		t.Fatalf("page=%+v", page)
	}
	for _, passage := range page.Reading.Passages {
		if passage.Text != string([]rune(content)[passage.Start:passage.End]) {
			t.Fatal("invalid source position")
		}
	}
	if !strings.Contains(buildSmartAnswerUserPrompt(pack.Query, pack), answer) {
		t.Fatal("synthesis lost later evidence")
	}
}

func TestJEVPaginationAlternatesFetchedSources(t *testing.T) {
	const a, b = "https://example.com/a", "https://example.com/b"
	prefix := strings.Repeat("Unrelated background.\n", 300)
	fetcher := &paginationFetcher{content: map[string]string{
		a: prefix + "\n\ntargetfact partial detail.\n\n" + prefix,
		b: prefix + "\n\ntargetfact decisive answer.\n\n" + prefix,
	}}
	judge := paginationJudge(t, func(evidence string, _ int) (float64, int) {
		if strings.Contains(evidence, "targetfact decisive answer") {
			return 0.9, 200
		}
		return 0.1, 200
	})
	executor := &ResearchExecutor{Searcher: &jevFakeSearch{urls: []string{a, b}}, Fetcher: fetcher, JEV: judge}
	pack, err := executor.Run(context.Background(), ResearchOptions{Query: "targetfact", Mode: "jev", Depth: "standard", MaxFetches: 2})
	if err != nil {
		t.Fatal(err)
	}
	if pack.JEV.ReadPages != 4 || len(fetcher.calls) != 2 || !pack.JEV.Assessment.Sufficient {
		t.Fatalf("trace=%+v fetches=%v", pack.JEV, fetcher.calls)
	}
	for _, page := range pack.FetchedPagesSummary {
		if page.Reading.PagesRead != 2 {
			t.Fatalf("page=%+v", page)
		}
	}
}

func TestJEVPaginationStopsAtBudgetAndPreservesEvidence(t *testing.T) {
	const rawURL = "https://example.com/a"
	fetcher := &paginationFetcher{content: map[string]string{rawURL: strings.Repeat("targetfact incomplete evidence.\n", 4000)}}
	judge := paginationJudge(t, func(string, int) (float64, int) { return 0.1, 200 })
	executor := &ResearchExecutor{Searcher: &jevFakeSearch{urls: []string{rawURL}}, Fetcher: fetcher, JEV: judge}
	pack, err := executor.Run(context.Background(), ResearchOptions{Query: "targetfact", Mode: "jev", Depth: "quick", MaxFetches: 1})
	if err != nil {
		t.Fatal(err)
	}
	page := pack.FetchedPagesSummary[0]
	if pack.JEV.StopReason != "reading_limit" || pack.JEV.ReadPages != 2 || len(fetcher.calls) != 1 || page.Excerpt == "" || page.Reading.Complete || !page.Reading.Truncated {
		t.Fatalf("pack=%+v", pack)
	}
	if len([]rune(page.Excerpt)) > 2000 || len(pack.JEV.Citations) != 1 {
		t.Fatal("lost bounded evidence")
	}
}

func TestJEVPaginationStopsOnJudgmentFailure(t *testing.T) {
	const rawURL = "https://example.com/a"
	fetcher := &paginationFetcher{content: map[string]string{rawURL: strings.Repeat("targetfact unverified evidence.\n", 1000)}}
	judge := paginationJudge(t, func(_ string, n int) (float64, int) {
		if n == 2 {
			return 0, 503
		}
		return 0.1, 200
	})
	executor := &ResearchExecutor{Searcher: &jevFakeSearch{urls: []string{rawURL}}, Fetcher: fetcher, JEV: judge}
	pack, err := executor.Run(context.Background(), ResearchOptions{Query: "targetfact", Mode: "jev", Depth: "standard"})
	if err != nil {
		t.Fatal(err)
	}
	if pack.JEV.StopReason != "judgment_unavailable" || pack.JEV.Assessment.Status != "unknown" || pack.JEV.ReadPages != 2 || len(fetcher.calls) != 1 || pack.FetchedPagesSummary[0].Excerpt == "" {
		t.Fatalf("pack=%+v", pack)
	}
}

func TestJEVPaginationStopsWhenFirstPageSufficient(t *testing.T) {
	const rawURL = "https://example.com/a"
	fetcher := &paginationFetcher{content: map[string]string{rawURL: "targetfact sufficient evidence.\n\n" + strings.Repeat("Unrelated text.\n", 1000)}}
	judge := paginationJudge(t, func(string, int) (float64, int) { return 0.9, 200 })
	executor := &ResearchExecutor{Searcher: &jevFakeSearch{urls: []string{rawURL}}, Fetcher: fetcher, JEV: judge}
	pack, err := executor.Run(context.Background(), ResearchOptions{Query: "targetfact", Mode: "jev"})
	if err != nil {
		t.Fatal(err)
	}
	if pack.JEV.StopReason != "sufficient" || pack.JEV.ReadPages != 1 || pack.FetchedPagesSummary[0].Reading.Complete {
		t.Fatalf("pack=%+v", pack)
	}
}

func TestJEVPaginationCancellationStopsReading(t *testing.T) {
	const rawURL = "https://example.com/a"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fetcher := &paginationFetcher{content: map[string]string{rawURL: strings.Repeat("targetfact text.\n", 1000)}}
	judge := paginationJudge(t, func(string, int) (float64, int) { cancel(); return 0.1, 200 })
	executor := &ResearchExecutor{Searcher: &jevFakeSearch{urls: []string{rawURL}}, Fetcher: fetcher, JEV: judge}
	pack, err := executor.Run(ctx, ResearchOptions{Query: "targetfact", Mode: "jev"})
	if err != nil {
		t.Fatal(err)
	}
	if pack.JEV.StopReason != "deadline" || pack.JEV.ReadPages != 1 || len(fetcher.calls) != 1 {
		t.Fatalf("pack=%+v", pack)
	}
}

func TestJEVPaginationContinuesPastEmptyFirstPage(t *testing.T) {
	const rawURL = "https://example.com/a"
	fetcher := &paginationFetcher{content: map[string]string{rawURL: strings.Repeat(" ", 6500) + "\n\ntargetfact answer.\n"}}
	judge := paginationJudge(t, func(evidence string, _ int) (float64, int) {
		if !strings.Contains(evidence, "targetfact answer") {
			t.Error("judged empty excerpt")
		}
		return 0.9, 200
	})
	executor := &ResearchExecutor{Searcher: &jevFakeSearch{urls: []string{rawURL}}, Fetcher: fetcher, JEV: judge}
	pack, err := executor.Run(context.Background(), ResearchOptions{Query: "targetfact", Mode: "jev"})
	if err != nil {
		t.Fatal(err)
	}
	if !pack.JEV.Assessment.Sufficient || pack.JEV.ReadPages != 2 {
		t.Fatalf("pack=%+v", pack)
	}
}
