package engine

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	// ResponseToolWebSearch enables xAI's built-in real-time web search tool on
	// the Responses API.
	ResponseToolWebSearch = "web_search"
	// ResponseToolXSearch enables xAI's built-in X/Twitter search tool on the
	// Responses API.
	ResponseToolXSearch = "x_search"
)

var allowedResponseTools = map[string]struct{}{
	ResponseToolWebSearch: {},
	ResponseToolXSearch:   {},
}

// GrokClient wraps calls to a Grok-compatible (OpenAI-format) API with web search.
type GrokClient struct {
	Name    string
	BaseURL string
	APIKey  string
	Model   string
	Profile string
	// APIType selects the request/response protocol:
	//   ""  or "chat"      → POST /v1/chat/completions  (default, all grok2api proxies)
	//   "responses"        → POST /v1/responses         (xAI Responses API, native xAI endpoints)
	APIType string
	// SendSearchFlag controls search activation. The exact mechanism differs by
	// APIType: "chat" sends "search":true; "responses" appends built-in tools.
	// Many grok2api proxies auto-search and either ignore or reject the flag, so
	// it's configurable per-endpoint.
	SendSearchFlag bool
	// ResponseTools selects built-in Responses API tools when SendSearchFlag is
	// true. Empty keeps the legacy default of web_search only.
	ResponseTools []string
	HTTPClient    *http.Client
	// RetryConfig governs httpDoWithRetry behaviour: 429/5xx + network errors are
	// retried with capped exponential backoff, honouring any Retry-After header.
	RetryConfig RetryConfig
}

func (c *GrokClient) EffectiveProfile() string {
	if c == nil {
		return DefaultGrokEndpointProfile
	}
	return normalizeGrokEndpointProfile(c.Profile)
}

// NewGrokClient creates a Grok client whose requests are bounded by the caller
// context and Grok pool budget, with search enabled by default.
func NewGrokClient(baseURL, apiKey, model string) *GrokClient {
	return &GrokClient{
		Name:           "grok",
		BaseURL:        baseURL,
		APIKey:         apiKey,
		Model:          model,
		SendSearchFlag: true,
		HTTPClient:     &http.Client{},
		RetryConfig:    DefaultRetryConfig(),
	}
}

// SearchResult holds the AI answer plus extracted source URLs.
type SearchResult struct {
	Content                string   `json:"content"`
	SourceURLs             []string `json:"source_urls"`
	SourcesCount           int      `json:"sources_count"`
	SourceOrigin           string   `json:"source_origin"`
	ValidSourcesCount      int      `json:"valid_sources_count"`
	StructuredSourcesCount int      `json:"structured_sources_count"`
}

type sourceEvidence struct {
	URLs            []string
	Origin          string
	StructuredCount int
}

func newSearchResult(content string, evidence sourceEvidence) *SearchResult {
	return &SearchResult{
		Content:                content,
		SourceURLs:             evidence.URLs,
		SourcesCount:           len(evidence.URLs),
		SourceOrigin:           evidence.Origin,
		ValidSourcesCount:      len(evidence.URLs),
		StructuredSourcesCount: evidence.StructuredCount,
	}
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
		return nil, fmt.Errorf("grok API %d: %s", resp.StatusCode, c.redactedBody(data))
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

	return newSearchResult(content, extractSourceEvidence(&raw, content)), nil
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
		tools, err := NormalizeResponseTools(c.ResponseTools)
		if err != nil {
			return nil, fmt.Errorf("normalize response tools: %w", err)
		}
		body["tools"] = responseToolPayload(EffectiveResponseTools(tools))
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
		return nil, fmt.Errorf("grok responses API %d: %s", resp.StatusCode, c.redactedBody(data))
	}

	if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		return nil, fmt.Errorf("grok responses API returned streaming response; streaming not yet supported for apiType=responses")
	}

	var raw responsesAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode responses API response: %w", err)
	}

	content, evidence := extractResponsesAPIResult(&raw)

	return newSearchResult(content, evidence), nil
}

// NormalizeResponseTools trims, validates, and deduplicates Responses API tool
// names while preserving first-seen order.
func NormalizeResponseTools(tools []string) ([]string, error) {
	if len(tools) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(tools))
	seen := make(map[string]struct{}, len(tools))
	for i, tool := range tools {
		tool = strings.TrimSpace(tool)
		if tool == "" {
			return nil, fmt.Errorf("responseTools[%d] is empty", i)
		}
		if _, ok := allowedResponseTools[tool]; !ok {
			return nil, fmt.Errorf("responseTools[%d] has unsupported tool %q (supported: %s, %s)",
				i, tool, ResponseToolWebSearch, ResponseToolXSearch)
		}
		if _, ok := seen[tool]; ok {
			continue
		}
		seen[tool] = struct{}{}
		out = append(out, tool)
	}
	return out, nil
}

// EffectiveResponseTools returns the Responses API tools to send for a
// search-enabled request. Empty config preserves the historical web_search-only
// behavior.
func EffectiveResponseTools(tools []string) []string {
	if len(tools) == 0 {
		return []string{ResponseToolWebSearch}
	}
	return append([]string(nil), tools...)
}

func responseToolPayload(tools []string) []map[string]any {
	payload := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		payload = append(payload, map[string]any{"type": tool})
	}
	return payload
}

// extractResponsesAPIResult extracts text content and source URLs from a
// Responses API response. It concatenates all output_text blocks and collects
// citations in this priority order:
//  1. annotations on output_text content blocks
//  2. top-level citations[]
//  3. http(s) URLs scraped from the combined text (regex fallback)
func extractResponsesAPIResult(raw *responsesAPIResponse) (string, sourceEvidence) {
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
		if evidence := structuredSourceEvidence(annotationURLs, SourceOriginAnnotations); len(evidence.URLs) > 0 {
			return content, evidence
		}
	}
	if len(raw.Citations) > 0 {
		if evidence := structuredSourceEvidence(raw.Citations, SourceOriginCitations); len(evidence.URLs) > 0 {
			return content, evidence
		}
	}
	if content != "" {
		if evidence := regexSourceEvidence(content); len(evidence.URLs) > 0 {
			return content, evidence
		}
	}
	return content, sourceEvidence{Origin: SourceOriginNone}
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
	return newSearchResult(text, extractSourceEvidence(&raw, text)), nil
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

// extractSourceEvidence picks the best valid source list from a Grok response.
//
// Priority order (most-structured first):
//  1. choices[0].message.annotations[].url_citation.url  (OpenAI tools-spec; many grok2api flavors).
//  2. top-level search_sources[].url                     (grok2api wykon/yyds flavor).
//  3. top-level citations[]                              (native xAI Grok).
//  4. top-level search_results[].url                     (older grok2api flavor).
//  5. http(s) URLs scraped from the answer text          (last-resort regex).
func extractSourceEvidence(raw *grokRawResponse, content string) sourceEvidence {
	if len(raw.Choices) > 0 {
		anns := raw.Choices[0].Message.Annotations
		if len(anns) > 0 {
			urls := make([]string, 0, len(anns))
			for _, a := range anns {
				if a.URLCitation.URL != "" {
					urls = append(urls, a.URLCitation.URL)
				}
			}
			if evidence := structuredSourceEvidence(urls, SourceOriginAnnotations); len(evidence.URLs) > 0 {
				return evidence
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
		if evidence := structuredSourceEvidence(urls, SourceOriginSearchSources); len(evidence.URLs) > 0 {
			return evidence
		}
	}
	if len(raw.Citations) > 0 {
		if evidence := structuredSourceEvidence(raw.Citations, SourceOriginCitations); len(evidence.URLs) > 0 {
			return evidence
		}
	}
	if len(raw.SearchResults) > 0 {
		urls := make([]string, 0, len(raw.SearchResults))
		for _, r := range raw.SearchResults {
			if r.URL != "" {
				urls = append(urls, r.URL)
			}
		}
		if evidence := structuredSourceEvidence(urls, SourceOriginSearchResults); len(evidence.URLs) > 0 {
			return evidence
		}
	}
	if content != "" {
		if evidence := regexSourceEvidence(content); len(evidence.URLs) > 0 {
			return evidence
		}
	}
	return sourceEvidence{Origin: SourceOriginNone}
}

func structuredSourceEvidence(urls []string, origin string) sourceEvidence {
	normalized := NormalizeSourceURLs(urls)
	return sourceEvidence{URLs: normalized, Origin: origin, StructuredCount: len(normalized)}
}

func regexSourceEvidence(content string) sourceEvidence {
	urls := NormalizeSourceURLs(urlRegex.FindAllString(content, -1))
	return sourceEvidence{URLs: urls, Origin: SourceOriginRegex}
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
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("grok models API %d: %s", resp.StatusCode, c.redactedBody(data))
	}

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

func (c *GrokClient) redactedBody(data []byte) string {
	if key := strings.TrimSpace(c.APIKey); key != "" {
		data = bytes.ReplaceAll(data, []byte(key), []byte("<redacted>"))
	}
	const limit = 500
	body := strings.TrimSpace(string(data))
	if len(body) <= limit {
		return body
	}
	return body[:limit] + "..."
}
