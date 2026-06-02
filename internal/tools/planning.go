package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// RegisterSearchPlanning registers a deterministic planning helper. It does
// not call any network service; it returns a compact multi-step search plan the
// caller can execute with web_search/get_sources/web_fetch.
func RegisterSearchPlanning(s *mcpserver.MCPServer) {
	tool := mcp.NewTool("search_planning",
		mcp.WithDescription("Create a staged search plan for complex research before running web_search/web_fetch."),
		mcp.WithString("query", mcp.Required(), mcp.Description("Research question or topic")),
		mcp.WithString("depth", mcp.Description("Research depth: quick, standard, or deep (default standard)")),
		mcp.WithString("platform", mcp.Description("Optional platform focus, e.g. 'GitHub, Reddit'")),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query, _ := req.Params.Arguments["query"].(string)
		if strings.TrimSpace(query) == "" {
			return mcp.NewToolResultError("query is required"), nil
		}
		depth, _ := req.Params.Arguments["depth"].(string)
		platform, _ := req.Params.Arguments["platform"].(string)
		return mcp.NewToolResultText(BuildSearchPlan(query, depth, platform)), nil
	})
}

// BuildSearchPlan renders a deterministic, multi-step search plan from a
// research query. It is exported so non-MCP entrypoints (such as the cli
// package) can reuse the exact same planner the search_planning MCP tool
// emits — pure logic, no network calls.
func BuildSearchPlan(query, depth, platform string) string {
	query = strings.TrimSpace(query)
	depth = normalizeDepth(depth)
	searchCount, fetchCount := planCounts(depth)

	var sb strings.Builder
	fmt.Fprintf(&sb, "search_plan\nquery: %s\ndepth: %s\n", query, depth)
	if strings.TrimSpace(platform) != "" {
		fmt.Fprintf(&sb, "platform_focus: %s\n", strings.TrimSpace(platform))
	}
	sb.WriteString("\nobjectives:\n")
	sb.WriteString("- Find current, source-backed information rather than relying on model memory.\n")
	sb.WriteString("- Collect multiple independent sources and fetch primary/high-signal pages.\n")
	sb.WriteString("- Separate confirmed facts from uncertain or conflicting claims.\n")

	sb.WriteString("\nsearches:\n")
	for i, q := range plannedQueries(query, platform, searchCount) {
		fmt.Fprintf(&sb, "%d. web_search query=%q", i+1, q)
		if strings.TrimSpace(platform) != "" {
			fmt.Fprintf(&sb, " platform=%q", strings.TrimSpace(platform))
		}
		sb.WriteString("\n")
	}

	fmt.Fprintf(&sb, "\nsource_workflow:\n")
	sb.WriteString("1. For each useful web_search result, call get_sources(session_id).\n")
	fmt.Fprintf(&sb, "2. Fetch the top %d primary/high-signal URLs with web_fetch.\n", fetchCount)
	sb.WriteString("3. Prefer official docs, primary repos, papers, changelogs, standards, and direct announcements.\n")
	sb.WriteString("4. Cross-check dates, authorship, and whether pages are primary or commentary.\n")

	sb.WriteString("\nfinal_answer_checklist:\n")
	sb.WriteString("- Cite the strongest URLs.\n")
	sb.WriteString("- State what is confirmed, what is inferred, and what remains uncertain.\n")
	sb.WriteString("- Use exact dates for time-sensitive claims.\n")
	if depth == "deep" {
		sb.WriteString("- Include contradictions or source disagreements if found.\n")
		sb.WriteString("- Run one final targeted web_search for gaps discovered during synthesis.\n")
	}
	return sb.String()
}

// StructuredResearchPlan is the stable JSON output for `sourcemux plan --json`.
// It is deterministic and offline: it only decomposes the query into SourceMux
// capabilities that already exist, and does not inspect config or call network
// providers.
type StructuredResearchPlan struct {
	Mode              string                  `json:"mode"`
	Query             string                  `json:"query"`
	Depth             string                  `json:"depth"`
	Platform          string                  `json:"platform,omitempty"`
	IntentSignals     []string                `json:"intent_signals"`
	Decomposition     []string                `json:"decomposition"`
	CapabilityPlan    PlanCapabilityPlan      `json:"capability_plan"`
	Steps             []StructuredPlanStep    `json:"steps"`
	EvidencePolicy    string                  `json:"evidence_policy"`
	GapCheck          PlanGapCheck            `json:"gap_check"`
	FinalAnswerPolicy PlanFinalAnswerPolicy   `json:"final_answer_policy"`
	ProfilePolicy     PlanSearchProfilePolicy `json:"profile_policy"`
}

type PlanCapabilityPlan struct {
	AllowedCapabilities []string `json:"allowed_capabilities"`
	PrimaryRoute        []string `json:"primary_route"`
	DirectExecution     string   `json:"direct_execution"`
}

type StructuredPlanStep struct {
	ID         string   `json:"id"`
	Capability string   `json:"capability"`
	Purpose    string   `json:"purpose"`
	Query      string   `json:"query,omitempty"`
	Input      string   `json:"input,omitempty"`
	Depth      string   `json:"depth,omitempty"`
	Profile    string   `json:"profile,omitempty"`
	SearchType string   `json:"search_type,omitempty"`
	MaxItems   int      `json:"max_items,omitempty"`
	DependsOn  []string `json:"depends_on,omitempty"`
}

type PlanGapCheck struct {
	Required   bool     `json:"required"`
	Capability string   `json:"capability"`
	Method     string   `json:"method"`
	Queries    []string `json:"queries"`
	StopWhen   []string `json:"stop_when"`
}

type PlanFinalAnswerPolicy struct {
	UseFetchedEvidence          bool `json:"use_fetched_evidence"`
	CiteStrongestSources        bool `json:"cite_strongest_sources"`
	UseExactDates               bool `json:"use_exact_dates"`
	SeparateFactsAndInferences  bool `json:"separate_facts_and_inferences"`
	SurfaceContradictions       bool `json:"surface_contradictions"`
	CallOutRemainingUncertainty bool `json:"call_out_remaining_uncertainty"`
}

type PlanSearchProfilePolicy struct {
	DefaultProfile       string   `json:"default_profile"`
	PlannedProfile       string   `json:"planned_profile"`
	HeavyWhen            []string `json:"heavy_when"`
	HeavyIntentSignals   []string `json:"heavy_intent_signals,omitempty"`
	EffectiveIfAvailable string   `json:"effective_if_available"`
	FallbackProfile      string   `json:"fallback_profile"`
	Reason               string   `json:"reason"`
}

// BuildStructuredSearchPlan returns the JSON planner surface for complex
// research. It intentionally mirrors BuildSearchPlan's query generation while
// adding stable routing, profile, evidence, and gap-check policy.
func BuildStructuredSearchPlan(query, depth, platform string) StructuredResearchPlan {
	query = strings.TrimSpace(query)
	depth = normalizeDepth(depth)
	platform = strings.TrimSpace(platform)
	searchCount, fetchCount := planCounts(depth)
	signals := detectIntentSignals(query, depth, platform)
	queries := plannedQueries(query, platform, searchCount)
	stepIDs := make([]string, 0, len(queries))
	steps := make([]StructuredPlanStep, 0, len(queries)+2)
	for i, q := range queries {
		id := fmt.Sprintf("s%d", i+1)
		stepIDs = append(stepIDs, id)
		step := structuredSearchStep(id, q, depth, signals)
		if i == 0 {
			step.Capability = "search"
			step.Purpose = "Start with SourceMux search for current/source discovery and fallback tracing."
			step.Profile = SearchProfileAuto
			step.SearchType = ""
		}
		steps = append(steps, step)
	}
	steps = append(steps, StructuredPlanStep{
		ID:         "fetch-key-sources",
		Capability: "fetch",
		Purpose:    "Fetch primary and high-signal URLs before making source-critical claims.",
		Input:      "selected_source_urls_from_previous_steps",
		MaxItems:   fetchCount,
		DependsOn:  append([]string(nil), stepIDs...),
	})
	gapQueries := []string{
		query + " contradictions unresolved questions",
		query + " limitations failure modes",
	}
	if depth == "deep" {
		steps = append(steps, StructuredPlanStep{
			ID:         "gap-search",
			Capability: "search",
			Purpose:    "Run a final targeted search for contradictions, recent updates, and remaining gaps.",
			Query:      gapQueries[0],
			Profile:    SearchProfileAuto,
			DependsOn:  []string{"fetch-key-sources"},
		})
	}
	return StructuredResearchPlan{
		Mode:          "deep_research_plan",
		Query:         query,
		Depth:         depth,
		Platform:      platform,
		IntentSignals: signals,
		Decomposition: planDecomposition(signals),
		CapabilityPlan: PlanCapabilityPlan{
			AllowedCapabilities: []string{"search", "docs-search", "exa-search", "exa-contents", "fetch", "map", "crawl", "research", "smart-answer"},
			PrimaryRoute:        primaryRoute(depth, signals),
			DirectExecution:     fmt.Sprintf("research --depth %s --profile auto --json", depth),
		},
		Steps:          steps,
		EvidencePolicy: "fetch_before_claim",
		GapCheck: PlanGapCheck{
			Required:   depth == "deep" || hasAnySignal(signals, "current", "comparison", "high-risk", "source-critical"),
			Capability: "search",
			Method:     "After fetching key sources, search for contradictions, newer updates, and missing primary evidence.",
			Queries:    gapQueries,
			StopWhen: []string{
				"primary sources cover the main claim",
				"dates and version context are confirmed",
				"remaining uncertainty is explicitly listed",
			},
		},
		FinalAnswerPolicy: PlanFinalAnswerPolicy{
			UseFetchedEvidence:          true,
			CiteStrongestSources:        true,
			UseExactDates:               true,
			SeparateFactsAndInferences:  true,
			SurfaceContradictions:       depth == "deep" || hasAnySignal(signals, "comparison", "high-risk"),
			CallOutRemainingUncertainty: true,
		},
		ProfilePolicy: profilePolicyForPlan(signals),
	}
}

func normalizeDepth(depth string) string {
	switch strings.ToLower(strings.TrimSpace(depth)) {
	case "quick", "standard", "deep":
		return strings.ToLower(strings.TrimSpace(depth))
	default:
		return "standard"
	}
}

func plannedQueries(query, platform string, limit int) []string {
	base := strings.TrimSpace(query)
	focus := strings.TrimSpace(platform)
	queries := []string{
		base,
		base + " official documentation OR announcement",
		base + " latest updates changelog release notes",
		base + " comparison analysis limitations",
		base + " site:github.com issues discussion",
		base + " site:reddit.com OR forum",
		base + " benchmark evaluation source",
	}
	if focus != "" {
		queries = append([]string{base + " " + focus}, queries...)
	}
	if limit > len(queries) {
		limit = len(queries)
	}
	return queries[:limit]
}

func planCounts(depth string) (int, int) {
	switch depth {
	case "quick":
		return 2, 2
	case "deep":
		return 7, 8
	default:
		return 4, 4
	}
}

func structuredSearchStep(id, query, depth string, signals []string) StructuredPlanStep {
	lower := strings.ToLower(query)
	step := StructuredPlanStep{
		ID:         id,
		Capability: "search",
		Purpose:    "Discover current source URLs and independent coverage.",
		Query:      query,
		Profile:    SearchProfileAuto,
	}
	switch {
	case strings.Contains(lower, "official documentation"):
		step.Capability = "docs-search"
		step.Purpose = "Find official docs, changelogs, release notes, or direct announcements."
		step.Profile = ""
	case depth == "deep" && (strings.Contains(lower, "comparison") || strings.Contains(lower, "benchmark") || hasAnySignal(signals, "comparison")):
		step.Capability = "exa-search"
		step.Purpose = "Find low-noise source material for comparison, evaluation, and limitations."
		step.Profile = ""
		step.SearchType = "deep"
	}
	return step
}

func detectIntentSignals(query, depth, platform string) []string {
	q := strings.ToLower(strings.Join([]string{query, platform}, " "))
	signals := make([]string, 0, 6)
	if depth == "deep" {
		signals = append(signals, "deep")
	}
	if strings.TrimSpace(platform) != "" {
		signals = append(signals, "platform-focus")
	}
	for _, candidate := range []struct {
		signal  string
		needles []string
	}{
		{"current", []string{"current", "latest", "recent", "today", "yesterday", "this week", "2026", "release", "changelog", "\u6700\u65b0", "\u8fd1\u671f", "\u5f53\u524d"}},
		{"comparison", []string{"compare", "comparison", " versus ", " vs ", "tradeoff", "alternative", "better", "\u5bf9\u6bd4", "\u6bd4\u8f83"}},
		{"high-risk", []string{"high-risk", "risk", "security", "legal", "financial", "medical", "compliance", "\u5b89\u5168", "\u6cd5\u5f8b", "\u91d1\u878d", "\u533b\u7597", "\u98ce\u9669"}},
		{"docs", []string{"docs", "documentation", "api", "sdk", "framework", "library", "official docs", "\u6587\u6863", "\u5b98\u65b9"}},
		{"source-critical", []string{"verify", "evidence", "citation", "source-critical", "source backed", "source-backed", " sources", " source ", "claim", "audit", "\u6838\u9a8c", "\u8bc1\u636e", "\u6765\u6e90"}},
	} {
		if containsAny(q, candidate.needles) {
			signals = append(signals, candidate.signal)
		}
	}
	if len(signals) == 0 {
		return []string{"general-research"}
	}
	return signals
}

func planDecomposition(signals []string) []string {
	items := []string{
		"Clarify the exact question, decision, or claim to verify.",
		"Discover current and primary sources before relying on summaries.",
		"Fetch the strongest URLs before making source-critical claims.",
		"Separate confirmed facts, inferences, contradictions, and open gaps.",
	}
	if hasAnySignal(signals, "comparison") {
		items = append(items, "Compare options on criteria, tradeoffs, limitations, and recency.")
	}
	if hasAnySignal(signals, "high-risk") {
		items = append(items, "Prefer authoritative primary sources and mark advice-sensitive uncertainty.")
	}
	return items
}

func primaryRoute(depth string, signals []string) []string {
	route := []string{"search", "fetch"}
	if hasAnySignal(signals, "docs") {
		route = []string{"docs-search", "search", "fetch"}
	}
	if depth == "deep" || hasAnySignal(signals, "comparison", "source-critical") {
		route = append(route[:len(route)-1], "exa-search", "fetch")
	}
	return route
}

func profilePolicyForPlan(signals []string) PlanSearchProfilePolicy {
	heavySignals := matchingSignals(signals, "deep", "current", "comparison", "high-risk", "source-critical")
	reason := "profile=auto keeps routine planning on the configured default profile unless heavy intent is detected"
	effective := "default"
	if len(heavySignals) > 0 {
		reason = "profile=auto can resolve to heavy for deep/current/comparison/high-risk/source-critical intent when a heavy Grok profile is configured"
		effective = "heavy"
	}
	return PlanSearchProfilePolicy{
		DefaultProfile:       SearchProfileAuto,
		PlannedProfile:       SearchProfileAuto,
		HeavyWhen:            []string{"deep", "current", "comparison", "high-risk", "source-critical"},
		HeavyIntentSignals:   heavySignals,
		EffectiveIfAvailable: effective,
		FallbackProfile:      "default",
		Reason:               reason,
	}
}

func containsAny(s string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}

func hasAnySignal(signals []string, wants ...string) bool {
	return len(matchingSignals(signals, wants...)) > 0
}

func matchingSignals(signals []string, wants ...string) []string {
	wantSet := make(map[string]struct{}, len(wants))
	for _, want := range wants {
		wantSet[want] = struct{}{}
	}
	matches := make([]string, 0, len(signals))
	for _, signal := range signals {
		if _, ok := wantSet[signal]; ok {
			matches = append(matches, signal)
		}
	}
	return matches
}
