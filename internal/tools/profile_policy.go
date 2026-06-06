package tools

import (
	"fmt"
	"strings"

	"github.com/500tpig/sourcemux-go/internal/config"
	"github.com/500tpig/sourcemux-go/internal/engine"
)

const (
	SearchProfileAuto = config.SearchProfileAuto

	searchProfileFlowSearch   = "search"
	searchProfileFlowResearch = "research"
)

// SearchProfileContext describes the caller intent used by profile=auto.
type SearchProfileContext struct {
	Flow         string
	Depth        string
	Query        string
	Platform     string
	SearchPolicy config.SearchPolicy
}

// SearchProfileResolution records both user intent and the concrete Grok pool
// profile used for this request.
type SearchProfileResolution struct {
	RequestedProfile string `json:"requested_profile"`
	EffectiveProfile string `json:"effective_profile"`
	ProfileReason    string `json:"profile_reason,omitempty"`
}

// ResolveSearchProfile maps a user-requested profile to the concrete Grok pool
// profile. Empty requested profile follows searchPolicy.defaultProfile.
// Explicit heavy must be configured; auto safely degrades to default.
func ResolveSearchProfile(pool *engine.GrokPool, requested string, ctx SearchProfileContext) (SearchProfileResolution, error) {
	policy := effectiveSearchPolicy(ctx.SearchPolicy)
	requested = normalizeSearchProfile(requested)
	if requested == "" {
		requested = policy.DefaultProfile
	}

	switch requested {
	case engine.DefaultGrokEndpointProfile:
		return SearchProfileResolution{
			RequestedProfile: requested,
			EffectiveProfile: engine.DefaultGrokEndpointProfile,
			ProfileReason:    "default profile requested",
		}, nil
	case engine.HeavyGrokEndpointProfile:
		if !poolHasProfile(pool, engine.HeavyGrokEndpointProfile) {
			return SearchProfileResolution{
				RequestedProfile: requested,
				EffectiveProfile: engine.HeavyGrokEndpointProfile,
				ProfileReason:    "explicit heavy profile requested but not configured",
			}, fmt.Errorf("grok pool has no endpoints for profile %q", engine.HeavyGrokEndpointProfile)
		}
		return SearchProfileResolution{
			RequestedProfile: requested,
			EffectiveProfile: engine.HeavyGrokEndpointProfile,
			ProfileReason:    "explicit heavy profile requested",
		}, nil
	case SearchProfileAuto:
		wantsHeavy, reason := autoProfileWantsHeavy(ctx, policy)
		if wantsHeavy && poolHasProfile(pool, engine.HeavyGrokEndpointProfile) {
			return SearchProfileResolution{
				RequestedProfile: SearchProfileAuto,
				EffectiveProfile: engine.HeavyGrokEndpointProfile,
				ProfileReason:    "auto selected heavy: " + reason,
			}, nil
		}
		if wantsHeavy {
			return SearchProfileResolution{
				RequestedProfile: SearchProfileAuto,
				EffectiveProfile: engine.DefaultGrokEndpointProfile,
				ProfileReason:    "auto selected default: heavy profile not configured",
			}, nil
		}
		return SearchProfileResolution{
			RequestedProfile: SearchProfileAuto,
			EffectiveProfile: engine.DefaultGrokEndpointProfile,
			ProfileReason:    "auto selected default: " + reason,
		}, nil
	default:
		return SearchProfileResolution{
			RequestedProfile: requested,
			EffectiveProfile: requested,
			ProfileReason:    "explicit profile requested",
		}, nil
	}
}

func normalizeSearchProfile(profile string) string {
	return strings.ToLower(strings.TrimSpace(profile))
}

func defaultResearchProfile(profile string) string {
	profile = normalizeSearchProfile(profile)
	if profile == "" {
		return SearchProfileAuto
	}
	return profile
}

func effectiveSearchPolicy(policy config.SearchPolicy) config.SearchPolicy {
	if strings.TrimSpace(policy.DefaultProfile) == "" {
		return config.DefaultSearchPolicy()
	}
	if strings.TrimSpace(policy.AgentProfile) == "" {
		policy.AgentProfile = config.SearchProfileAuto
	}
	if strings.TrimSpace(policy.AutoPreference) == "" {
		policy.AutoPreference = config.SearchAutoPreferenceIntentBased
	}
	if policy.FallbackAfterSec == 0 && policy.FallbackAfter == 0 {
		policy.FallbackAfterSec = config.DefaultSearchFallbackAfterSec
	}
	if policy.TimeoutSec == 0 && policy.Timeout == 0 {
		policy.TimeoutSec = config.DefaultSearchTimeoutSec
	}
	return policy
}

func poolHasProfile(pool *engine.GrokPool, profile string) bool {
	return pool != nil && pool.HasProfile(profile)
}

func autoProfileWantsHeavy(ctx SearchProfileContext, policy config.SearchPolicy) (bool, string) {
	switch policy.AutoPreference {
	case config.SearchAutoPreferenceHeavyFirst:
		return true, "searchPolicy.autoPreference=heavy-first"
	case config.SearchAutoPreferenceDefaultFirst:
		return false, "searchPolicy.autoPreference=default-first"
	}
	flow := strings.ToLower(strings.TrimSpace(ctx.Flow))
	if flow == searchProfileFlowResearch {
		return true, "intent-based research flow"
	}
	if strings.EqualFold(strings.TrimSpace(ctx.Depth), "deep") {
		return true, "intent-based deep research depth"
	}

	q := strings.ToLower(strings.Join([]string{ctx.Query, ctx.Platform}, " "))
	signals := []struct {
		needle string
		reason string
	}{
		{"research", "research intent"},
		{"deep", "deep intent"},
		{"complex", "complex query"},
		{"current", "current information"},
		{"latest", "current information"},
		{"recent", "current information"},
		{"today", "current information"},
		{"compare", "comparison intent"},
		{"comparison", "comparison intent"},
		{" versus ", "comparison intent"},
		{" vs ", "comparison intent"},
		{"high-risk", "high-risk intent"},
		{"risk", "high-risk intent"},
		{"security", "high-risk intent"},
		{"legal", "high-risk intent"},
		{"financial", "high-risk intent"},
		{"medical", "high-risk intent"},
		{"citation", "source-critical intent"},
		{"source-critical", "source-critical intent"},
		{"evidence", "source-critical intent"},
		{"最新", "current information"},
		{"近期", "current information"},
		{"当前", "current information"},
		{"比较", "comparison intent"},
		{"对比", "comparison intent"},
		{"风险", "high-risk intent"},
		{"证据", "source-critical intent"},
		{"来源", "source-critical intent"},
		{"深入", "deep intent"},
	}
	for _, signal := range signals {
		if strings.Contains(q, signal.needle) {
			return true, "intent-based " + signal.reason
		}
	}
	return false, "no intent-based heavy signal"
}
