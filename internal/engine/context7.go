package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const DefaultContext7APIURL = "https://context7.com"

// Context7Endpoint configures one named Context7 REST API instance.
type Context7Endpoint struct {
	Name            string   `json:"name"`
	APIURL          string   `json:"apiURL,omitempty"`
	APIKey          string   `json:"apiKey,omitempty"`
	Priority        int      `json:"priority,omitempty"`
	LibraryScopes   []string `json:"library_scopes,omitempty"`
	MonthlyBudget   int      `json:"monthly_budget,omitempty"`
	CooldownSeconds int      `json:"cooldown_on_rate_limit_seconds,omitempty"`
}

// Context7Client wraps Context7's REST API. It intentionally does not use the
// Context7 MCP server surface; this project needs reusable CLI/MCP plumbing.
type Context7Client struct {
	Endpoint    Context7Endpoint
	BaseURL     string
	APIKey      string
	HTTPClient  *http.Client
	RetryConfig RetryConfig
}

func NewContext7Client(endpoint Context7Endpoint) *Context7Client {
	baseURL := strings.TrimRight(endpoint.APIURL, "/")
	if baseURL == "" {
		baseURL = DefaultContext7APIURL
	}
	return &Context7Client{
		Endpoint: endpoint,
		BaseURL:  baseURL,
		APIKey:   strings.TrimSpace(endpoint.APIKey),
		HTTPClient: &http.Client{
			Timeout: 60 * time.Second,
		},
		RetryConfig: DefaultRetryConfig(),
	}
}

func (c *Context7Client) Name() string {
	if c == nil {
		return ""
	}
	name := strings.TrimSpace(c.Endpoint.Name)
	if name == "" {
		return "context7"
	}
	return name
}

type Context7LibrarySearchRequest struct {
	LibraryName string
	Query       string
	Fast        bool
}

type Context7LibrarySearchResult struct {
	Results             []Context7Library `json:"results"`
	SearchFilterApplied bool              `json:"searchFilterApplied"`
}

type Context7Library struct {
	ID             string   `json:"id"`
	Title          string   `json:"title"`
	Description    string   `json:"description"`
	Branch         string   `json:"branch"`
	LastUpdateDate string   `json:"lastUpdateDate"`
	State          string   `json:"state"`
	TotalTokens    int      `json:"totalTokens"`
	TotalSnippets  int      `json:"totalSnippets"`
	Stars          int      `json:"stars"`
	TrustScore     float64  `json:"trustScore"`
	BenchmarkScore float64  `json:"benchmarkScore"`
	Versions       []string `json:"versions"`
}

type Context7DocsRequest struct {
	LibraryID string
	Query     string
	Type      string
	Fast      bool
}

type Context7DocsResult struct {
	CodeSnippets []Context7CodeSnippet `json:"codeSnippets"`
	InfoSnippets []Context7InfoSnippet `json:"infoSnippets"`
	Rules        map[string]any        `json:"rules,omitempty"`
}

type Context7CodeSnippet struct {
	CodeTitle       string             `json:"codeTitle"`
	CodeDescription string             `json:"codeDescription"`
	CodeLanguage    string             `json:"codeLanguage"`
	CodeTokens      int                `json:"codeTokens"`
	CodeID          string             `json:"codeId"`
	PageTitle       string             `json:"pageTitle"`
	CodeList        []Context7CodeItem `json:"codeList"`
}

type Context7CodeItem struct {
	Language string `json:"language"`
	Code     string `json:"code"`
}

type Context7InfoSnippet struct {
	PageID        string `json:"pageId"`
	Breadcrumb    string `json:"breadcrumb"`
	Content       string `json:"content"`
	ContentTokens int    `json:"contentTokens"`
}

func (c *Context7Client) SearchLibraries(ctx context.Context, req Context7LibrarySearchRequest) (*Context7LibrarySearchResult, error) {
	if strings.TrimSpace(req.LibraryName) == "" {
		return nil, fmt.Errorf("context7 library search: library name is required")
	}
	if strings.TrimSpace(req.Query) == "" {
		return nil, fmt.Errorf("context7 library search: query is required")
	}
	values := url.Values{}
	values.Set("libraryName", strings.TrimSpace(req.LibraryName))
	values.Set("query", strings.TrimSpace(req.Query))
	values.Set("fast", boolString(req.Fast))

	var result Context7LibrarySearchResult
	if err := c.getJSON(ctx, "/api/v2/libs/search", values, &result); err != nil {
		return nil, fmt.Errorf("context7 library search failed: %w", err)
	}
	if len(result.Results) == 0 {
		return nil, fmt.Errorf("context7 library search returned no results for %q", req.LibraryName)
	}
	return &result, nil
}

func (c *Context7Client) GetDocs(ctx context.Context, req Context7DocsRequest) (*Context7DocsResult, error) {
	if strings.TrimSpace(req.LibraryID) == "" {
		return nil, fmt.Errorf("context7 docs: library id is required")
	}
	if strings.TrimSpace(req.Query) == "" {
		return nil, fmt.Errorf("context7 docs: query is required")
	}
	values := url.Values{}
	values.Set("libraryId", strings.TrimSpace(req.LibraryID))
	values.Set("query", strings.TrimSpace(req.Query))
	values.Set("type", normalizedContext7Type(req.Type))
	values.Set("fast", boolString(req.Fast))

	var result Context7DocsResult
	if err := c.getJSON(ctx, "/api/v2/context", values, &result); err != nil {
		return nil, fmt.Errorf("context7 docs failed: %w", err)
	}
	if len(result.CodeSnippets) == 0 && len(result.InfoSnippets) == 0 && len(result.Rules) == 0 {
		return nil, fmt.Errorf("context7 docs returned empty result for %s", req.LibraryID)
	}
	return &result, nil
}

func (c *Context7Client) getJSON(ctx context.Context, path string, values url.Values, out any) error {
	if c == nil {
		return fmt.Errorf("context7 client is nil")
	}
	if c.BaseURL == "" {
		c.BaseURL = DefaultContext7APIURL
	}
	factory := func() (*http.Request, error) {
		u := c.BaseURL + path
		if encoded := values.Encode(); encoded != "" {
			u += "?" + encoded
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Accept", "application/json")
		if key := strings.TrimSpace(c.APIKey); key != "" {
			req.Header.Set("Authorization", "Bearer "+key)
		}
		return req, nil
	}

	resp, err := httpDoWithRetry(ctx, c.HTTPClient, factory, c.RetryConfig)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		body := redactSecret(strings.TrimSpace(string(data)), c.APIKey)
		return HTTPStatusError{Provider: "context7", StatusCode: resp.StatusCode, Body: body}
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode: %w", err)
	}
	return nil
}

func FormatContext7LibrariesContent(res *Context7LibrarySearchResult) string {
	if res == nil {
		return ""
	}
	var sb strings.Builder
	if res.SearchFilterApplied {
		sb.WriteString("Context7 search filter applied by teamspace settings.\n\n")
	}
	for i, lib := range res.Results {
		title := lib.Title
		if title == "" {
			title = lib.ID
		}
		fmt.Fprintf(&sb, "%d. %s\n", i+1, title)
		if lib.ID != "" {
			fmt.Fprintf(&sb, "   ID: %s\n", lib.ID)
		}
		if lib.Description != "" {
			fmt.Fprintf(&sb, "   Description: %s\n", clipString(lib.Description, 240))
		}
		if lib.TrustScore != 0 || lib.BenchmarkScore != 0 {
			fmt.Fprintf(&sb, "   Scores: trust=%.1f benchmark=%.1f\n", lib.TrustScore, lib.BenchmarkScore)
		}
		if len(lib.Versions) > 0 {
			fmt.Fprintf(&sb, "   Versions: %s\n", strings.Join(lib.Versions, ", "))
		}
	}
	return strings.TrimSpace(sb.String())
}

func FormatContext7DocsContent(res *Context7DocsResult, maxSnippetChars int) string {
	if res == nil {
		return ""
	}
	if maxSnippetChars <= 0 {
		maxSnippetChars = 1200
	}
	var sb strings.Builder
	if len(res.CodeSnippets) > 0 {
		sb.WriteString("Code snippets:\n")
		for i, snippet := range res.CodeSnippets {
			title := firstNonEmpty(snippet.CodeTitle, snippet.PageTitle, snippet.CodeID)
			fmt.Fprintf(&sb, "%d. %s\n", i+1, title)
			if snippet.CodeDescription != "" {
				fmt.Fprintf(&sb, "   Description: %s\n", clipString(snippet.CodeDescription, 300))
			}
			if snippet.CodeID != "" {
				fmt.Fprintf(&sb, "   Source: %s\n", snippet.CodeID)
			}
			for _, item := range snippet.CodeList {
				code := strings.TrimSpace(item.Code)
				if code == "" {
					continue
				}
				lang := firstNonEmpty(item.Language, snippet.CodeLanguage, "text")
				fmt.Fprintf(&sb, "   ```%s\n%s\n   ```\n", lang, clipString(code, maxSnippetChars))
				break
			}
		}
	}
	if len(res.InfoSnippets) > 0 {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("Documentation snippets:\n")
		for i, snippet := range res.InfoSnippets {
			title := firstNonEmpty(snippet.Breadcrumb, snippet.PageID, "snippet")
			fmt.Fprintf(&sb, "%d. %s\n", i+1, title)
			if snippet.PageID != "" {
				fmt.Fprintf(&sb, "   Source: %s\n", snippet.PageID)
			}
			if snippet.Content != "" {
				fmt.Fprintf(&sb, "   Content: %s\n", clipString(snippet.Content, maxSnippetChars))
			}
		}
	}
	return strings.TrimSpace(sb.String())
}

func Context7DocsSourceURLs(res *Context7DocsResult) []string {
	if res == nil {
		return nil
	}
	var urls []string
	for _, snippet := range res.CodeSnippets {
		if snippet.CodeID != "" {
			urls = append(urls, snippet.CodeID)
		}
	}
	for _, snippet := range res.InfoSnippets {
		if snippet.PageID != "" {
			urls = append(urls, snippet.PageID)
		}
	}
	return dedupURLs(urls)
}

func SortContext7Endpoints(endpoints []Context7Endpoint) []Context7Endpoint {
	out := append([]Context7Endpoint(nil), endpoints...)
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Priority < out[j].Priority
	})
	return out
}

func normalizedContext7Type(value string) string {
	switch strings.TrimSpace(value) {
	case "txt":
		return "txt"
	default:
		return "json"
	}
}

func boolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func clipString(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	if maxRunes <= 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes]) + "..."
}

func redactSecret(value, secret string) string {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return value
	}
	return strings.ReplaceAll(value, secret, "[REDACTED]")
}
