package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/bettas/grok-search-go/internal/engine"
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
		ReasoningEndpoint: "deepseek",
	})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if res.Answer == "" || res.ReasoningEndpoint != "deepseek" || res.ReasoningModel != "deepseek-v4-flash" {
		t.Fatalf("result = %+v", res)
	}
	if researcher.opts.Depth != "quick" {
		t.Fatalf("research opts = %+v", researcher.opts)
	}
	if reasoner.endpoint != "deepseek" {
		t.Fatalf("reasoning endpoint = %q", reasoner.endpoint)
	}
	if !strings.Contains(reasoner.req.UserPrompt, "research_pack") || !strings.Contains(reasoner.req.UserPrompt, "https://example.com/deepseek") {
		t.Fatalf("reasoning prompt missing research evidence: %s", reasoner.req.UserPrompt)
	}
}

func TestSmartAnswererRequiresReasoner(t *testing.T) {
	answerer := &SmartAnswerer{Researcher: &fakeSmartResearcher{}}
	_, err := answerer.Run(context.Background(), SmartAnswerOptions{Query: "hello"})
	if err == nil || !strings.Contains(err.Error(), "reasoning endpoint is not configured") {
		t.Fatalf("err = %v", err)
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
