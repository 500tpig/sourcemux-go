package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/bettas/grok-search-go/internal/engine"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

const smartAnswerEvidenceMaxChars = 48000

// SmartAnswerOptions controls the evidence-gathering plus reasoning workflow.
type SmartAnswerOptions struct {
	Query             string   `json:"query"`
	Depth             string   `json:"depth,omitempty"`
	Platform          string   `json:"platform,omitempty"`
	Domains           []string `json:"domains,omitempty"`
	MaxFetches        int      `json:"max_fetches,omitempty"`
	ReasoningEndpoint string   `json:"reasoning_endpoint,omitempty"`
	ReasoningModel    string   `json:"reasoning_model,omitempty"`
}

// SmartAnswerResult is the stable output envelope for MCP and CLI users.
type SmartAnswerResult struct {
	Query             string       `json:"query"`
	Answer            string       `json:"answer"`
	ReasoningEndpoint string       `json:"reasoning_endpoint"`
	ReasoningModel    string       `json:"reasoning_model"`
	Research          ResearchPack `json:"research"`
	Error             string       `json:"error,omitempty"`
}

// SmartResearcher is satisfied by ResearchExecutor and by tests.
type SmartResearcher interface {
	Run(ctx context.Context, opts ResearchOptions) (ResearchPack, error)
}

// SmartReasoner is satisfied by engine.ReasoningPool and by tests.
type SmartReasoner interface {
	Complete(ctx context.Context, req engine.ReasoningRequest, endpointName string) (*engine.PoolReasoningResult, error)
}

// SmartAnswerer composes research_run with a final reasoning endpoint.
type SmartAnswerer struct {
	Researcher SmartResearcher
	Reasoner   SmartReasoner
}

// RegisterSmartAnswer registers the smart_answer MCP tool.
func RegisterSmartAnswer(s *mcpserver.MCPServer, answerer *SmartAnswerer) {
	tool := mcp.NewTool("smart_answer",
		mcp.WithDescription("Gather evidence with the existing research workflow, then synthesize a final answer with a configured reasoning endpoint such as DeepSeek V4 Flash/Pro."),
		mcp.WithString("query", mcp.Required(), mcp.Description("Question to answer")),
		mcp.WithString("depth", mcp.Description("Research depth: quick, standard, or deep (default standard)"), mcp.Enum("quick", "standard", "deep")),
		mcp.WithString("platform", mcp.Description("Optional platform focus, e.g. 'GitHub, Reddit'")),
		mcp.WithArray("domains",
			mcp.Description("Optional allow-list of domains or site roots for the research phase"),
			mcp.Items(map[string]any{"type": "string"}),
		),
		mcp.WithNumber("max_fetches", mcp.Description("Maximum ranked URLs to fetch during research")),
		mcp.WithString("reasoning_endpoint", mcp.Description("Optional reasoning endpoint name from reasoningEndpoints")),
		mcp.WithString("reasoning_model", mcp.Description("Optional one-shot model override, e.g. deepseek-v4-pro")),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if answerer == nil {
			return mcp.NewToolResultError("smart_answer is not configured"), nil
		}
		query, _ := req.Params.Arguments["query"].(string)
		endpoint, _ := req.Params.Arguments["reasoning_endpoint"].(string)
		model, _ := req.Params.Arguments["reasoning_model"].(string)
		depth, _ := req.Params.Arguments["depth"].(string)
		platform, _ := req.Params.Arguments["platform"].(string)

		res, err := answerer.Run(ctx, SmartAnswerOptions{
			Query:             query,
			Depth:             depth,
			Platform:          platform,
			Domains:           stringSliceArg(req.Params.Arguments, "domains"),
			MaxFetches:        intArgOr(req.Params.Arguments, "max_fetches", 0),
			ReasoningEndpoint: endpoint,
			ReasoningModel:    model,
		})
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(FormatSmartAnswerResult(res)), nil
	})
}

// Run executes evidence gathering and final synthesis.
func (a *SmartAnswerer) Run(ctx context.Context, opts SmartAnswerOptions) (SmartAnswerResult, error) {
	query := strings.TrimSpace(opts.Query)
	if query == "" {
		return SmartAnswerResult{}, fmt.Errorf("query is required")
	}
	if a == nil || a.Researcher == nil {
		return SmartAnswerResult{Query: query}, fmt.Errorf("researcher is not configured")
	}
	if a.Reasoner == nil {
		return SmartAnswerResult{Query: query}, fmt.Errorf("reasoning endpoint is not configured")
	}

	pack, err := a.Researcher.Run(ctx, ResearchOptions{
		Query:      query,
		Depth:      opts.Depth,
		Platform:   opts.Platform,
		Domains:    opts.Domains,
		MaxFetches: opts.MaxFetches,
	})
	if err != nil {
		return SmartAnswerResult{Query: query, Research: pack}, fmt.Errorf("research phase failed: %w", err)
	}

	reasoningReq := engine.ReasoningRequest{
		SystemPrompt: smartAnswerSystemPrompt(),
		UserPrompt:   buildSmartAnswerUserPrompt(query, pack),
		Model:        strings.TrimSpace(opts.ReasoningModel),
	}
	reasoningRes, err := a.Reasoner.Complete(ctx, reasoningReq, opts.ReasoningEndpoint)
	if err != nil {
		return SmartAnswerResult{Query: query, Research: pack}, fmt.Errorf("reasoning phase failed: %w", err)
	}

	return SmartAnswerResult{
		Query:             query,
		Answer:            reasoningRes.Content,
		ReasoningEndpoint: reasoningRes.EndpointName,
		ReasoningModel:    reasoningRes.EndpointModel,
		Research:          pack,
	}, nil
}

func smartAnswerSystemPrompt() string {
	return strings.TrimSpace(`
You are an evidence-grounded research synthesizer.

Rules:
- Use the provided research pack as the source of truth for factual and current claims.
- Cite source URLs inline when making source-backed claims.
- If the evidence is weak, stale, or conflicting, say so directly.
- Do not invent facts that are not supported by the research pack.
- Answer in the same language as the user's question unless the user asks otherwise.
- Prefer concise, actionable output.
`)
}

func buildSmartAnswerUserPrompt(query string, pack ResearchPack) string {
	evidence := FormatResearchPack(pack)
	if len(evidence) > smartAnswerEvidenceMaxChars {
		evidence = evidence[:smartAnswerEvidenceMaxChars] + "\n\n[research pack clipped for model context]"
	}
	return fmt.Sprintf(`Question:
%s

Research pack:
%s

Task:
Synthesize the final answer. Include concrete next steps when the user is asking what to do.`, query, evidence)
}

// FormatSmartAnswerResult renders a compact LLM-readable output.
func FormatSmartAnswerResult(res SmartAnswerResult) string {
	var sb strings.Builder
	sb.WriteString("smart_answer\n")
	fmt.Fprintf(&sb, "query: %s\n", res.Query)
	fmt.Fprintf(&sb, "reasoning: %s (%s)\n", res.ReasoningEndpoint, res.ReasoningModel)
	fmt.Fprintf(&sb, "research_depth: %s\n", res.Research.EffectiveDepth)
	fmt.Fprintf(&sb, "sources_count: %d\n", res.Research.SourceSummary.UniqueURLs)
	if res.Error != "" {
		fmt.Fprintf(&sb, "error: %s\n", res.Error)
	}
	sb.WriteString("\nanswer:\n")
	sb.WriteString(strings.TrimSpace(res.Answer))
	if len(res.Research.HighSignalSources) > 0 {
		sb.WriteString("\n\nhigh_signal_sources:\n")
		limit := len(res.Research.HighSignalSources)
		if limit > 8 {
			limit = 8
		}
		for _, source := range res.Research.HighSignalSources[:limit] {
			fmt.Fprintf(&sb, "- %s\n", source.URL)
		}
	}
	return strings.TrimSpace(sb.String())
}
