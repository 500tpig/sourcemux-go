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

// ExaClient wraps Exa Search and Contents APIs.
type ExaClient struct {
	BaseURL     string
	APIKey      string
	HTTPClient  *http.Client
	RetryConfig RetryConfig
}

func NewExaClient(baseURL, apiKey string) *ExaClient {
	return &ExaClient{
		BaseURL: strings.TrimRight(baseURL, "/"),
		APIKey:  apiKey,
		HTTPClient: &http.Client{
			Timeout: 60 * time.Second,
		},
		RetryConfig: DefaultRetryConfig(),
	}
}

// ExaSearchHit is a single Exa result entry.
type ExaSearchHit struct {
	Title         string   `json:"title"`
	URL           string   `json:"url"`
	ID            string   `json:"id"`
	PublishedDate string   `json:"publishedDate"`
	Author        string   `json:"author"`
	Text          string   `json:"text"`
	Summary       string   `json:"summary"`
	Highlights    []string `json:"highlights"`
}

// ExaSearchResult is the response shape for Exa Search.
type ExaSearchResult struct {
	RequestID  string         `json:"requestId"`
	Results    []ExaSearchHit `json:"results"`
	SearchType string         `json:"searchType"`
	Context    string         `json:"context"`
}

// Search performs an Exa web search. It asks for highlights rather than full
// text to keep fallback search cheap and source-focused.
func (c *ExaClient) Search(ctx context.Context, query string) (*ExaSearchResult, error) {
	if query == "" {
		return nil, fmt.Errorf("exa search: empty query")
	}
	body := map[string]any{
		"query":      query,
		"type":       "auto",
		"numResults": 5,
		"contents": map[string]any{
			"highlights": true,
		},
	}

	var result ExaSearchResult
	if err := c.postJSON(ctx, "/search", body, &result); err != nil {
		return nil, fmt.Errorf("exa search failed: %w", err)
	}
	if len(result.Results) == 0 && result.Context == "" {
		return nil, fmt.Errorf("exa search returned empty result for %q", query)
	}
	return &result, nil
}

// Extract fetches and extracts content from a URL using Exa Contents API.
func (c *ExaClient) Extract(ctx context.Context, url string) (*ExtractResult, error) {
	if url == "" {
		return nil, fmt.Errorf("exa contents: empty url")
	}
	body := map[string]any{
		"urls": []string{url},
		"text": true,
	}

	var result struct {
		Results  []ExaSearchHit `json:"results"`
		Statuses []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
			Error  struct {
				Tag            string `json:"tag"`
				HTTPStatusCode int    `json:"httpStatusCode"`
			} `json:"error"`
		} `json:"statuses"`
	}
	if err := c.postJSON(ctx, "/contents", body, &result); err != nil {
		return nil, fmt.Errorf("exa contents failed: %w", err)
	}
	if len(result.Results) == 0 {
		if len(result.Statuses) > 0 && result.Statuses[0].Status == "error" {
			st := result.Statuses[0]
			return nil, fmt.Errorf("exa contents returned no result for %s: %s (%d)", url, st.Error.Tag, st.Error.HTTPStatusCode)
		}
		return nil, fmt.Errorf("exa contents returned no result for %s", url)
	}

	content := bestExaHitContent(result.Results[0])
	if content == "" {
		return nil, fmt.Errorf("exa contents returned empty content for %s", url)
	}

	resultURL := result.Results[0].URL
	if resultURL == "" {
		resultURL = url
	}
	return &ExtractResult{
		URL:     resultURL,
		Content: content,
	}, nil
}

func (c *ExaClient) postJSON(ctx context.Context, path string, body map[string]any, out any) error {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	factory := func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			c.BaseURL+path, bytes.NewReader(jsonBody))
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("x-api-key", c.APIKey)
		return req, nil
	}

	resp, err := httpDoWithRetry(ctx, c.HTTPClient, factory, c.RetryConfig)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("exa API %d: %s", resp.StatusCode, string(data))
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode: %w", err)
	}
	return nil
}

// ExaSearchSourceURLs returns de-duplicated source URLs from a search result.
func ExaSearchSourceURLs(res *ExaSearchResult) []string {
	if res == nil {
		return nil
	}
	urls := make([]string, 0, len(res.Results))
	for _, r := range res.Results {
		if r.URL != "" {
			urls = append(urls, r.URL)
		}
	}
	return dedupURLs(urls)
}

// FormatExaSearchContent turns source-first Exa results into an LLM-readable
// body. It intentionally avoids pretending Exa returned a synthesized answer.
func FormatExaSearchContent(res *ExaSearchResult) string {
	if res == nil {
		return ""
	}
	if res.Context != "" && len(res.Results) == 0 {
		return res.Context
	}

	var sb strings.Builder
	sb.WriteString("Exa returned source-first search results. Use get_sources/web_fetch for verification.\n\n")
	for i, r := range res.Results {
		title := r.Title
		if title == "" {
			title = r.URL
		}
		fmt.Fprintf(&sb, "%d. %s\n", i+1, title)
		if r.URL != "" {
			fmt.Fprintf(&sb, "   URL: %s\n", r.URL)
		}
		if r.PublishedDate != "" {
			fmt.Fprintf(&sb, "   Published: %s\n", r.PublishedDate)
		}
		if snippet := compactExaSnippet(r, 700); snippet != "" {
			fmt.Fprintf(&sb, "   Snippet: %s\n", snippet)
		}
	}
	return strings.TrimSpace(sb.String())
}

func bestExaHitContent(hit ExaSearchHit) string {
	if hit.Text != "" {
		return hit.Text
	}
	if hit.Summary != "" {
		return hit.Summary
	}
	if len(hit.Highlights) > 0 {
		return strings.Join(hit.Highlights, "\n\n")
	}
	return ""
}

func compactExaSnippet(hit ExaSearchHit, maxRunes int) string {
	text := ""
	if len(hit.Highlights) > 0 {
		text = strings.Join(hit.Highlights, " ")
	} else {
		text = bestExaHitContent(hit)
	}
	return clipRunes(strings.Join(strings.Fields(text), " "), maxRunes)
}

func clipRunes(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return s
	}
	rs := []rune(s)
	if len(rs) <= maxRunes {
		return s
	}
	return string(rs[:maxRunes]) + "..."
}
