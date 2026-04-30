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
	searchCount := map[string]int{
		"quick":    2,
		"standard": 4,
		"deep":     7,
	}[depth]
	fetchCount := map[string]int{
		"quick":    2,
		"standard": 4,
		"deep":     8,
	}[depth]

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
