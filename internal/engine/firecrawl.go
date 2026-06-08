package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const DefaultFirecrawlAPIURL = "https://api.firecrawl.dev/v2"

type FirecrawlKey = TinyFishKey

// FirecrawlClient wraps explicit Firecrawl v2 scraping and mapping APIs.
type FirecrawlClient struct {
	Name        string
	BaseURL     string
	APIKey      string
	HTTPClient  *http.Client
	RetryConfig RetryConfig
}

func NewFirecrawlClient(baseURL, apiKey string) *FirecrawlClient {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = DefaultFirecrawlAPIURL
	}
	return &FirecrawlClient{
		BaseURL: baseURL,
		APIKey:  strings.TrimSpace(apiKey),
		HTTPClient: &http.Client{
			Timeout: 90 * time.Second,
		},
		RetryConfig: DefaultRetryConfig(),
	}
}

// FirecrawlPool routes scrape/map calls across configured keys. Each request
// starts at a rotating key, then tries the remaining keys on upstream errors or
// empty provider results.
type FirecrawlPool struct {
	mu      sync.Mutex
	next    int
	clients []*FirecrawlClient
}

type FirecrawlPoolOptions struct {
	PerKeyTimeout time.Duration
}

func NewFirecrawlPool(keys []FirecrawlKey, baseURL string) *FirecrawlPool {
	clients := make([]*FirecrawlClient, 0, len(keys))
	for i, key := range keys {
		if strings.TrimSpace(key.APIKey) == "" {
			continue
		}
		c := NewFirecrawlClient(baseURL, key.APIKey)
		c.Name = strings.TrimSpace(key.Name)
		c.RetryConfig = RetryConfig{MaxAttempts: 1}
		if c.Name == "" {
			c.Name = fmt.Sprintf("key-%d", i)
		}
		clients = append(clients, c)
	}
	next := 0
	if len(clients) > 1 {
		next = int(time.Now().UnixNano() % int64(len(clients)))
	}
	return &FirecrawlPool{next: next, clients: clients}
}

func (p *FirecrawlPool) Len() int {
	if p == nil {
		return 0
	}
	return len(p.clients)
}

func (p *FirecrawlPool) Clients() []*FirecrawlClient {
	if p == nil {
		return nil
	}
	return p.clients
}

type FirecrawlPoolScrapeResult struct {
	*FirecrawlScrapeResult
	KeyName  string
	Attempts []FirecrawlPoolAttempt
}

type FirecrawlPoolMapResult struct {
	*FirecrawlMapResult
	KeyName string
}

type FirecrawlPoolAttempt struct {
	KeyName   string `json:"key"`
	Status    string `json:"status"`
	LatencyMS int64  `json:"latency_ms,omitempty"`
	Error     string `json:"error,omitempty"`
}

func (p *FirecrawlPool) Scrape(ctx context.Context, req FirecrawlScrapeRequest) (*FirecrawlPoolScrapeResult, error) {
	return p.ScrapeWithOptions(ctx, req, FirecrawlPoolOptions{})
}

func (p *FirecrawlPool) ScrapeWithOptions(ctx context.Context, req FirecrawlScrapeRequest, opts FirecrawlPoolOptions) (*FirecrawlPoolScrapeResult, error) {
	clients := p.orderedClients()
	if len(clients) == 0 {
		return nil, errors.New("firecrawl pool is empty: no keys configured")
	}
	var errs []string
	var attempts []FirecrawlPoolAttempt
	for _, c := range clients {
		attemptCtx, cancel := firecrawlPoolAttemptContext(ctx, opts.PerKeyTimeout)
		start := time.Now()
		res, err := c.Scrape(attemptCtx, req)
		cancel()
		latency := time.Since(start).Milliseconds()
		if err != nil {
			attempts = append(attempts, FirecrawlPoolAttempt{
				KeyName:   c.Name,
				Status:    "error",
				LatencyMS: latency,
				Error:     err.Error(),
			})
			errs = append(errs, fmt.Sprintf("%s: %v", c.Name, err))
			continue
		}
		if strings.TrimSpace(res.Data.Markdown) == "" {
			attempts = append(attempts, FirecrawlPoolAttempt{
				KeyName:   c.Name,
				Status:    "empty",
				LatencyMS: latency,
				Error:     "empty markdown",
			})
			errs = append(errs, fmt.Sprintf("%s: empty markdown", c.Name))
			continue
		}
		attempts = append(attempts, FirecrawlPoolAttempt{
			KeyName:   c.Name,
			Status:    "ok",
			LatencyMS: latency,
		})
		return &FirecrawlPoolScrapeResult{FirecrawlScrapeResult: res, KeyName: c.Name, Attempts: attempts}, nil
	}
	return &FirecrawlPoolScrapeResult{Attempts: attempts}, fmt.Errorf("all %d firecrawl keys failed: %s", len(clients), strings.Join(errs, "; "))
}

func firecrawlPoolAttemptContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return ctx, func() {}
	}
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return ctx, func() {}
		}
		if remaining < timeout {
			timeout = remaining
		}
	}
	return context.WithTimeout(ctx, timeout)
}

func (p *FirecrawlPool) Map(ctx context.Context, req FirecrawlMapRequest) (*FirecrawlPoolMapResult, error) {
	clients := p.orderedClients()
	if len(clients) == 0 {
		return nil, errors.New("firecrawl pool is empty: no keys configured")
	}
	var errs []string
	for _, c := range clients {
		res, err := c.Map(ctx, req)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", c.Name, err))
			continue
		}
		return &FirecrawlPoolMapResult{FirecrawlMapResult: res, KeyName: c.Name}, nil
	}
	return nil, fmt.Errorf("all %d firecrawl keys failed: %s", len(clients), strings.Join(errs, "; "))
}

func (p *FirecrawlPool) orderedClients() []*FirecrawlClient {
	if p == nil || len(p.clients) == 0 {
		return nil
	}
	p.mu.Lock()
	start := p.next % len(p.clients)
	p.next = (p.next + 1) % len(p.clients)
	p.mu.Unlock()

	ordered := make([]*FirecrawlClient, 0, len(p.clients))
	ordered = append(ordered, p.clients[start:]...)
	ordered = append(ordered, p.clients[:start]...)
	return ordered
}

type FirecrawlScrapeRequest struct {
	URL                string   `json:"url"`
	Formats            []string `json:"formats,omitempty"`
	OnlyMainContent    *bool    `json:"onlyMainContent,omitempty"`
	OnlyCleanContent   *bool    `json:"onlyCleanContent,omitempty"`
	IncludeTags        []string `json:"includeTags,omitempty"`
	ExcludeTags        []string `json:"excludeTags,omitempty"`
	WaitFor            int      `json:"waitFor,omitempty"`
	Timeout            int      `json:"timeout,omitempty"`
	Mobile             *bool    `json:"mobile,omitempty"`
	RemoveBase64Images *bool    `json:"removeBase64Images,omitempty"`
	BlockAds           *bool    `json:"blockAds,omitempty"`
	Proxy              string   `json:"proxy,omitempty"`
	StoreInCache       *bool    `json:"storeInCache,omitempty"`
	ZeroDataRetention  *bool    `json:"zeroDataRetention,omitempty"`
}

type FirecrawlScrapeMetadata struct {
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	SourceURL   string `json:"sourceURL,omitempty"`
	URL         string `json:"url,omitempty"`
	StatusCode  int    `json:"statusCode,omitempty"`
	ContentType string `json:"contentType,omitempty"`
	Error       string `json:"error,omitempty"`
}

type FirecrawlScrapeData struct {
	Markdown string                     `json:"markdown,omitempty"`
	Summary  string                     `json:"summary,omitempty"`
	HTML     string                     `json:"html,omitempty"`
	RawHTML  string                     `json:"rawHtml,omitempty"`
	Links    []string                   `json:"links,omitempty"`
	Metadata FirecrawlScrapeMetadata    `json:"metadata,omitempty"`
	Warning  string                     `json:"warning,omitempty"`
	Extra    map[string]json.RawMessage `json:"-"`
}

type FirecrawlScrapeResult struct {
	Success bool                `json:"success"`
	Data    FirecrawlScrapeData `json:"data"`
	Error   string              `json:"error,omitempty"`
	Raw     json.RawMessage     `json:"-"`
}

func (c *FirecrawlClient) Scrape(ctx context.Context, req FirecrawlScrapeRequest) (*FirecrawlScrapeResult, error) {
	if strings.TrimSpace(req.URL) == "" {
		return nil, fmt.Errorf("firecrawl scrape: empty url")
	}
	if len(req.Formats) == 0 {
		req.Formats = []string{"markdown"}
	}
	var result FirecrawlScrapeResult
	if err := c.postJSON(ctx, "/scrape", req, &result); err != nil {
		return nil, fmt.Errorf("firecrawl scrape failed: %w", err)
	}
	if !result.Success {
		if strings.TrimSpace(result.Error) != "" {
			return nil, fmt.Errorf("firecrawl scrape unsuccessful: %s", result.Error)
		}
		return nil, fmt.Errorf("firecrawl scrape unsuccessful")
	}
	if strings.TrimSpace(result.Data.Markdown) == "" {
		return nil, fmt.Errorf("firecrawl scrape returned empty markdown for %s", req.URL)
	}
	return &result, nil
}

type FirecrawlMapRequest struct {
	URL                   string `json:"url"`
	Search                string `json:"search,omitempty"`
	Sitemap               string `json:"sitemap,omitempty"`
	IncludeSubdomains     *bool  `json:"includeSubdomains,omitempty"`
	IgnoreQueryParameters *bool  `json:"ignoreQueryParameters,omitempty"`
	IgnoreCache           *bool  `json:"ignoreCache,omitempty"`
	Limit                 int    `json:"limit,omitempty"`
	Timeout               int    `json:"timeout,omitempty"`
}

type FirecrawlMapLink struct {
	URL         string `json:"url"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
}

type FirecrawlMapResult struct {
	Success bool               `json:"success"`
	Links   []FirecrawlMapLink `json:"links"`
	Error   string             `json:"error,omitempty"`
}

func (c *FirecrawlClient) Map(ctx context.Context, req FirecrawlMapRequest) (*FirecrawlMapResult, error) {
	if strings.TrimSpace(req.URL) == "" {
		return nil, fmt.Errorf("firecrawl map: empty url")
	}
	var result FirecrawlMapResult
	if err := c.postJSON(ctx, "/map", req, &result); err != nil {
		return nil, fmt.Errorf("firecrawl map failed: %w", err)
	}
	if !result.Success {
		if strings.TrimSpace(result.Error) != "" {
			return nil, fmt.Errorf("firecrawl map unsuccessful: %s", result.Error)
		}
		return nil, fmt.Errorf("firecrawl map unsuccessful")
	}
	if result.Links == nil {
		result.Links = []FirecrawlMapLink{}
	}
	return &result, nil
}

func (c *FirecrawlClient) postJSON(ctx context.Context, path string, body any, out any) error {
	if c == nil {
		return fmt.Errorf("firecrawl client is nil")
	}
	if strings.TrimSpace(c.BaseURL) == "" {
		return fmt.Errorf("firecrawl apiURL is empty")
	}
	if strings.TrimSpace(c.APIKey) == "" {
		return fmt.Errorf("firecrawl apiKey is empty")
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	factory := func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(jsonBody))
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
		return req, nil
	}
	resp, err := httpDoWithRetry(ctx, c.HTTPClient, factory, c.RetryConfig)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return HTTPStatusError{Provider: "firecrawl", StatusCode: resp.StatusCode, Body: c.redactSecret(data)}
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode: %w", err)
	}
	return nil
}

func (c *FirecrawlClient) redactSecret(data []byte) string {
	s := string(data)
	key := strings.TrimSpace(c.APIKey)
	if key == "" {
		return s
	}
	return strings.ReplaceAll(s, key, "<redacted>")
}

func FormatFirecrawlScrapeContent(res *FirecrawlScrapeResult, maxChars int) string {
	if res == nil {
		return ""
	}
	if maxChars <= 0 {
		maxChars = 1800
	}
	var sb strings.Builder
	md := strings.TrimSpace(res.Data.Markdown)
	if title := strings.TrimSpace(res.Data.Metadata.Title); title != "" {
		fmt.Fprintf(&sb, "title: %s\n", title)
	}
	if u := firecrawlResultURL(res); u != "" {
		fmt.Fprintf(&sb, "url: %s\n", u)
	}
	fmt.Fprintf(&sb, "content_chars: %d\n", len([]rune(md)))
	if len(res.Data.Links) > 0 {
		fmt.Fprintf(&sb, "links_count: %d\n", len(res.Data.Links))
	}
	if warning := strings.TrimSpace(res.Data.Warning); warning != "" {
		fmt.Fprintf(&sb, "warning: %s\n", warning)
	}
	if md != "" {
		fmt.Fprintf(&sb, "\nexcerpt:\n%s", clipFirecrawlRunes(md, maxChars))
	}
	return strings.TrimSpace(sb.String())
}

func FormatFirecrawlMapContent(res *FirecrawlMapResult, maxLinks int) string {
	if res == nil {
		return ""
	}
	if maxLinks <= 0 || maxLinks > len(res.Links) {
		maxLinks = len(res.Links)
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "links_count: %d\n", len(res.Links))
	for i, link := range res.Links[:maxLinks] {
		if strings.TrimSpace(link.URL) == "" {
			continue
		}
		fmt.Fprintf(&sb, "%d. %s", i+1, link.URL)
		if strings.TrimSpace(link.Title) != "" {
			fmt.Fprintf(&sb, " - %s", link.Title)
		}
		if strings.TrimSpace(link.Description) != "" {
			fmt.Fprintf(&sb, "\n   %s", link.Description)
		}
		sb.WriteString("\n")
	}
	return strings.TrimSpace(sb.String())
}

func FirecrawlScrapeResultURL(res *FirecrawlScrapeResult, fallback string) string {
	if u := firecrawlResultURL(res); u != "" {
		return u
	}
	return fallback
}

func firecrawlResultURL(res *FirecrawlScrapeResult) string {
	if res == nil {
		return ""
	}
	if strings.TrimSpace(res.Data.Metadata.SourceURL) != "" {
		return strings.TrimSpace(res.Data.Metadata.SourceURL)
	}
	return strings.TrimSpace(res.Data.Metadata.URL)
}

func FirecrawlMapURLs(res *FirecrawlMapResult) []string {
	if res == nil {
		return nil
	}
	urls := make([]string, 0, len(res.Links))
	for _, link := range res.Links {
		if u := strings.TrimSpace(link.URL); u != "" {
			urls = append(urls, u)
		}
	}
	return dedupURLs(urls)
}

func clipFirecrawlRunes(text string, maxChars int) string {
	text = strings.TrimSpace(text)
	if text == "" || maxChars <= 0 {
		return text
	}
	runes := []rune(text)
	if len(runes) <= maxChars {
		return text
	}
	return strings.TrimSpace(string(runes[:maxChars])) + "\n... [truncated]"
}
