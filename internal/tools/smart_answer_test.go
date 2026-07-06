package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/500tpig/sourcemux-go/internal/engine"
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
	opts ResearchOptions
	pack ResearchPack
	err  error
}

func (f *fakeSmartResearcher) Run(ctx context.Context, opts ResearchOptions) (ResearchPack, error) {
	f.opts = opts
	return f.pack, f.err
}

type fakeSmartReasoner struct {
	req      engine.ReasoningRequest
	endpoint string
	result   *engine.PoolReasoningResult
	err      error
}

func (f *fakeSmartReasoner) Complete(ctx context.Context, req engine.ReasoningRequest, endpointName string) (*engine.PoolReasoningResult, error) {
	f.req = req
	f.endpoint = endpointName
	return f.result, f.err
}
