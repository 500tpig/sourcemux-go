package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// GrokEndpoint is one entry in the ordered endpoint pool stored in
// sourcemux.json.
type GrokEndpoint struct {
	Name           string `json:"name"`
	BaseURL        string `json:"baseURL"`
	APIKey         string `json:"apiKey"`
	Model          string `json:"model"`
	SendSearchFlag bool   `json:"sendSearchFlag"`
	// Enabled controls whether this endpoint participates in Grok pool search.
	// Nil preserves the historical default of enabled=true.
	Enabled *bool `json:"enabled,omitempty"`
	// Profile gates heavier endpoints behind explicit selection. Empty is
	// normalized to "default"; normal search only uses the default profile.
	Profile string `json:"profile,omitempty"`
	// APIType selects the request protocol. Valid values:
	//   "" or "chat"   → POST /v1/chat/completions (default)
	//   "responses"    → POST /v1/responses        (xAI Responses API)
	APIType string `json:"apiType"`
	// ResponseTools selects built-in xAI Responses API tools when APIType is
	// "responses" and SendSearchFlag is true. Empty means the legacy default:
	// web_search only.
	ResponseTools []string `json:"responseTools,omitempty"`
}

const DefaultGrokEndpointProfile = "default"

// GrokPool routes a search through an ordered list of Grok endpoints,
// failing over to the next endpoint when one returns an error or empty content.
type GrokPool struct {
	clients []*GrokClient
	// OverallTimeout caps the total wall-clock budget Search will spend across
	// all endpoints + retries. 0 means no cap; the only deadline is then the
	// caller's ctx (and each client's per-attempt RetryConfig.MaxDelay).
	OverallTimeout time.Duration
}

// NewGrokPool builds a pool from endpoint configs. Endpoints are tried in the
// order given.
func NewGrokPool(endpoints []GrokEndpoint) *GrokPool {
	clients := make([]*GrokClient, 0, len(endpoints))
	for _, ep := range endpoints {
		if !ep.IsEnabled() {
			continue
		}
		c := NewGrokClient(ep.BaseURL, ep.APIKey, ep.Model)
		if ep.Name != "" {
			c.Name = ep.Name
		}
		c.Profile = ep.EffectiveProfile()
		c.SendSearchFlag = ep.SendSearchFlag
		c.APIType = ep.APIType
		c.ResponseTools = append([]string(nil), ep.ResponseTools...)
		clients = append(clients, c)
	}
	return &GrokPool{clients: clients}
}

// Len reports the number of configured endpoints.
func (p *GrokPool) Len() int {
	if p == nil {
		return 0
	}
	return len(p.clients)
}

// Clients returns the underlying clients in priority order. Mainly for
// diagnostics / listing in the get_config_info tool.
func (p *GrokPool) Clients() []*GrokClient {
	if p == nil {
		return nil
	}
	return p.clients
}

// WithOverallTimeout returns a shallow copy of the pool with the same endpoint
// clients and a different total wall-clock budget. This is useful for per-call
// overrides without mutating a shared server pool.
func (p *GrokPool) WithOverallTimeout(timeout time.Duration) *GrokPool {
	if p == nil {
		return nil
	}
	cloned := *p
	cloned.clients = append([]*GrokClient(nil), p.clients...)
	cloned.OverallTimeout = timeout
	return &cloned
}

// PoolSearchResult bundles a SearchResult with the endpoint that produced it.
type PoolSearchResult struct {
	*SearchResult
	EndpointName  string
	EndpointModel string
}

// Search tries endpoints in order, returning the first non-empty success.
// On full failure it returns an aggregated error listing per-endpoint reasons.
func (p *GrokPool) Search(ctx context.Context, query string) (*PoolSearchResult, error) {
	return p.SearchWithModel(ctx, query, "")
}

// SearchWithModel is like Search, but overrides every endpoint's configured
// model for this single request when model is non-empty. It does not mutate the
// pool, so concurrent requests keep using their own configured/default model.
func (p *GrokPool) SearchWithModel(ctx context.Context, query, model string) (*PoolSearchResult, error) {
	return p.SearchWithModelAndProfile(ctx, query, model, "")
}

// SearchWithModelAndProfile is like SearchWithModel, but only tries endpoints
// assigned to profile. Empty profile means the default profile.
func (p *GrokPool) SearchWithModelAndProfile(ctx context.Context, query, model, profile string) (*PoolSearchResult, error) {
	if p == nil || len(p.clients) == 0 {
		return nil, errors.New("grok pool is empty: no endpoints configured")
	}
	profile = normalizeGrokEndpointProfile(profile)
	clients := p.profileClients(profile)
	if len(clients) == 0 {
		return nil, fmt.Errorf("grok pool has no endpoints for profile %q", profile)
	}
	if p.OverallTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.OverallTimeout)
		defer cancel()
	}
	var errs []string
	for _, c := range clients {
		// Stop early if the overall budget (or caller's ctx) is exhausted.
		if err := ctx.Err(); err != nil {
			errs = append(errs, fmt.Sprintf("(deadline reached: %v)", err))
			break
		}
		active := c
		if model != "" {
			cloned := *c
			cloned.Model = model
			active = &cloned
		}
		res, err := active.Search(ctx, query)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", active.Name, err))
			continue
		}
		if res == nil || res.Content == "" {
			errs = append(errs, fmt.Sprintf("%s: empty content", active.Name))
			continue
		}
		if res.SourcesCount == 0 && looksLikeGrokUnavailableContent(res.Content) {
			errs = append(errs, fmt.Sprintf("%s: unavailable response: %s", active.Name, strings.TrimSpace(res.Content)))
			continue
		}
		return &PoolSearchResult{
			SearchResult:  res,
			EndpointName:  active.Name,
			EndpointModel: active.Model,
		}, nil
	}
	return nil, fmt.Errorf("all %d grok endpoints failed for profile %q: %s",
		len(clients), profile, strings.Join(errs, "; "))
}

func (p *GrokPool) profileClients(profile string) []*GrokClient {
	profile = normalizeGrokEndpointProfile(profile)
	out := make([]*GrokClient, 0, len(p.clients))
	for _, c := range p.clients {
		if c.EffectiveProfile() == profile {
			out = append(out, c)
		}
	}
	return out
}

func (e GrokEndpoint) IsEnabled() bool {
	return e.Enabled == nil || *e.Enabled
}

func (e GrokEndpoint) EffectiveProfile() string {
	return normalizeGrokEndpointProfile(e.Profile)
}

// FilterGrokEndpoints returns enabled endpoints assigned to profile. Empty
// profile means the default profile.
func FilterGrokEndpoints(endpoints []GrokEndpoint, profile string) []GrokEndpoint {
	profile = normalizeGrokEndpointProfile(profile)
	out := make([]GrokEndpoint, 0, len(endpoints))
	for _, ep := range endpoints {
		if ep.IsEnabled() && ep.EffectiveProfile() == profile {
			out = append(out, ep)
		}
	}
	return out
}

func normalizeGrokEndpointProfile(profile string) string {
	profile = strings.ToLower(strings.TrimSpace(profile))
	if profile == "" {
		return DefaultGrokEndpointProfile
	}
	return profile
}

func looksLikeGrokUnavailableContent(content string) bool {
	s := strings.ToLower(strings.TrimSpace(content))
	if s == "" || len([]rune(s)) > 400 {
		return false
	}
	needles := []string{
		"this model is overloaded",
		"model is overloaded",
		"please try again shortly",
		"temporarily unavailable",
		"only super grok users",
		"supergrok users",
	}
	for _, needle := range needles {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}
