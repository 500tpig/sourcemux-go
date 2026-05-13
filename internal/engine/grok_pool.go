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
	// APIType selects the request protocol. Valid values:
	//   "" or "chat"   → POST /v1/chat/completions (default)
	//   "responses"    → POST /v1/responses        (xAI Responses API)
	APIType string `json:"apiType"`
	// ResponseTools selects built-in xAI Responses API tools when APIType is
	// "responses" and SendSearchFlag is true. Empty means the legacy default:
	// web_search only.
	ResponseTools []string `json:"responseTools,omitempty"`
}

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
		c := NewGrokClient(ep.BaseURL, ep.APIKey, ep.Model)
		if ep.Name != "" {
			c.Name = ep.Name
		}
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
	if p == nil || len(p.clients) == 0 {
		return nil, errors.New("grok pool is empty: no endpoints configured")
	}
	if p.OverallTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.OverallTimeout)
		defer cancel()
	}
	var errs []string
	for _, c := range p.clients {
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
	return nil, fmt.Errorf("all %d grok endpoints failed: %s",
		len(p.clients), strings.Join(errs, "; "))
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
