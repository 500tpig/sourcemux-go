package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// TavilyClient wraps the Tavily Search, Extract and Map APIs.
type TavilyClient struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
	// RetryConfig governs httpDoWithRetry behaviour for Search/Extract/Map:
	// 429/5xx + network errors are retried with capped exponential backoff,
	// honouring any Retry-After header from upstream.
	RetryConfig RetryConfig
}

func NewTavilyClient(baseURL, apiKey string) *TavilyClient {
	return &TavilyClient{
		BaseURL: baseURL,
		APIKey:  apiKey,
		HTTPClient: &http.Client{
			Timeout: 60 * time.Second,
		},
		RetryConfig: DefaultRetryConfig(),
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

	factory := func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			c.BaseURL+"/extract", bytes.NewReader(jsonBody))
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
		return req, nil
	}

	resp, err := httpDoWithRetry(ctx, c.HTTPClient, factory, c.RetryConfig)
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

	factory := func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			c.BaseURL+"/map", bytes.NewReader(jsonBody))
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
		return req, nil
	}

	resp, err := httpDoWithRetry(ctx, c.HTTPClient, factory, c.RetryConfig)
	if err != nil {
		return nil, fmt.Errorf("tavily map failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("tavily map API %d: %s", resp.StatusCode, string(data))
	}

	var result struct {
		Results []string `json:"results"`
		URLs    []string `json:"urls"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	urls := result.Results
	if len(urls) == 0 {
		urls = result.URLs
	}
	if urls == nil {
		urls = []string{}
	}
	return &MapResult{URLs: urls}, nil
}

// TavilyCrawlRequest holds parameters for the Tavily Crawl API.
type TavilyCrawlRequest struct {
	URL           string `json:"url"`
	Instructions  string `json:"instructions,omitempty"`
	MaxDepth      int    `json:"max_depth,omitempty"`
	MaxBreadth    int    `json:"max_breadth,omitempty"`
	Limit         int    `json:"limit,omitempty"`
	ExtractDepth  string `json:"extract_depth,omitempty"`
	Format        string `json:"format,omitempty"`
	IncludeImages bool   `json:"include_images,omitempty"`
}

// TavilyCrawlPage is a single crawled and extracted page.
type TavilyCrawlPage struct {
	URL        string   `json:"url"`
	RawContent string   `json:"raw_content"`
	Images     []string `json:"images,omitempty"`
}

// TavilyCrawlResult is the response shape for the Tavily Crawl API.
type TavilyCrawlResult struct {
	BaseURL      string            `json:"base_url"`
	Results      []TavilyCrawlPage `json:"results"`
	ResponseTime float64           `json:"response_time"`
}

// Crawl traverses a site and extracts content using Tavily Crawl API.
func (c *TavilyClient) Crawl(ctx context.Context, crawlReq TavilyCrawlRequest) (*TavilyCrawlResult, error) {
	if crawlReq.URL == "" {
		return nil, fmt.Errorf("tavily crawl: empty url")
	}
	jsonBody, err := json.Marshal(crawlReq)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	factory := func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			c.BaseURL+"/crawl", bytes.NewReader(jsonBody))
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
		return req, nil
	}

	resp, err := httpDoWithRetry(ctx, c.HTTPClient, factory, c.RetryConfig)
	if err != nil {
		return nil, fmt.Errorf("tavily crawl failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("tavily crawl API %d: %s", resp.StatusCode, string(data))
	}

	var result TavilyCrawlResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	if len(result.Results) == 0 {
		return nil, fmt.Errorf("tavily crawl returned empty result for %s", crawlReq.URL)
	}

	return &result, nil
}

// FormatTavilyCrawlContent renders crawl results in a compact, LLM-readable
// form. maxCharsPerPage caps each page body; non-positive values use a safe
// default for MCP and human CLI output.
func FormatTavilyCrawlContent(res *TavilyCrawlResult, maxCharsPerPage int) string {
	if res == nil {
		return ""
	}
	if maxCharsPerPage <= 0 {
		maxCharsPerPage = 1200
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "base_url: %s\n", res.BaseURL)
	fmt.Fprintf(&sb, "results_count: %d\n", len(res.Results))
	if res.ResponseTime > 0 {
		fmt.Fprintf(&sb, "response_time: %.2fs\n", res.ResponseTime)
	}
	for i, r := range res.Results {
		if r.URL == "" && r.RawContent == "" {
			continue
		}
		fmt.Fprintf(&sb, "\n%d. %s\n", i+1, r.URL)
		fmt.Fprintf(&sb, "content_chars: %d\n", len([]rune(r.RawContent)))
		if len(r.Images) > 0 {
			fmt.Fprintf(&sb, "images_count: %d\n", len(r.Images))
		}
		if snippet := crawlSnippet(r.RawContent, maxCharsPerPage); snippet != "" {
			sb.WriteString(snippet)
			sb.WriteString("\n")
		}
	}
	return strings.TrimSpace(sb.String())
}

func crawlSnippet(text string, maxChars int) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= maxChars {
		return text
	}
	return strings.TrimSpace(string(runes[:maxChars])) + "\n... [truncated]"
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

	factory := func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			c.BaseURL+"/search", bytes.NewReader(jsonBody))
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
		return req, nil
	}

	resp, err := httpDoWithRetry(ctx, c.HTTPClient, factory, c.RetryConfig)
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
