package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultTinyFishSearchURL = "https://api.search.tinyfish.ai"
	DefaultTinyFishFetchURL  = "https://api.fetch.tinyfish.ai"
	DefaultTinyFishAgentURL  = "https://agent.tinyfish.ai/v1/automation/run"
)

// TinyFishClient wraps TinyFish's REST Search, Fetch, and synchronous Agent APIs.
// It is used only by the local benchmark CLI, not by production MCP routing.
type TinyFishClient struct {
	APIKey    string
	SearchURL string
	FetchURL  string
	AgentURL  string

	HTTPClient *http.Client
}

func NewTinyFishClient(apiKey string) *TinyFishClient {
	return &TinyFishClient{
		APIKey:    apiKey,
		SearchURL: DefaultTinyFishSearchURL,
		FetchURL:  DefaultTinyFishFetchURL,
		AgentURL:  DefaultTinyFishAgentURL,
		HTTPClient: &http.Client{
			Timeout: 150 * time.Second,
		},
	}
}

type TinyFishSearchRequest struct {
	Query    string
	Location string
	Language string
	Page     *int
}

type TinyFishSearchResponse struct {
	HTTPStatus   int                    `json:"-"`
	Query        string                 `json:"query"`
	Results      []TinyFishSearchResult `json:"results"`
	TotalResults int                    `json:"total_results"`
	Page         *int                   `json:"page,omitempty"`
}

type TinyFishSearchResult struct {
	Position int    `json:"position"`
	SiteName string `json:"site_name"`
	Title    string `json:"title"`
	Snippet  string `json:"snippet"`
	URL      string `json:"url"`
}

func (c *TinyFishClient) Search(ctx context.Context, req TinyFishSearchRequest) (*TinyFishSearchResponse, error) {
	if strings.TrimSpace(req.Query) == "" {
		return nil, fmt.Errorf("tinyfish search: empty query")
	}
	u, err := url.Parse(c.searchURL())
	if err != nil {
		return nil, fmt.Errorf("tinyfish search: invalid search URL: %w", err)
	}
	q := u.Query()
	q.Set("query", req.Query)
	if req.Location != "" {
		q.Set("location", req.Location)
	}
	if req.Language != "" {
		q.Set("language", req.Language)
	}
	if req.Page != nil {
		q.Set("page", strconv.Itoa(*req.Page))
	}
	u.RawQuery = q.Encode()

	httpReq, err := c.newRequest(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	var out TinyFishSearchResponse
	status, err := c.doJSON(httpReq, &out)
	out.HTTPStatus = status
	return &out, err
}

type TinyFishFetchRequest struct {
	URLs       []string `json:"urls"`
	Format     string   `json:"format,omitempty"`
	Links      bool     `json:"links"`
	ImageLinks bool     `json:"image_links"`
}

type TinyFishFetchResponse struct {
	HTTPStatus int                    `json:"-"`
	Results    []TinyFishFetchResult  `json:"results"`
	Errors     []TinyFishFetchFailure `json:"errors"`
}

type TinyFishFetchResult struct {
	URL           string          `json:"url"`
	FinalURL      string          `json:"final_url,omitempty"`
	Title         string          `json:"title,omitempty"`
	Description   string          `json:"description,omitempty"`
	Language      string          `json:"language,omitempty"`
	Author        string          `json:"author,omitempty"`
	PublishedDate string          `json:"published_date,omitempty"`
	Text          json.RawMessage `json:"text"`
	Links         []string        `json:"links,omitempty"`
	ImageLinks    []string        `json:"image_links,omitempty"`
	LatencyMS     *int64          `json:"latency_ms,omitempty"`
	Format        string          `json:"format,omitempty"`
}

type TinyFishFetchFailure struct {
	URL   string `json:"url"`
	Code  string `json:"code,omitempty"`
	Error string `json:"error"`
}

func (c *TinyFishClient) Fetch(ctx context.Context, req TinyFishFetchRequest) (*TinyFishFetchResponse, error) {
	if len(req.URLs) == 0 {
		return nil, fmt.Errorf("tinyfish fetch: no URLs")
	}
	if len(req.URLs) > 10 {
		return nil, fmt.Errorf("tinyfish fetch: at most 10 URLs per request")
	}
	for i, u := range req.URLs {
		if strings.TrimSpace(u) == "" {
			return nil, fmt.Errorf("tinyfish fetch: URL #%d is empty", i)
		}
	}
	if req.Format == "" {
		req.Format = "markdown"
	}

	httpReq, err := c.newJSONRequest(ctx, c.fetchURL(), req)
	if err != nil {
		return nil, err
	}
	var out TinyFishFetchResponse
	status, err := c.doJSON(httpReq, &out)
	out.HTTPStatus = status
	return &out, err
}

type TinyFishAgentRequest struct {
	URL            string         `json:"url"`
	Goal           string         `json:"goal"`
	BrowserProfile string         `json:"browser_profile,omitempty"`
	AgentConfig    map[string]any `json:"agent_config,omitempty"`
	CaptureConfig  map[string]any `json:"capture_config,omitempty"`
	OutputSchema   map[string]any `json:"output_schema,omitempty"`
}

type TinyFishAgentResponse struct {
	HTTPStatus int             `json:"-"`
	RunID      string          `json:"run_id"`
	Status     string          `json:"status"`
	StartedAt  string          `json:"started_at"`
	FinishedAt string          `json:"finished_at"`
	NumOfSteps *int            `json:"num_of_steps"`
	Steps      json.RawMessage `json:"steps,omitempty"`
	Result     json.RawMessage `json:"result"`
	Error      json.RawMessage `json:"error"`
}

func (c *TinyFishClient) RunAgent(ctx context.Context, req TinyFishAgentRequest) (*TinyFishAgentResponse, error) {
	if strings.TrimSpace(req.URL) == "" {
		return nil, fmt.Errorf("tinyfish agent: empty URL")
	}
	if strings.TrimSpace(req.Goal) == "" {
		return nil, fmt.Errorf("tinyfish agent: empty goal")
	}

	httpReq, err := c.newJSONRequest(ctx, c.agentURL(), req)
	if err != nil {
		return nil, err
	}
	var out TinyFishAgentResponse
	status, err := c.doJSON(httpReq, &out)
	out.HTTPStatus = status
	return &out, err
}

type TinyFishHTTPError struct {
	StatusCode int
	Body       string
}

func (e *TinyFishHTTPError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("tinyfish API HTTP %d", e.StatusCode)
	}
	return fmt.Sprintf("tinyfish API HTTP %d: %s", e.StatusCode, e.Body)
}

func (c *TinyFishClient) newJSONRequest(ctx context.Context, endpoint string, v any) (*http.Request, error) {
	body, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return c.newRequest(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
}

func (c *TinyFishClient) newRequest(ctx context.Context, method, endpoint string, body io.Reader) (*http.Request, error) {
	if strings.TrimSpace(c.APIKey) == "" {
		return nil, fmt.Errorf("tinyfish: missing API key")
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-API-Key", c.APIKey)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

func (c *TinyFishClient) doJSON(req *http.Request, out any) (int, error) {
	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, &TinyFishHTTPError{StatusCode: resp.StatusCode, Body: clipTinyFishBody(c.redactSecret(data))}
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return resp.StatusCode, nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return resp.StatusCode, fmt.Errorf("decode tinyfish response: %w", err)
	}
	return resp.StatusCode, nil
}

func (c *TinyFishClient) searchURL() string {
	if c.SearchURL != "" {
		return c.SearchURL
	}
	return DefaultTinyFishSearchURL
}

func (c *TinyFishClient) fetchURL() string {
	if c.FetchURL != "" {
		return c.FetchURL
	}
	return DefaultTinyFishFetchURL
}

func (c *TinyFishClient) agentURL() string {
	if c.AgentURL != "" {
		return c.AgentURL
	}
	return DefaultTinyFishAgentURL
}

func clipTinyFishBody(data []byte) string {
	s := strings.TrimSpace(string(data))
	if len(s) <= 500 {
		return s
	}
	return s[:500] + "..."
}

func (c *TinyFishClient) redactSecret(data []byte) []byte {
	key := strings.TrimSpace(c.APIKey)
	if key == "" {
		return data
	}
	return bytes.ReplaceAll(data, []byte(key), []byte("<redacted>"))
}

// TinyFishTextLength returns a stable benchmark length for Fetch text, which can
// be either a string or a JSON document tree depending on requested format.
func TinyFishTextLength(raw json.RawMessage) int {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return 0
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return len(s)
	}
	return len(raw)
}
