package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// GrokEndpoint is one entry in a Grok endpoint pool. Matches the JSON shape
// accepted by GROK_ENDPOINTS_JSON / GROK_ENDPOINTS_FILE.
type GrokEndpoint struct {
	Name           string `json:"name"`
	BaseURL        string `json:"baseURL"`
	APIKey         string `json:"apiKey"`
	Model          string `json:"model"`
	SendSearchFlag bool   `json:"sendSearchFlag"`
}

// GrokPool routes a search through an ordered list of Grok endpoints,
// failing over to the next endpoint when one returns an error or empty content.
type GrokPool struct {
	clients []*GrokClient
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
	if p == nil || len(p.clients) == 0 {
		return nil, errors.New("grok pool is empty: no endpoints configured")
	}
	var errs []string
	for _, c := range p.clients {
		res, err := c.Search(ctx, query)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", c.Name, err))
			continue
		}
		if res == nil || res.Content == "" {
			errs = append(errs, fmt.Sprintf("%s: empty content", c.Name))
			continue
		}
		return &PoolSearchResult{
			SearchResult:  res,
			EndpointName:  c.Name,
			EndpointModel: c.Model,
		}, nil
	}
	return nil, fmt.Errorf("all %d grok endpoints failed: %s",
		len(p.clients), strings.Join(errs, "; "))
}
