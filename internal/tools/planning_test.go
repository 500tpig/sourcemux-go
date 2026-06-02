package tools

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildSearchPlanDeepIncludesWorkflow(t *testing.T) {
	plan := BuildSearchPlan("SourceMux MCP", "deep", "GitHub")
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

func TestBuildStructuredSearchPlanDeepProfilePolicy(t *testing.T) {
	plan := BuildStructuredSearchPlan("Compare current security risk in SourceMux providers", "deep", "")
	if plan.Mode != "deep_research_plan" {
		t.Fatalf("mode = %q", plan.Mode)
	}
	if plan.EvidencePolicy != "fetch_before_claim" {
		t.Fatalf("evidence_policy = %q", plan.EvidencePolicy)
	}
	if plan.ProfilePolicy.DefaultProfile != SearchProfileAuto || plan.ProfilePolicy.PlannedProfile != SearchProfileAuto {
		t.Fatalf("profile policy = %+v", plan.ProfilePolicy)
	}
	if plan.ProfilePolicy.EffectiveIfAvailable != "heavy" {
		t.Fatalf("effective_if_available = %q, want heavy", plan.ProfilePolicy.EffectiveIfAvailable)
	}
	for _, want := range []string{"deep", "current", "comparison", "high-risk"} {
		if !containsString(plan.IntentSignals, want) {
			t.Fatalf("intent signals missing %q: %+v", want, plan.IntentSignals)
		}
		if !containsString(plan.ProfilePolicy.HeavyIntentSignals, want) {
			t.Fatalf("heavy intent signals missing %q: %+v", want, plan.ProfilePolicy.HeavyIntentSignals)
		}
	}
	if len(plan.Steps) == 0 {
		t.Fatal("expected planned steps")
	}
	allowed := make(map[string]struct{})
	for _, capability := range plan.CapabilityPlan.AllowedCapabilities {
		allowed[capability] = struct{}{}
	}
	for _, step := range plan.Steps {
		if _, ok := allowed[step.Capability]; !ok {
			t.Fatalf("step %s uses unsupported capability %q", step.ID, step.Capability)
		}
		if step.Capability != "search" && step.Profile != "" {
			t.Fatalf("step %s capability %q should not carry search profile %q", step.ID, step.Capability, step.Profile)
		}
	}
	data, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	var decoded StructuredResearchPlan
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Mode != plan.Mode || decoded.Query != plan.Query {
		t.Fatalf("decoded plan = %+v, want mode/query from %+v", decoded, plan)
	}
}

func TestBuildStructuredSearchPlanRoutineUsesAutoDefaultHint(t *testing.T) {
	plan := BuildStructuredSearchPlan("SourceMux overview", "standard", "")
	if plan.ProfilePolicy.PlannedProfile != SearchProfileAuto {
		t.Fatalf("planned profile = %q", plan.ProfilePolicy.PlannedProfile)
	}
	if plan.ProfilePolicy.EffectiveIfAvailable != "default" {
		t.Fatalf("effective_if_available = %q, want default", plan.ProfilePolicy.EffectiveIfAvailable)
	}
	if !containsString(plan.IntentSignals, "general-research") {
		t.Fatalf("intent signals = %+v", plan.IntentSignals)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
