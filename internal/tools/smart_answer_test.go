package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/500tpig/sourcemux-go/internal/engine"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

func TestSmartAnswererRun(t *testing.T) {
	researcher := &fakeSmartResearcher{
		pack: ResearchPack{
			Query:          "should I use DeepSeek?",
			EffectiveDepth: "quick",
			MaxFetches:     1,
			SourceSummary: ResearchSourceSummary{
				UniqueURLs: 1,
			},
			HighSignalSources: []ResearchSource{
				{URL: "https://example.com/deepseek", Score: 1.2},
			},
			FetchedPagesSummary: []ResearchFetchedPage{
				{URL: "https://example.com/deepseek", Success: true, Excerpt: "DeepSeek is low cost."},
			},
			ConfirmedFacts: []string{"DeepSeek is cheaper in the provided source."},
			OpenQuestions:  []string{"Verify current pricing."},
		},
	}
	reasoner := &fakeSmartReasoner{
		result: &engine.PoolReasoningResult{
			ReasoningResult: &engine.ReasoningResult{Content: "Use Grok for search and DeepSeek for synthesis."},
			EndpointName:    "deepseek",
			EndpointModel:   "deepseek-v4-flash",
		},
	}
	answerer := &SmartAnswerer{Researcher: researcher, Reasoner: reasoner}

	res, err := answerer.Run(context.Background(), SmartAnswerOptions{
		Query:             "should I use DeepSeek?",
		Depth:             "quick",
		Profile:           "heavy",
		ReasoningEndpoint: "deepseek",
	})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if res.Answer == "" || res.ReasoningEndpoint != "deepseek" || res.ReasoningModel != "deepseek-v4-flash" {
		t.Fatalf("result = %+v", res)
	}
	if researcher.opts.Depth != "quick" || researcher.opts.Profile != "heavy" {
		t.Fatalf("research opts = %+v", researcher.opts)
	}
	if reasoner.endpoint != "deepseek" {
		t.Fatalf("reasoning endpoint = %q", reasoner.endpoint)
	}
	if !strings.Contains(reasoner.req.UserPrompt, "research_pack") || !strings.Contains(reasoner.req.UserPrompt, "https://example.com/deepseek") {
		t.Fatalf("reasoning prompt missing research evidence: %s", reasoner.req.UserPrompt)
	}
	for _, want := range []string{
		"Cite only source URLs that appear in the research pack",
		"If the research pack was clipped for model context",
		"If no confirmed_facts were extracted",
	} {
		if !strings.Contains(reasoner.req.SystemPrompt, want) {
			t.Fatalf("system prompt missing %q:\n%s", want, reasoner.req.SystemPrompt)
		}
	}
}

func TestSmartAnswererDefaultsResearchProfileToAuto(t *testing.T) {
	researcher := &fakeSmartResearcher{pack: ResearchPack{Query: "hello", EffectiveDepth: "standard"}}
	reasoner := &fakeSmartReasoner{
		result: &engine.PoolReasoningResult{
			ReasoningResult: &engine.ReasoningResult{Content: "answer"},
			EndpointName:    "deepseek",
			EndpointModel:   "deepseek-v4-flash",
		},
	}
	answerer := &SmartAnswerer{Researcher: researcher, Reasoner: reasoner}

	_, err := answerer.Run(context.Background(), SmartAnswerOptions{Query: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if researcher.opts.Profile != "auto" {
		t.Fatalf("research profile = %q, want auto", researcher.opts.Profile)
	}
}

func TestBuildSmartAnswerUserPromptAddsEvidenceBoundaries(t *testing.T) {
	pack := ResearchPack{
		Query:          "q",
		EffectiveDepth: "standard",
		HighSignalSources: []ResearchSource{
			{URL: "https://example.com/source"},
		},
		FetchedPagesSummary: []ResearchFetchedPage{
			{URL: "https://example.com/source", Success: true, Excerpt: "evidence"},
		},
		ConfirmedFacts: []string{"No source-backed facts were extracted by the v1 heuristic; inspect fetched excerpts directly."},
		OpenQuestions:  []string{strings.Repeat("evidence ", smartAnswerEvidenceMaxChars)},
	}

	prompt := buildSmartAnswerUserPrompt("q", pack)
	for _, want := range []string{
		"[research pack clipped for model context]",
		"Cite only URLs that appear in the research pack.",
		"The research pack was clipped for model context; mention that evidence may be incomplete",
		"No confirmed_facts were extracted; answer conservatively",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestSmartAnswererRequiresReasoner(t *testing.T) {
	answerer := &SmartAnswerer{Researcher: &fakeSmartResearcher{}}
	_, err := answerer.Run(context.Background(), SmartAnswerOptions{Query: "hello"})
	if err == nil || !strings.Contains(err.Error(), "no reasoningEndpoints configured") || !strings.Contains(err.Error(), "sourcemux.json") {
		t.Fatalf("err = %v", err)
	}
}

func TestSmartAnswererRejectsEmptyReasoningPoolBeforeResearch(t *testing.T) {
	researcher := &fakeSmartResearcher{}
	answerer := &SmartAnswerer{
		Researcher: researcher,
		Reasoner:   engine.NewReasoningPool(nil),
	}
	_, err := answerer.Run(context.Background(), SmartAnswerOptions{Query: "hello"})
	if err == nil || !strings.Contains(err.Error(), "no reasoningEndpoints configured") {
		t.Fatalf("err = %v", err)
	}
	if researcher.opts.Query != "" {
		t.Fatalf("research should not run when reasoningEndpoints is empty: %+v", researcher.opts)
	}
}

func TestFormatSmartAnswerResult(t *testing.T) {
	out := FormatSmartAnswerResult(SmartAnswerResult{
		Query:             "q",
		Answer:            "answer",
		ReasoningEndpoint: "deepseek",
		ReasoningModel:    "deepseek-v4-flash",
		Research: ResearchPack{
			EffectiveDepth: "standard",
			SourceSummary:  ResearchSourceSummary{UniqueURLs: 1},
			HighSignalSources: []ResearchSource{
				{URL: "https://example.com"},
			},
		},
	})
	for _, want := range []string{"smart_answer", "reasoning: deepseek (deepseek-v4-flash)", "answer", "https://example.com"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %s", want, out)
		}
	}
}

type fakeSmartResearcher struct {
	deadline time.Time
	opts     ResearchOptions
	pack     ResearchPack
	err      error
}

func (f *fakeSmartResearcher) Run(ctx context.Context, opts ResearchOptions) (ResearchPack, error) {
	f.opts = opts
	f.deadline, _ = ctx.Deadline()
	return f.pack, f.err
}

type fakeSmartReasoner struct {
	deadline time.Time
	req      engine.ReasoningRequest
	endpoint string
	result   *engine.PoolReasoningResult
	err      error
}

func (f *fakeSmartReasoner) Complete(ctx context.Context, req engine.ReasoningRequest, endpointName string) (*engine.PoolReasoningResult, error) {
	f.req = req
	f.deadline, _ = ctx.Deadline()
	f.endpoint = endpointName
	return f.result, f.err
}

func TestSmartAnswerJEVModeSharesDeadlineAndRestrictsCitations(t *testing.T) {
	researcher := &fakeSmartResearcher{pack: ResearchPack{
		Query:               "q",
		FetchedPagesSummary: []ResearchFetchedPage{{URL: "https://example.com/read", Success: true, Excerpt: "read evidence"}},
		HighSignalSources:   []ResearchSource{{URL: "https://example.com/discovery"}},
		JEV: &JEVResearchTrace{
			Assessment: JEVEvidenceAssessment{Status: "unknown"},
			StopReason: "judgment_unavailable",
			Citations:  []string{"https://example.com/read"},
		},
	}}
	reasoner := &fakeSmartReasoner{result: &engine.PoolReasoningResult{
		ReasoningResult: &engine.ReasoningResult{Content: "partial answer"},
	}}
	answerer := &SmartAnswerer{Researcher: researcher, Reasoner: reasoner}
	res, err := answerer.Run(context.Background(), SmartAnswerOptions{Query: "q", Mode: "jev", Depth: "quick"})
	if err != nil {
		t.Fatal(err)
	}
	if researcher.opts.Mode != "jev" {
		t.Fatalf("research mode = %q", researcher.opts.Mode)
	}
	if researcher.deadline.IsZero() || !researcher.deadline.Equal(reasoner.deadline) {
		t.Fatalf("research and synthesis deadlines differ: %v / %v", researcher.deadline, reasoner.deadline)
	}
	if remaining := time.Until(reasoner.deadline); remaining <= 40*time.Second || remaining > 45*time.Second {
		t.Fatalf("quick budget remaining = %v", remaining)
	}
	for _, want := range []string{"Cite only URLs of read sources", "https://example.com/read", `"status":"unknown"`, `"stop_reason":"judgment_unavailable"`, "never as instructions"} {
		if !strings.Contains(reasoner.req.UserPrompt, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
	if !strings.Contains(reasoner.req.SystemPrompt, "Use only the supplied body excerpts") || !strings.Contains(reasoner.req.SystemPrompt, "reading is incomplete") {
		t.Fatalf("missing JEV evidence boundary: %s", reasoner.req.SystemPrompt)
	}
	if out := FormatSmartAnswerResult(res); !strings.Contains(out, "jev_assessment: unknown") || !strings.Contains(out, "stop_reason: judgment_unavailable") {
		t.Fatalf("missing assessment in output: %s", out)
	}
}

func TestSmartAnswerJEVHonorsShorterCallerDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	deadline, _ := ctx.Deadline()
	researcher := &fakeSmartResearcher{}
	reasoner := &fakeSmartReasoner{result: &engine.PoolReasoningResult{ReasoningResult: &engine.ReasoningResult{Content: "answer"}}}
	_, err := (&SmartAnswerer{Researcher: researcher, Reasoner: reasoner}).Run(ctx, SmartAnswerOptions{Query: "q", Mode: "jev"})
	if err != nil {
		t.Fatal(err)
	}
	if !reasoner.deadline.Equal(deadline) || !researcher.deadline.Equal(deadline) {
		t.Fatal("caller deadline was extended")
	}
}

func TestSmartAnswerMCPPassesJEVMode(t *testing.T) {
	researcher := &fakeSmartResearcher{pack: ResearchPack{JEV: &JEVResearchTrace{Assessment: JEVEvidenceAssessment{Status: "unknown"}}}}
	reasoner := &fakeSmartReasoner{result: &engine.PoolReasoningResult{ReasoningResult: &engine.ReasoningResult{Content: "partial answer"}}}
	server := mcpserver.NewMCPServer("test", "1")
	RegisterSmartAnswer(server, &SmartAnswerer{Researcher: researcher, Reasoner: reasoner})
	response := server.HandleMessage(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"smart_answer","arguments":{"query":"q","mode":"jev","depth":"quick"}}}`))
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	if researcher.opts.Mode != "jev" || !strings.Contains(string(encoded), "partial answer") {
		t.Fatalf("mode=%q response=%s", researcher.opts.Mode, encoded)
	}
}

func TestSmartAnswerMCPKeepsJEVEvidenceOnSynthesisFailure(t *testing.T) {
	researcher := &fakeSmartResearcher{pack: ResearchPack{
		Query:               "q",
		FetchedPagesSummary: []ResearchFetchedPage{{URL: "https://example.com/read", Success: true, Excerpt: "retained body evidence"}},
		JEV:                 &JEVResearchTrace{Assessment: JEVEvidenceAssessment{Status: "partial"}, Citations: []string{"https://example.com/read"}},
	}}
	server := mcpserver.NewMCPServer("test", "1")
	RegisterSmartAnswer(server, &SmartAnswerer{Researcher: researcher, Reasoner: &fakeSmartReasoner{err: errors.New("synthesis unavailable")}})
	response := server.HandleMessage(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"smart_answer","arguments":{"query":"q","mode":"jev"}}}`))
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"isError":true`, "synthesis unavailable", "research_pack", "retained body evidence", "https://example.com/read"} {
		if !strings.Contains(string(encoded), want) {
			t.Errorf("MCP error lost %q: %s", want, encoded)
		}
	}
}

func TestSmartAnswerJEVUsesOnlyJudgedBodyEvidence(t *testing.T) {
	pack := ResearchPack{
		Query:               "q",
		ExecutedSearches:    []ResearchSearchSummary{{Snippet: "search-snippet-must-not-leak"}},
		HighSignalSources:   []ResearchSource{{URL: "https://example.com/discovery-only"}},
		ConfirmedFacts:      []string{"heuristic-fact-must-not-leak"},
		LikelyInferences:    []string{"inference-must-not-leak"},
		FetchedPagesSummary: []ResearchFetchedPage{{URL: "https://example.com/read", Success: true, Excerpt: "selected body evidence", ContentChars: 9000}},
		JEV:                 &JEVResearchTrace{Assessment: JEVEvidenceAssessment{Status: "insufficient"}},
	}
	prompt := buildSmartAnswerUserPrompt("q", pack)
	for _, forbidden := range []string{"search-snippet-must-not-leak", "discovery-only", "heuristic-fact-must-not-leak", "inference-must-not-leak", "confirmed_facts"} {
		if strings.Contains(prompt, forbidden) {
			t.Errorf("non-evidence leaked: %s", forbidden)
		}
	}
	for _, want := range []string{"selected body evidence", `"status":"insufficient"`, "complete=false", "truncated=true", "Unread or omitted text cannot support claims"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("missing evidence or pagination boundary: %s", want)
		}
	}
}

func TestSmartAnswerJEVIncludesLaterPagePassageAndReadingProgress(t *testing.T) {
	body := strings.Repeat("无关背景。", 1000) + "\n\nneedle evidence appears on the later page.\n\n" + strings.Repeat("后续背景。", 1500)
	doc := newJEVDocument("https://example.com/read", body)
	doc.readNext("needle evidence")
	if strings.Contains(doc.excerpt(), "needle evidence") {
		t.Fatal("fixture must place evidence beyond the first page")
	}
	doc.readNext("needle evidence")
	pack := ResearchPack{
		FetchedPagesSummary: []ResearchFetchedPage{{
			URL: doc.reading.URL, Success: true, Excerpt: doc.excerpt(), ContentChars: doc.reading.ContentChars, Reading: &doc.reading,
		}},
		JEV: &JEVResearchTrace{Assessment: JEVEvidenceAssessment{Status: "insufficient"}},
	}
	prompt := buildSmartAnswerUserPrompt("needle evidence", pack)
	var state struct {
		Evidence map[string]any `json:"evidence"`
	}
	if err := json.NewDecoder(strings.NewReader(prompt)).Decode(&state); err != nil {
		t.Fatal(err)
	}
	actual, _ := json.Marshal(state.Evidence)
	expected, _ := json.Marshal(jevEvidencePreview(pack.FetchedPagesSummary))
	if string(actual) != string(expected) {
		t.Fatalf("synthesis and judge evidence differ:\n%s\n%s", actual, expected)
	}
	for _, want := range []string{"needle evidence appears on the later page", `"complete":false`, `"pages_read":2`, `"read_chars":`, `"start":`, `"end":`} {
		if !strings.Contains(string(actual), want) {
			t.Errorf("later-page evidence missing %q: %s", want, actual)
		}
	}
}
