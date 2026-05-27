package tools

import (
	"fmt"
	"strings"

	"github.com/500tpig/sourcemux-go/internal/engine"
)

const (
	SearchProfileAuto = "auto"

	searchProfileFlowSearch   = "search"
	searchProfileFlowResearch = "research"
)

// SearchProfileContext describes the caller intent used by profile=auto.
type SearchProfileContext struct {
	Flow     string
	Depth    string
	Query    string
	Platform string
}

// SearchProfileResolution records both user intent and the concrete Grok pool
// profile used for this request.
type SearchProfileResolution struct {
	RequestedProfile string `json:"requested_profile"`
	EffectiveProfile string `json:"effective_profile"`
	ProfileReason    string `json:"profile_reason,omitempty"`
}

// ResolveSearchProfile maps a user-requested profile to the concrete Grok pool
// profile. Empty requested profile keeps normal fast search on the default
// profile. Explicit heavy must be configured; auto safely degrades to default.
func ResolveSearchProfile(pool *engine.GrokPool, requested string, ctx SearchProfileContext) (SearchProfileResolution, error) {
	requested = normalizeSearchProfile(requested)
	if requested == "" {
		requested = engine.DefaultGrokEndpointProfile
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
		wantsHeavy, reason := autoProfileWantsHeavy(ctx)
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
			ProfileReason:    "auto selected default: no heavy intent signal",
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

func poolHasProfile(pool *engine.GrokPool, profile string) bool {
	return pool != nil && pool.HasProfile(profile)
}

func autoProfileWantsHeavy(ctx SearchProfileContext) (bool, string) {
	flow := strings.ToLower(strings.TrimSpace(ctx.Flow))
	if flow == searchProfileFlowResearch {
		return true, "research flow"
	}
	if strings.EqualFold(strings.TrimSpace(ctx.Depth), "deep") {
		return true, "deep research depth"
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
			return true, signal.reason
		}
	}
	return false, ""
}
