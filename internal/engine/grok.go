package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// urlRegex captures http(s):// URLs in plain text. Conservative stop set so we
// don't gobble closing brackets, quotes, or whitespace from surrounding prose.
var urlRegex = regexp.MustCompile(`https?://[^\s)\]<>"']+`)

// urlTrailingPunct is stripped from URLs extracted via regex (sentences often
// end with these right after the URL).
const urlTrailingPunct = ".,;:!?"

// GrokClient wraps calls to a Grok-compatible (OpenAI-format) API with web search.
type GrokClient struct {
	BaseURL    string
	APIKey     string
	Model      string
	HTTPClient *http.Client
}

func NewGrokClient(baseURL, apiKey, model string) *GrokClient {
	return &GrokClient{
		BaseURL: baseURL,
		APIKey:  apiKey,
		Model:   model,
		HTTPClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// SearchResult holds the AI answer plus extracted source URLs.
type SearchResult struct {
	Content      string   `json:"content"`
	SourceURLs   []string `json:"source_urls"`
	SourcesCount int      `json:"sources_count"`
}

// grokRawResponse mirrors the OpenAI-compatible Grok response, including the
// optional source fields different proxies expose.
type grokRawResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`

	// Native Grok format: array of URL strings.
	Citations []string `json:"citations"`

	// Some Grok proxies expose structured search results instead of citations.
	SearchResults []struct {
		URL string `json:"url"`
	} `json:"search_results"`
}

// Search sends a query to the Grok chat completions endpoint.
func (c *GrokClient) Search(ctx context.Context, query string) (*SearchResult, error) {
	body := map[string]any{
		"model": c.Model,
		"messages": []map[string]string{
			{"role": "user", "content": query},
		},
		"search": true,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.BaseURL+"/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("grok request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("grok API %d: %s", resp.StatusCode, string(data))
	}

	var raw grokRawResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	content := ""
	if len(raw.Choices) > 0 {
		content = raw.Choices[0].Message.Content
	}

	urls := extractSourceURLs(&raw, content)

	return &SearchResult{
		Content:      content,
		SourceURLs:   urls,
		SourcesCount: len(urls),
	}, nil
}

// extractSourceURLs picks the best available source list from a Grok response.
//
// Priority order:
//  1. Native `citations` array (most Grok-compatible providers).
//  2. Structured `search_results` objects (some grok2api flavors).
//  3. http(s) URLs scraped from the answer text (last resort).
func extractSourceURLs(raw *grokRawResponse, content string) []string {
	if len(raw.Citations) > 0 {
		return dedupURLs(raw.Citations)
	}
	if len(raw.SearchResults) > 0 {
		urls := make([]string, 0, len(raw.SearchResults))
		for _, r := range raw.SearchResults {
			if r.URL != "" {
				urls = append(urls, r.URL)
			}
		}
		if len(urls) > 0 {
			return dedupURLs(urls)
		}
	}
	if content != "" {
		matches := urlRegex.FindAllString(content, -1)
		cleaned := make([]string, 0, len(matches))
		for _, m := range matches {
			m = strings.TrimRight(m, urlTrailingPunct)
			if m != "" {
				cleaned = append(cleaned, m)
			}
		}
		if len(cleaned) > 0 {
			return dedupURLs(cleaned)
		}
	}
	return nil
}

// dedupURLs preserves first-seen order while dropping empties and duplicates.
func dedupURLs(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, u := range in {
		if u == "" {
			continue
		}
		if _, ok := seen[u]; ok {
			continue
		}
		seen[u] = struct{}{}
		out = append(out, u)
	}
	return out
}

// ListModels returns available models from the Grok-compatible endpoint.
func (c *GrokClient) ListModels(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.BaseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	models := make([]string, len(result.Data))
	for i, m := range result.Data {
		models[i] = m.ID
	}
	return models, nil
}
