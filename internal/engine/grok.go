package engine

import (
	"bufio"
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
	Name    string
	BaseURL string
	APIKey  string
	Model   string
	// APIType selects the request/response protocol:
	//   ""  or "chat"      → POST /v1/chat/completions  (default, all grok2api proxies)
	//   "responses"        → POST /v1/responses         (xAI Responses API, native xAI endpoints)
	APIType string
	// SendSearchFlag controls web-search activation. The exact mechanism differs
	// by APIType: "chat" sends "search":true; "responses" appends a web_search tool.
	// Many grok2api proxies auto-search and either ignore or reject the flag, so
	// it's configurable per-endpoint.
	SendSearchFlag bool
	HTTPClient     *http.Client
	// RetryConfig governs httpDoWithRetry behaviour: 429/5xx + network errors are
	// retried with capped exponential backoff, honouring any Retry-After header.
	RetryConfig RetryConfig
}

// NewGrokClient creates a Grok client with default 60s timeout and search flag enabled.
func NewGrokClient(baseURL, apiKey, model string) *GrokClient {
	return &GrokClient{
		Name:           "grok",
		BaseURL:        baseURL,
		APIKey:         apiKey,
		Model:          model,
		SendSearchFlag: true,
		HTTPClient: &http.Client{
			Timeout: 60 * time.Second,
		},
		RetryConfig: DefaultRetryConfig(),
	}
}

// SearchResult holds the AI answer plus extracted source URLs.
type SearchResult struct {
	Content      string   `json:"content"`
	SourceURLs   []string `json:"source_urls"`
	SourcesCount int      `json:"sources_count"`
}

// grokRawResponse mirrors the OpenAI-compatible Grok response, including all the
// optional source fields different proxies expose.
type grokRawResponse struct {
	Choices []struct {
		Message struct {
			Content     string `json:"content"`
			Annotations []struct {
				Type        string `json:"type"`
				URLCitation struct {
					URL   string `json:"url"`
					Title string `json:"title"`
				} `json:"url_citation"`
			} `json:"annotations"`
		} `json:"message"`
	} `json:"choices"`

	// Native xAI Grok format: array of URL strings.
	Citations []string `json:"citations"`

	// grok2api wykon/yyds flavor: top-level structured search sources.
	SearchSources []struct {
		URL   string `json:"url"`
		Title string `json:"title"`
		Type  string `json:"type"`
	} `json:"search_sources"`

	// Older grok2api flavor: top-level results array.
	SearchResults []struct {
		URL string `json:"url"`
	} `json:"search_results"`
}

// Search sends a query to the configured Grok endpoint.
// It routes to /v1/responses or /v1/chat/completions based on c.APIType.
func (c *GrokClient) Search(ctx context.Context, query string) (*SearchResult, error) {
	if c.APIType == "responses" {
		return c.searchViaResponses(ctx, query)
	}
	return c.searchViaChatCompletions(ctx, query)
}

// searchViaChatCompletions sends a query to the Grok chat completions endpoint.
func (c *GrokClient) searchViaChatCompletions(ctx context.Context, query string) (*SearchResult, error) {
	body := map[string]any{
		"model": c.Model,
		"messages": []map[string]string{
			{"role": "user", "content": query},
		},
	}
	if c.SendSearchFlag {
		body["search"] = true
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	factory := func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			c.BaseURL+"/chat/completions", bytes.NewReader(jsonBody))
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
		return req, nil
	}

	resp, err := httpDoWithRetry(ctx, c.HTTPClient, factory, c.RetryConfig)
	if err != nil {
		return nil, fmt.Errorf("grok request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("grok API %d: %s", resp.StatusCode, string(data))
	}

	if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		return decodeEventStreamSearchResult(resp.Body)
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

// responsesAPIResponse mirrors the xAI Responses API response schema.
type responsesAPIResponse struct {
	// Output is an ordered list of response items (messages, tool calls, reasoning).
	Output []struct {
		Type    string `json:"type"`
		Role    string `json:"role"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
			// Annotations live on output_text content blocks.
			Annotations []struct {
				Type  string `json:"type"`
				URL   string `json:"url"`
				Title string `json:"title"`
			} `json:"annotations"`
		} `json:"content"`
	} `json:"output"`
	// Citations is the top-level fallback used when annotation-level citations
	// are absent (mirrors native xAI chat completions behaviour).
	Citations []string `json:"citations"`
}

// searchViaResponses sends a query to the xAI Responses API endpoint (/v1/responses).
// It uses the OpenAI Responses API request/response shape which differs from
// /v1/chat/completions: "input" instead of "messages", "output" instead of "choices".
func (c *GrokClient) searchViaResponses(ctx context.Context, query string) (*SearchResult, error) {
	body := map[string]any{
		"model": c.Model,
		"input": []map[string]string{
			{"role": "user", "content": query},
		},
		"store": false,
	}
	if c.SendSearchFlag {
		body["tools"] = []map[string]any{
			{"type": "web_search"},
		}
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	factory := func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			c.BaseURL+"/responses", bytes.NewReader(jsonBody))
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
		return req, nil
	}

	resp, err := httpDoWithRetry(ctx, c.HTTPClient, factory, c.RetryConfig)
	if err != nil {
		return nil, fmt.Errorf("grok responses request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("grok responses API %d: %s", resp.StatusCode, string(data))
	}

	if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		return nil, fmt.Errorf("grok responses API returned streaming response; streaming not yet supported for apiType=responses")
	}

	var raw responsesAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode responses API response: %w", err)
	}

	content, urls := extractResponsesAPIResult(&raw)

	return &SearchResult{
		Content:      content,
		SourceURLs:   urls,
		SourcesCount: len(urls),
	}, nil
}

// extractResponsesAPIResult extracts text content and source URLs from a
// Responses API response. It concatenates all output_text blocks and collects
// citations in this priority order:
//  1. annotations on output_text content blocks
//  2. top-level citations[]
//  3. http(s) URLs scraped from the combined text (regex fallback)
func extractResponsesAPIResult(raw *responsesAPIResponse) (string, []string) {
	var textParts []string
	var annotationURLs []string

	for _, item := range raw.Output {
		if item.Type != "message" {
			continue
		}
		for _, block := range item.Content {
			if block.Type != "output_text" {
				continue
			}
			if block.Text != "" {
				textParts = append(textParts, block.Text)
			}
			for _, ann := range block.Annotations {
				if ann.Type == "url_citation" && ann.URL != "" {
					annotationURLs = append(annotationURLs, ann.URL)
				}
			}
		}
	}

	content := strings.Join(textParts, "\n")

	if len(annotationURLs) > 0 {
		return content, dedupURLs(annotationURLs)
	}
	if len(raw.Citations) > 0 {
		return content, dedupURLs(raw.Citations)
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
			return content, dedupURLs(cleaned)
		}
	}
	return content, nil
}

type grokStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		Message struct {
			Content     string `json:"content"`
			Annotations []struct {
				Type        string `json:"type"`
				URLCitation struct {
					URL   string `json:"url"`
					Title string `json:"title"`
				} `json:"url_citation"`
			} `json:"annotations"`
		} `json:"message"`
	} `json:"choices"`
	Citations     []string `json:"citations"`
	SearchSources []struct {
		URL   string `json:"url"`
		Title string `json:"title"`
		Type  string `json:"type"`
	} `json:"search_sources"`
	SearchResults []struct {
		URL string `json:"url"`
	} `json:"search_results"`
}

func decodeEventStreamSearchResult(r io.Reader) (*SearchResult, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)

	var content strings.Builder
	var raw grokRawResponse
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}

		var chunk grokStreamChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			return nil, fmt.Errorf("decode event stream chunk: %w", err)
		}
		mergeStreamChunk(&raw, &content, &chunk)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read event stream: %w", err)
	}

	text := content.String()
	if text == "" && len(raw.Choices) > 0 {
		text = raw.Choices[0].Message.Content
	}
	urls := extractSourceURLs(&raw, text)
	return &SearchResult{
		Content:      text,
		SourceURLs:   urls,
		SourcesCount: len(urls),
	}, nil
}

func mergeStreamChunk(raw *grokRawResponse, content *strings.Builder, chunk *grokStreamChunk) {
	if len(chunk.Choices) > 0 {
		if len(raw.Choices) == 0 {
			raw.Choices = append(raw.Choices, struct {
				Message struct {
					Content     string `json:"content"`
					Annotations []struct {
						Type        string `json:"type"`
						URLCitation struct {
							URL   string `json:"url"`
							Title string `json:"title"`
						} `json:"url_citation"`
					} `json:"annotations"`
				} `json:"message"`
			}{})
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				content.WriteString(choice.Delta.Content)
			}
			if choice.Message.Content != "" {
				raw.Choices[0].Message.Content += choice.Message.Content
			}
			if len(choice.Message.Annotations) > 0 {
				raw.Choices[0].Message.Annotations = append(raw.Choices[0].Message.Annotations, choice.Message.Annotations...)
			}
		}
	}
	if len(chunk.Citations) > 0 {
		raw.Citations = append(raw.Citations, chunk.Citations...)
	}
	if len(chunk.SearchSources) > 0 {
		raw.SearchSources = append(raw.SearchSources, chunk.SearchSources...)
	}
	if len(chunk.SearchResults) > 0 {
		raw.SearchResults = append(raw.SearchResults, chunk.SearchResults...)
	}
}

// extractSourceURLs picks the best available source list from a Grok response.
//
// Priority order (most-structured first):
//  1. choices[0].message.annotations[].url_citation.url  (OpenAI tools-spec; many grok2api flavors).
//  2. top-level search_sources[].url                     (grok2api wykon/yyds flavor).
//  3. top-level citations[]                              (native xAI Grok).
//  4. top-level search_results[].url                     (older grok2api flavor).
//  5. http(s) URLs scraped from the answer text          (last-resort regex).
func extractSourceURLs(raw *grokRawResponse, content string) []string {
	if len(raw.Choices) > 0 {
		anns := raw.Choices[0].Message.Annotations
		if len(anns) > 0 {
			urls := make([]string, 0, len(anns))
			for _, a := range anns {
				if a.URLCitation.URL != "" {
					urls = append(urls, a.URLCitation.URL)
				}
			}
			if len(urls) > 0 {
				return dedupURLs(urls)
			}
		}
	}
	if len(raw.SearchSources) > 0 {
		urls := make([]string, 0, len(raw.SearchSources))
		for _, s := range raw.SearchSources {
			if s.URL != "" {
				urls = append(urls, s.URL)
			}
		}
		if len(urls) > 0 {
			return dedupURLs(urls)
		}
	}
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
// Transient errors (network failures, HTTP 429, 5xx) are retried per
// c.RetryConfig, mirroring Search.
func (c *GrokClient) ListModels(ctx context.Context) ([]string, error) {
	factory := func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet,
			c.BaseURL+"/models", nil)
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
		return req, nil
	}

	resp, err := httpDoWithRetry(ctx, c.HTTPClient, factory, c.RetryConfig)
	if err != nil {
		return nil, fmt.Errorf("grok list models failed: %w", err)
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
