package engine

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// JinaClient wraps the Jina Reader API.
//
// Jina Reader exposes a single endpoint: GET {BaseURL}/{target_url} which
// returns the target page rendered as Markdown. The service is free; passing
// an API key via Authorization yields higher rate limits.
type JinaClient struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
}

// NewJinaClient builds a Jina Reader client. If baseURL is empty, the public
// endpoint https://r.jina.ai is used. apiKey is optional.
func NewJinaClient(baseURL, apiKey string) *JinaClient {
	if baseURL == "" {
		baseURL = "https://r.jina.ai"
	}
	return &JinaClient{
		BaseURL: strings.TrimRight(baseURL, "/"),
		APIKey:  apiKey,
		HTTPClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// Fetch retrieves a URL via Jina Reader and returns the rendered Markdown
// content. The target URL is appended to BaseURL verbatim per Jina's
// documented contract (no extra URL-encoding).
func (c *JinaClient) Fetch(ctx context.Context, target string) (*ExtractResult, error) {
	if target == "" {
		return nil, fmt.Errorf("jina fetch: empty target URL")
	}
	endpoint := c.BaseURL + "/" + target

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "text/markdown, text/plain;q=0.9")
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jina fetch failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jina API %d: %s", resp.StatusCode, string(body))
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("jina returned empty content for %s", target)
	}

	return &ExtractResult{
		URL:     target,
		Content: string(body),
	}, nil
}
