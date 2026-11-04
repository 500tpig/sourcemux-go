package tools

import (
	"strings"
	"testing"
)

func TestResearchAgentJEVJudgmentOverridesHeuristicFacts(t *testing.T) {
	for _, tc := range []struct {
		status     string
		sufficient bool
		readiness  string
	}{
		{"unknown", false, "unknown"},
		{"partial", false, "needs_more_evidence"},
		{"sufficient", true, "ready"},
	} {
		t.Run(tc.status, func(t *testing.T) {
			pack := ResearchPack{
				ConfirmedFacts:      []string{"A heuristic fact."},
				HighSignalSources:   []ResearchSource{{URL: "https://example.com/discovery"}, {URL: "https://example.com/read"}},
				FetchedPagesSummary: []ResearchFetchedPage{{URL: "https://example.com/read", Success: true, Excerpt: "Read evidence."}},
				JEV: &JEVResearchTrace{
					Assessment: JEVEvidenceAssessment{Status: tc.status, Sufficient: tc.sufficient, Gaps: []string{"authority"}},
					StopReason: "round_limit",
					Citations:  []string{"https://example.com/read"},
				},
			}
			out := BuildResearchAgentOutput(pack)
			if out.AnswerReadiness != tc.readiness {
				t.Fatalf("readiness = %s, want %s", out.AnswerReadiness, tc.readiness)
			}
			if len(out.SelectedSources) != 1 || out.SelectedSources[0].URL != "https://example.com/read" {
				t.Fatalf("unread source offered for citation: %+v", out.SelectedSources)
			}
			if !strings.Contains(strings.Join(out.Warnings, " "), "stop_reason=round_limit") || len(out.Gaps) == 0 {
				t.Fatalf("judgment details omitted: %+v", out)
			}
			pack.Error = "failed"
			out = BuildResearchAgentOutput(pack)
			if out.Status != "error" || out.AnswerReadiness != "partial" {
				t.Fatalf("error overwritten by judgment: %+v", out)
			}
		})
	}
}

func TestResearchAgentJEVWithoutReadSourcesDoesNotSuggestCitations(t *testing.T) {
	out := BuildResearchAgentOutput(ResearchPack{
		HighSignalSources: []ResearchSource{{URL: "https://example.com/discovery"}},
		JEV:               &JEVResearchTrace{Assessment: JEVEvidenceAssessment{Status: "unknown"}},
	})
	if len(out.SelectedSources) != 0 || strings.Contains(strings.Join(out.NextActions, " "), "cite source ids") {
		t.Fatalf("unread citations suggested: %+v", out)
	}
}

func TestResearchAgentJEVKeepsRedirectedAndUnrankedCitations(t *testing.T) {
	pack := ResearchPack{
		HighSignalSources: []ResearchSource{
			{URL: "https://example.com/original"},
			{URL: "https://example.com/discovery2"},
			{URL: "https://example.com/discovery3"},
			{URL: "https://example.com/discovery4"},
			{URL: "https://example.com/discovery5"},
			{URL: "https://example.com/read#ranking", Domain: "example.com", Score: 7, Reasons: []string{"ranked evidence"}},
		},
		FetchedPagesSummary: []ResearchFetchedPage{
			{URL: "https://example.com/redirect#body", Success: true, Source: "jina", Excerpt: "redirect body"},
			{URL: "https://example.com/read#body", Success: true, Source: "exa", Excerpt: "ranked body"},
			{URL: "https://example.com/unranked", Success: true, Source: "tavily", Excerpt: "unranked body"},
		},
		JEV: &JEVResearchTrace{
			Assessment: JEVEvidenceAssessment{Status: "sufficient", Sufficient: true},
			Citations:  []string{"https://example.com/redirect#citation", "https://example.com/read", "https://example.com/unranked"},
		},
	}
	out := BuildResearchAgentOutput(pack)
	if len(out.SelectedSources) != 3 {
		t.Fatalf("citations lost: %+v", out.SelectedSources)
	}
	for i, want := range []struct{ url, excerpt, provider string }{
		{"https://example.com/redirect", "redirect body", "jina"},
		{"https://example.com/read", "ranked body", "exa"},
		{"https://example.com/unranked", "unranked body", "tavily"},
	} {
		got := out.SelectedSources[i]
		if got.URL != want.url || got.Excerpt != want.excerpt || got.Provider != want.provider || got.Domain != "example.com" {
			t.Errorf("source %d = %+v, want %+v", i, got, want)
		}
	}
	if out.SelectedSources[1].Score != 7 || out.SelectedSources[1].Reason != "ranked evidence" {
		t.Fatalf("ranking metadata lost: %+v", out.SelectedSources[1])
	}
}
