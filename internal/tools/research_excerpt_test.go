package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type excerptTestFetcher string

func (f excerptTestFetcher) Fetch(_ context.Context, rawURL string) (*WebFetchResult, error) {
	return &WebFetchResult{URL: rawURL, Content: string(f)}, nil
}

func TestResearchRetainsLaterEvidenceForJudgmentAndSynthesis(t *testing.T) {
	const answer = "go mod tidy adds missing and removes unused modules."
	content := strings.Repeat("前", 5000) + "\n" + answer + strings.Repeat("后", 2000)
	executor := &ResearchExecutor{Fetcher: excerptTestFetcher(content)}
	pages := executor.fetchSelectedSources(context.Background(), []string{"https://example.com/docs"}, 1)
	if len(pages) != 1 || !pages[0].Success || !strings.Contains(pages[0].Excerpt, answer) {
		t.Fatal("later evidence was lost during fetch")
	}
	if pages[0].ContentChars != len([]rune(content)) || len([]rune(pages[0].Excerpt)) > 6003 {
		t.Fatal("original length or bounded excerpt is incorrect")
	}
	preview, err := json.Marshal(jevEvidencePreview(pages))
	if err != nil || !strings.Contains(string(preview), answer) {
		t.Fatalf("judgment lost later evidence: %v", err)
	}
	pack := ResearchPack{FetchedPagesSummary: pages}
	if !strings.Contains(buildSmartAnswerUserPrompt("go mod tidy", pack), answer) {
		t.Fatal("synthesis lost evidence during display formatting")
	}
}
