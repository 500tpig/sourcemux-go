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

// TavilyClient wraps the Tavily Search, Extract and Map APIs.
type TavilyClient struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
}

func NewTavilyClient(baseURL, apiKey string) *TavilyClient {
	return &TavilyClient{
		BaseURL: baseURL,
		APIKey:  apiKey,
		HTTPClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// ExtractResult holds the content extracted from a URL.
type ExtractResult struct {
	URL     string `json:"url"`
	Content string `json:"content"`
}

// Extract fetches and extracts content from a URL using Tavily Extract API.
func (c *TavilyClient) Extract(ctx context.Context, url string) (*ExtractResult, error) {
	body := map[string]any{
		"urls": []string{url},
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.BaseURL+"/extract", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tavily extract failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("tavily API %d: %s", resp.StatusCode, string(data))
	}

	var result struct {
		Results []struct {
			URL     string `json:"url"`
			Content string `json:"raw_content"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	if len(result.Results) == 0 || result.Results[0].Content == "" {
		return nil, fmt.Errorf("tavily returned empty content for %s", url)
	}

	return &ExtractResult{
		URL:     url,
		Content: result.Results[0].Content,
	}, nil
}

// MapResult holds discovered URLs from site mapping.
type MapResult struct {
	URLs []string `json:"urls"`
}

// Map discovers URLs on a website using Tavily Map API.
func (c *TavilyClient) Map(ctx context.Context, url string, maxDepth, maxBreadth, limit int) (*MapResult, error) {
	body := map[string]any{
		"url":         url,
		"max_depth":   maxDepth,
		"max_breadth": maxBreadth,
		"limit":       limit,
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.BaseURL+"/map", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tavily map failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("tavily map API %d: %s", resp.StatusCode, string(data))
	}

	var result struct {
		URLs []string `json:"urls"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	return &MapResult{URLs: result.URLs}, nil
}

// TavilySearchHit is a single Tavily Search result entry.
type TavilySearchHit struct {
	Title   string  `json:"title"`
	URL     string  `json:"url"`
	Content string  `json:"content"`
	Score   float64 `json:"score"`
}

// TavilySearchResult is the response shape for the Tavily Search API.
type TavilySearchResult struct {
	Answer  string            `json:"answer"`
	Query   string            `json:"query"`
	Results []TavilySearchHit `json:"results"`
}

// Search performs a web search via Tavily Search and returns an answer plus
// a ranked list of source URLs. Used as a fallback for the primary Grok web
// search when Grok is unavailable.
func (c *TavilyClient) Search(ctx context.Context, query string) (*TavilySearchResult, error) {
	if query == "" {
		return nil, fmt.Errorf("tavily search: empty query")
	}
	body := map[string]any{
		"query":          query,
		"search_depth":   "basic",
		"include_answer": true,
		"max_results":    5,
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.BaseURL+"/search", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tavily search failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("tavily search API %d: %s", resp.StatusCode, string(data))
	}

	var result TavilySearchResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	if len(result.Results) == 0 && result.Answer == "" {
		return nil, fmt.Errorf("tavily search returned empty result for %q", query)
	}

	return &result, nil
}
