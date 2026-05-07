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
	Title         string         `json:"title"`
	URL           string         `json:"url"`
	ID            string         `json:"id"`
	PublishedDate string         `json:"publishedDate"`
	Author        string         `json:"author"`
	Text          string         `json:"text"`
	Summary       string         `json:"summary"`
	Highlights    []string       `json:"highlights"`
	Subpages      []ExaSearchHit `json:"subpages,omitempty"`
}

// ExaSearchResult is the response shape for Exa Search.
type ExaSearchResult struct {
	RequestID  string               `json:"requestId"`
	Results    []ExaSearchHit       `json:"results"`
	SearchType string               `json:"searchType"`
	Context    string               `json:"context"`
	Output     *ExaStructuredOutput `json:"output,omitempty"`
}

// ExaSearchTextOptions controls returned full-text content.
type ExaSearchTextOptions struct {
	Enabled       bool
	MaxCharacters int
}

// ExaHighlightsOptions controls returned highlight snippets.
type ExaHighlightsOptions struct {
	Enabled       bool
	Query         string
	MaxCharacters int
}

// ExaSearchRequest exposes the advanced knobs for /search without changing
// the default fallback route used by web_search.
type ExaSearchRequest struct {
	Query        string
	Type         string
	NumResults   int
	Text         ExaSearchTextOptions
	Highlights   ExaHighlightsOptions
	SystemPrompt string
	OutputSchema map[string]any
}

// ExaContentsRequest exposes selected advanced knobs for /contents.
type ExaContentsRequest struct {
	URL           string
	Text          ExaSearchTextOptions
	Highlights    ExaHighlightsOptions
	Subpages      int
	SubpageTarget []string
	MaxAgeHours   *int
}

// ExaStructuredOutput is the synthesized structured output returned when
// outputSchema is supplied to Exa Search.
type ExaStructuredOutput struct {
	Content   any            `json:"content"`
	Grounding []ExaGrounding `json:"grounding,omitempty"`
}

type ExaGrounding struct {
	Field      string        `json:"field"`
	Citations  []ExaCitation `json:"citations,omitempty"`
	Confidence string        `json:"confidence,omitempty"`
}

type ExaCitation struct {
	URL   string `json:"url"`
	Title string `json:"title"`
}

// ExaContentsResult is the raw /contents response, including any subpages.
type ExaContentsResult struct {
	RequestID string          `json:"requestId"`
	Results   []ExaSearchHit  `json:"results"`
	Statuses  []ExaStatusItem `json:"statuses"`
}

type ExaStatusItem struct {
	ID     string         `json:"id"`
	Status string         `json:"status"`
	Error  ExaStatusError `json:"error"`
}

type ExaStatusError struct {
	Tag            string `json:"tag"`
	HTTPStatusCode int    `json:"httpStatusCode"`
}

// Search performs an Exa web search. It asks for highlights rather than full
// text to keep fallback search cheap and source-focused.
func (c *ExaClient) Search(ctx context.Context, query string) (*ExaSearchResult, error) {
	return c.SearchAdvanced(ctx, ExaSearchRequest{
		Query:      query,
		Type:       "auto",
		NumResults: 5,
		Highlights: ExaHighlightsOptions{Enabled: true},
	})
}

// SearchAdvanced exposes a controlled subset of Exa's advanced search
// options without changing the default web_search fallback behavior.
func (c *ExaClient) SearchAdvanced(ctx context.Context, req ExaSearchRequest) (*ExaSearchResult, error) {
	if strings.TrimSpace(req.Query) == "" {
		return nil, fmt.Errorf("exa search: empty query")
	}
	body := buildExaSearchBody(req)
	searchType := normalizedExaSearchType(req.Type)

	var result ExaSearchResult
	if err := c.postJSON(ctx, "/search", body, &result); err != nil {
		return nil, fmt.Errorf("exa search failed: %w", err)
	}
	if result.SearchType == "" {
		result.SearchType = searchType
	}
	if len(result.Results) == 0 && result.Context == "" && (result.Output == nil || result.Output.Content == nil) {
		return nil, fmt.Errorf("exa search returned empty result for %q", req.Query)
	}
	return &result, nil
}

// Extract fetches and extracts content from a URL using Exa Contents API.
func (c *ExaClient) Extract(ctx context.Context, url string) (*ExtractResult, error) {
	result, err := c.ContentsAdvanced(ctx, ExaContentsRequest{
		URL:  url,
		Text: ExaSearchTextOptions{Enabled: true},
	})
	if err != nil {
		return nil, err
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

// ContentsAdvanced exposes selected /contents options such as subpages and
// cache freshness control, while keeping the default web_fetch route intact.
func (c *ExaClient) ContentsAdvanced(ctx context.Context, req ExaContentsRequest) (*ExaContentsResult, error) {
	if strings.TrimSpace(req.URL) == "" {
		return nil, fmt.Errorf("exa contents: empty url")
	}
	body := buildExaContentsBody(req)

	var result ExaContentsResult
	if err := c.postJSON(ctx, "/contents", body, &result); err != nil {
		return nil, fmt.Errorf("exa contents failed: %w", err)
	}
	if len(result.Results) == 0 {
		if len(result.Statuses) > 0 && result.Statuses[0].Status == "error" {
			st := result.Statuses[0]
			return nil, fmt.Errorf("exa contents returned no result for %s: %s (%d)", req.URL, st.Error.Tag, st.Error.HTTPStatusCode)
		}
		return nil, fmt.Errorf("exa contents returned no result for %s", req.URL)
	}
	return &result, nil
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
	if res.Context != "" && len(res.Results) == 0 && (res.Output == nil || res.Output.Content == nil) {
		return res.Context
	}

	var sb strings.Builder
	if res.Output != nil && res.Output.Content != nil {
		sb.WriteString("Exa structured output:\n")
		sb.WriteString(formatExaStructuredContent(res.Output.Content))
		if len(res.Output.Grounding) > 0 {
			sb.WriteString("\n\nGrounding:\n")
			for _, grounding := range res.Output.Grounding {
				if grounding.Field != "" {
					fmt.Fprintf(&sb, "- %s", grounding.Field)
				} else {
					sb.WriteString("- field")
				}
				if grounding.Confidence != "" {
					fmt.Fprintf(&sb, " (%s)", grounding.Confidence)
				}
				if len(grounding.Citations) > 0 {
					sb.WriteString(": ")
					for i, citation := range grounding.Citations {
						if i > 0 {
							sb.WriteString(", ")
						}
						title := citation.Title
						if title == "" {
							title = citation.URL
						}
						sb.WriteString(title)
					}
				}
				sb.WriteString("\n")
			}
		}
		if len(res.Results) > 0 {
			sb.WriteString("\n")
		}
	}
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

// FormatExaContentsContent renders advanced /contents results in a compact
// MCP/CLI-friendly form, including any returned subpages.
func FormatExaContentsContent(res *ExaContentsResult, maxChars, subpageMaxChars int) string {
	if res == nil {
		return ""
	}
	if maxChars <= 0 {
		maxChars = 1800
	}
	if subpageMaxChars <= 0 {
		subpageMaxChars = 500
	}

	var sb strings.Builder
	for i, hit := range res.Results {
		title := hit.Title
		if title == "" {
			title = hit.URL
		}
		fmt.Fprintf(&sb, "%d. %s\n", i+1, title)
		if hit.URL != "" {
			fmt.Fprintf(&sb, "   URL: %s\n", hit.URL)
		}
		if snippet := compactExaSnippet(hit, maxChars); snippet != "" {
			fmt.Fprintf(&sb, "   Content: %s\n", snippet)
		}
		if len(hit.Subpages) > 0 {
			sb.WriteString("   Subpages:\n")
			for _, subpage := range hit.Subpages {
				subTitle := subpage.Title
				if subTitle == "" {
					subTitle = subpage.URL
				}
				fmt.Fprintf(&sb, "   - %s\n", subTitle)
				if subpage.URL != "" {
					fmt.Fprintf(&sb, "     URL: %s\n", subpage.URL)
				}
				if snippet := compactExaSnippet(subpage, subpageMaxChars); snippet != "" {
					fmt.Fprintf(&sb, "     Content: %s\n", snippet)
				}
			}
		}
		if i < len(res.Results)-1 {
			sb.WriteString("\n")
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

func buildExaSearchBody(req ExaSearchRequest) map[string]any {
	searchType := normalizedExaSearchType(req.Type)
	numResults := req.NumResults
	if numResults <= 0 {
		numResults = 5
	}
	highlights := req.Highlights
	if !req.Text.Enabled && !highlights.Enabled && strings.TrimSpace(highlights.Query) == "" {
		highlights.Enabled = true
	}

	body := map[string]any{
		"query":      strings.TrimSpace(req.Query),
		"type":       searchType,
		"numResults": numResults,
	}
	contents := make(map[string]any)
	if v, ok := buildExaTextField(req.Text); ok {
		contents["text"] = v
	}
	if v, ok := buildExaHighlightsField(highlights); ok {
		contents["highlights"] = v
	}
	if len(contents) > 0 {
		body["contents"] = contents
	}
	if strings.TrimSpace(req.SystemPrompt) != "" {
		body["systemPrompt"] = strings.TrimSpace(req.SystemPrompt)
	}
	if len(req.OutputSchema) > 0 {
		body["outputSchema"] = req.OutputSchema
	}
	return body
}

func normalizedExaSearchType(raw string) string {
	searchType := strings.TrimSpace(raw)
	if searchType == "" {
		return "auto"
	}
	return searchType
}

func buildExaContentsBody(req ExaContentsRequest) map[string]any {
	text := req.Text
	highlights := req.Highlights
	if !text.Enabled && !highlights.Enabled && strings.TrimSpace(highlights.Query) == "" {
		text.Enabled = true
	}

	body := map[string]any{
		"urls": []string{strings.TrimSpace(req.URL)},
	}
	if v, ok := buildExaTextField(text); ok {
		body["text"] = v
	}
	if v, ok := buildExaHighlightsField(highlights); ok {
		body["highlights"] = v
	}
	if req.Subpages > 0 {
		body["subpages"] = req.Subpages
	}
	if len(req.SubpageTarget) > 0 {
		body["subpageTarget"] = req.SubpageTarget
	}
	if req.MaxAgeHours != nil {
		body["maxAgeHours"] = *req.MaxAgeHours
	}
	return body
}

func buildExaTextField(opts ExaSearchTextOptions) (any, bool) {
	if !opts.Enabled {
		return nil, false
	}
	if opts.MaxCharacters > 0 {
		return map[string]any{"maxCharacters": opts.MaxCharacters}, true
	}
	return true, true
}

func buildExaHighlightsField(opts ExaHighlightsOptions) (any, bool) {
	if !opts.Enabled && strings.TrimSpace(opts.Query) == "" {
		return nil, false
	}
	if strings.TrimSpace(opts.Query) != "" || opts.MaxCharacters > 0 {
		field := map[string]any{}
		if strings.TrimSpace(opts.Query) != "" {
			field["query"] = strings.TrimSpace(opts.Query)
		}
		if opts.MaxCharacters > 0 {
			field["maxCharacters"] = opts.MaxCharacters
		}
		return field, true
	}
	return true, true
}

func formatExaStructuredContent(content any) string {
	data, err := json.MarshalIndent(content, "", "  ")
	if err != nil {
		return fmt.Sprint(content)
	}
	return string(data)
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
