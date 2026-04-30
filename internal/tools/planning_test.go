package tools

import (
	"strings"
	"testing"
)

func TestBuildSearchPlanDeepIncludesWorkflow(t *testing.T) {
	plan := BuildSearchPlan("Grok Search MCP", "deep", "GitHub")
	for _, want := range []string{
		"depth: deep",
		"platform_focus: GitHub",
		"web_search query=",
		"get_sources(session_id)",
		"web_fetch",
		"Run one final targeted web_search",
	} {
		if !strings.Contains(plan, want) {
			t.Fatalf("plan missing %q:\n%s", want, plan)
		}
	}
}

func TestNormalizeDepthDefaultsStandard(t *testing.T) {
	if got := normalizeDepth("unknown"); got != "standard" {
		t.Fatalf("normalizeDepth = %q, want standard", got)
	}
}
