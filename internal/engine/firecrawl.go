package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// FirecrawlClient wraps the Firecrawl Scrape API as a fallback for Tavily.
type FirecrawlClient struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
}

func NewFirecrawlClient(baseURL, apiKey string) *FirecrawlClient {
	return &FirecrawlClient{
		BaseURL: baseURL,
		APIKey:  apiKey,
		HTTPClient: &http.Client{
			Timeout: 90 * time.Second,
		},
	}
}

// Scrape fetches content from a URL using Firecrawl's scrape endpoint.
func (c *FirecrawlClient) Scrape(ctx context.Context, url string) (*ExtractResult, error) {
	body := map[string]any{
		"url":     url,
		"formats": []string{"markdown"},
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.BaseURL+"/v1/scrape", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("firecrawl scrape failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("firecrawl API %d: %s", resp.StatusCode, string(data))
	}

	var result struct {
		Data struct {
			Markdown string `json:"markdown"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	if result.Data.Markdown == "" {
		return nil, fmt.Errorf("firecrawl returned empty content for %s", url)
	}

	return &ExtractResult{
		URL:     url,
		Content: result.Data.Markdown,
	}, nil
}
