package tools

import (
	"strings"
	"testing"

	"github.com/500tpig/sourcemux-go/internal/engine"
)

func TestResolveSearchProfileDefaultStaysDefault(t *testing.T) {
	pool := engine.NewGrokPool([]engine.GrokEndpoint{
		{Name: "default", BaseURL: "http://default", APIKey: "k", Model: "m"},
		{Name: "heavy", BaseURL: "http://heavy", APIKey: "k", Model: "m", Profile: "heavy"},
	})

	res, err := ResolveSearchProfile(pool, "", SearchProfileContext{Flow: searchProfileFlowSearch, Query: "quick lookup"})
	if err != nil {
		t.Fatal(err)
	}
	if res.RequestedProfile != "default" || res.EffectiveProfile != "default" {
		t.Fatalf("resolution = %+v, want default/default", res)
	}
}

func TestResolveSearchProfileExplicitHeavyRequiresConfiguredProfile(t *testing.T) {
	withHeavy := engine.NewGrokPool([]engine.GrokEndpoint{
		{Name: "heavy", BaseURL: "http://heavy", APIKey: "k", Model: "m", Profile: "heavy"},
	})
	res, err := ResolveSearchProfile(withHeavy, "heavy", SearchProfileContext{Flow: searchProfileFlowSearch})
	if err != nil {
		t.Fatal(err)
	}
	if res.EffectiveProfile != "heavy" {
		t.Fatalf("effective profile = %q, want heavy", res.EffectiveProfile)
	}

	withoutHeavy := engine.NewGrokPool([]engine.GrokEndpoint{
		{Name: "default", BaseURL: "http://default", APIKey: "k", Model: "m"},
	})
	_, err = ResolveSearchProfile(withoutHeavy, "heavy", SearchProfileContext{Flow: searchProfileFlowSearch})
	if err == nil || !strings.Contains(err.Error(), `profile "heavy"`) {
		t.Fatalf("err = %v, want missing heavy profile", err)
	}
}

func TestResolveSearchProfileAutoChoosesHeavyForResearchWhenAvailable(t *testing.T) {
	pool := engine.NewGrokPool([]engine.GrokEndpoint{
		{Name: "default", BaseURL: "http://default", APIKey: "k", Model: "m"},
		{Name: "heavy", BaseURL: "http://heavy", APIKey: "k", Model: "m", Profile: "heavy"},
	})

	res, err := ResolveSearchProfile(pool, "auto", SearchProfileContext{
		Flow:  searchProfileFlowResearch,
		Query: "routine research topic",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.RequestedProfile != "auto" || res.EffectiveProfile != "heavy" || !strings.Contains(res.ProfileReason, "research") {
		t.Fatalf("resolution = %+v, want auto/heavy research reason", res)
	}
}

func TestResolveSearchProfileAutoFallsBackToDefaultWithoutHeavy(t *testing.T) {
	pool := engine.NewGrokPool([]engine.GrokEndpoint{
		{Name: "default", BaseURL: "http://default", APIKey: "k", Model: "m"},
	})

	res, err := ResolveSearchProfile(pool, "auto", SearchProfileContext{
		Flow:  searchProfileFlowResearch,
		Query: "complex current comparison",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.RequestedProfile != "auto" || res.EffectiveProfile != "default" || !strings.Contains(res.ProfileReason, "not configured") {
		t.Fatalf("resolution = %+v, want auto/default missing heavy reason", res)
	}
}

func TestResolveSearchProfileAutoSearchUsesIntentSignals(t *testing.T) {
	pool := engine.NewGrokPool([]engine.GrokEndpoint{
		{Name: "default", BaseURL: "http://default", APIKey: "k", Model: "m"},
		{Name: "heavy", BaseURL: "http://heavy", APIKey: "k", Model: "m", Profile: "heavy"},
	})

	res, err := ResolveSearchProfile(pool, "auto", SearchProfileContext{
		Flow:  searchProfileFlowSearch,
		Query: "complex query comparing current options",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.EffectiveProfile != "heavy" {
		t.Fatalf("effective profile = %q, want heavy", res.EffectiveProfile)
	}

	res, err = ResolveSearchProfile(pool, "auto", SearchProfileContext{
		Flow:  searchProfileFlowSearch,
		Query: "one hop docs lookup",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.EffectiveProfile != "default" {
		t.Fatalf("effective profile = %q, want default", res.EffectiveProfile)
	}
}
