package tools

import (
	"fmt"
	"net/url"
	"strings"
)

const (
	FetchProfileAuto    = "auto"
	FetchProfileQuality = "quality"
	FetchProfileCheap   = "cheap"
	FetchProfileGitHub  = "github"
	FetchProfileCompare = "compare"

	FetchIntentOrdinary = "ordinary"
	FetchIntentGitHub   = "github"
	FetchIntentDocs     = "docs"
	FetchIntentHardPage = "hard-page"
)

// FetchPolicy describes the fetch profile and classified URL intent used to
// choose provider order. It is exported so CLI JSON can explain routing.
type FetchPolicy struct {
	RequestedProfile string   `json:"requested_profile"`
	EffectiveProfile string   `json:"effective_profile"`
	Intent           string   `json:"intent"`
	ProviderOrder    []string `json:"provider_order"`
	Reason           string   `json:"reason"`
}

func ResolveFetchPolicy(rawProfile, targetURL string, configuredOrder []string, strictOrder bool) (FetchPolicy, error) {
	profile, err := NormalizeFetchProfile(rawProfile)
	if err != nil {
		return FetchPolicy{}, err
	}
	intent := ClassifyFetchIntent(targetURL)
	effective := profile
	if profile == "" {
		effective = FetchProfileAuto
	}
	if effective == FetchProfileAuto && intent == FetchIntentGitHub {
		effective = FetchProfileGitHub
	}
	order := providerOrderForFetch(effective, intent, configuredOrder, strictOrder)
	return FetchPolicy{
		RequestedProfile: profileOrDefault(profile),
		EffectiveProfile: effective,
		Intent:           intent,
		ProviderOrder:    order,
		Reason:           fetchPolicyReason(effective, intent, strictOrder),
	}, nil
}

func NormalizeFetchProfile(raw string) (string, error) {
	switch profile := strings.ToLower(strings.TrimSpace(raw)); profile {
	case "", FetchProfileAuto:
		return FetchProfileAuto, nil
	case FetchProfileQuality:
		return FetchProfileQuality, nil
	case FetchProfileCheap, "low-cost", "lowcost", "quick", "zero-key", "zerokey":
		return FetchProfileCheap, nil
	case FetchProfileGitHub:
		return FetchProfileGitHub, nil
	case FetchProfileCompare, "compare-fetch":
		return FetchProfileCompare, nil
	default:
		return "", fmt.Errorf("unsupported fetch profile %q (want auto, quality, cheap, github, or compare)", raw)
	}
}

func ClassifyFetchIntent(rawURL string) string {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return FetchIntentOrdinary
	}
	host := strings.ToLower(strings.TrimPrefix(u.Hostname(), "www."))
	path := strings.Trim(u.EscapedPath(), "/")
	if host == "github.com" && githubPathHasOwnerRepo(path) {
		return FetchIntentGitHub
	}
	if strings.Contains(host, "github.") && githubPathHasOwnerRepo(path) {
		return FetchIntentGitHub
	}
	lower := strings.ToLower(host + "/" + path)
	if strings.Contains(lower, "docs") || strings.Contains(lower, "developer") || strings.Contains(lower, "api") {
		return FetchIntentDocs
	}
	if strings.Contains(lower, "forum") || strings.Contains(lower, "community") || strings.Contains(lower, "login") || strings.Contains(lower, "signin") {
		return FetchIntentHardPage
	}
	return FetchIntentOrdinary
}

func githubPathHasOwnerRepo(path string) bool {
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return false
	}
	if parts[0] == "" || parts[1] == "" {
		return false
	}
	switch parts[0] {
	case "features", "marketplace", "orgs", "organizations", "search", "settings", "topics":
		return false
	default:
		return true
	}
}

func providerOrderForFetch(profile, intent string, configuredOrder []string, strictOrder bool) []string {
	if strictOrder && len(configuredOrder) > 0 && profile == FetchProfileAuto {
		return append([]string(nil), configuredOrder...)
	}
	switch profile {
	case FetchProfileCheap:
		return []string{"jina", "firecrawl", "exa", "tavily"}
	case FetchProfileGitHub:
		return []string{"github", "exa", "jina", "firecrawl", "tavily"}
	case FetchProfileCompare:
		return []string{"github", "firecrawl", "jina", "exa", "tavily", "tinyfish"}
	case FetchProfileQuality, FetchProfileAuto:
		if intent == FetchIntentGitHub {
			return []string{"github", "exa", "jina", "firecrawl", "tavily"}
		}
		if intent == FetchIntentDocs {
			return []string{"firecrawl", "exa", "jina", "tavily", "tinyfish"}
		}
		return []string{"firecrawl", "jina", "exa", "tavily", "tinyfish"}
	default:
		return []string{"firecrawl", "jina", "exa", "tavily", "tinyfish"}
	}
}

func fetchPolicyReason(profile, intent string, strictOrder bool) string {
	if strictOrder && profile == FetchProfileAuto {
		return "auto honors explicit v2 web_fetch provider order"
	}
	switch profile {
	case FetchProfileCheap:
		return "cheap profile keeps Jina first for low-cost/zero-key reads"
	case FetchProfileGitHub:
		return "github profile tries repository-aware enrichment before generic web extraction"
	case FetchProfileCompare:
		return "compare profile attempts all configured fetch providers in policy order"
	case FetchProfileQuality:
		return "quality profile prefers cleaner extraction before low-cost fallbacks"
	default:
		if intent == FetchIntentGitHub {
			return "auto classified GitHub URL and tries repository-aware enrichment first"
		}
		return "auto defaults to quality-first fetch routing"
	}
}

func profileOrDefault(profile string) string {
	if strings.TrimSpace(profile) == "" {
		return FetchProfileAuto
	}
	return profile
}
