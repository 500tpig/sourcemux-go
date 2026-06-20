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

const defaultReasoningRequestTimeout = 120 * time.Second

// ReasoningEndpoint is one OpenAI-compatible endpoint used for final synthesis.
// It is intentionally separate from GrokEndpoint so non-search models do not
// short-circuit the web_search provider route.
type ReasoningEndpoint struct {
	Name    string `json:"name"`
	BaseURL string `json:"baseURL"`
	APIKey  string `json:"apiKey"`
	Model   string `json:"model"`
}

// ReasoningClient wraps an OpenAI-compatible chat completions endpoint for
// evidence-grounded final answers.
type ReasoningClient struct {
	Name           string
	BaseURL        string
	APIKey         string
	Model          string
	HTTPClient     *http.Client
	RequestTimeout time.Duration
	RetryConfig    RetryConfig
}

// NewReasoningClient creates a generic OpenAI-compatible reasoning client.
func NewReasoningClient(baseURL, apiKey, model string) *ReasoningClient {
	return &ReasoningClient{
		Name:           "reasoning",
		BaseURL:        strings.TrimRight(baseURL, "/"),
		APIKey:         apiKey,
		Model:          model,
		HTTPClient:     &http.Client{},
		RequestTimeout: defaultReasoningRequestTimeout,
		RetryConfig:    DefaultRetryConfig(),
	}
}

// ReasoningRequest is the provider-agnostic input for one synthesis call.
type ReasoningRequest struct {
	SystemPrompt string
	UserPrompt   string
	Model        string
}

// ReasoningResult is the provider-agnostic output from one synthesis call.
type ReasoningResult struct {
	Content string `json:"content"`
}

type reasoningMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type reasoningRawResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// Complete sends one chat-completions request and returns the first choice.
func (c *ReasoningClient) Complete(ctx context.Context, req ReasoningRequest) (*ReasoningResult, error) {
	if c == nil {
		return nil, fmt.Errorf("reasoning client is nil")
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = strings.TrimSpace(c.Model)
	}
	if model == "" {
		return nil, fmt.Errorf("reasoning model is required")
	}
	userPrompt := strings.TrimSpace(req.UserPrompt)
	if userPrompt == "" {
		return nil, fmt.Errorf("reasoning user prompt is required")
	}

	messages := make([]reasoningMessage, 0, 2)
	if systemPrompt := strings.TrimSpace(req.SystemPrompt); systemPrompt != "" {
		messages = append(messages, reasoningMessage{Role: "system", Content: systemPrompt})
	}
	messages = append(messages, reasoningMessage{Role: "user", Content: userPrompt})

	body := map[string]any{
		"model":    model,
		"messages": messages,
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal reasoning request: %w", err)
	}

	ctx, cancel := reasoningRequestContext(ctx, c.RequestTimeout)
	defer cancel()

	factory := func() (*http.Request, error) {
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
			c.BaseURL+"/chat/completions", bytes.NewReader(jsonBody))
		if err != nil {
			return nil, fmt.Errorf("create reasoning request: %w", err)
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
		return httpReq, nil
	}

	resp, err := httpDoWithRetry(ctx, c.HTTPClient, factory, c.RetryConfig)
	if err != nil {
		return nil, fmt.Errorf("reasoning request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("reasoning API %d: %s",
			resp.StatusCode, formatReasoningBodyDiagnostic(resp.Header.Get("Content-Type"), c.redactSecret(data)))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read reasoning response: %w", err)
	}

	content, err := c.decodeReasoningContent(data, resp.Header.Get("Content-Type"))
	if err != nil {
		return nil, err
	}
	return &ReasoningResult{Content: content}, nil
}

func reasoningRequestContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return ctx, func() {}
	}
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, timeout)
}

func (c *ReasoningClient) decodeReasoningContent(data []byte, contentType string) (string, error) {
	body := bytes.TrimSpace(data)

	var responsesRaw responsesAPIResponse
	if err := json.Unmarshal(body, &responsesRaw); err == nil && len(responsesRaw.Output) > 0 {
		content, _ := extractResponsesAPIResult(&responsesRaw)
		content = strings.TrimSpace(content)
		if content == "" {
			return "", fmt.Errorf("reasoning response content is empty")
		}
		return content, nil
	}

	var raw reasoningRawResponse
	if err := json.Unmarshal(body, &raw); err == nil {
		if len(raw.Choices) > 0 {
			content := strings.TrimSpace(raw.Choices[0].Message.Content)
			if content == "" {
				return "", fmt.Errorf("reasoning response content is empty")
			}
			return content, nil
		}

		return "", fmt.Errorf("reasoning response contained no choices")
	} else if looksLikeReasoningEventStream(contentType, body) {
		streamRes, streamErr := decodeEventStreamSearchResult(bytes.NewReader(body))
		if streamErr != nil {
			return "", fmt.Errorf("decode reasoning response: %w; %s",
				streamErr, formatReasoningBodyDiagnostic(contentType, c.redactSecret(body)))
		}
		content := strings.TrimSpace(streamRes.Content)
		if content == "" {
			return "", fmt.Errorf("reasoning response content is empty")
		}
		return content, nil
	} else {
		return "", fmt.Errorf("decode reasoning response: %w; %s",
			err, formatReasoningBodyDiagnostic(contentType, c.redactSecret(body)))
	}
}

func looksLikeReasoningEventStream(contentType string, body []byte) bool {
	if strings.Contains(strings.ToLower(contentType), "text/event-stream") {
		return true
	}
	return bytes.HasPrefix(bytes.TrimLeft(body, " \t\r\n"), []byte("data:"))
}

func (c *ReasoningClient) redactSecret(data []byte) []byte {
	key := strings.TrimSpace(c.APIKey)
	if key == "" {
		return data
	}
	return bytes.ReplaceAll(data, []byte(key), []byte("<redacted>"))
}

func clipReasoningBody(data []byte) string {
	s := strings.TrimSpace(string(data))
	if len(s) <= 500 {
		return s
	}
	return s[:500] + "..."
}

func formatReasoningBodyDiagnostic(contentType string, data []byte) string {
	body := clipReasoningBody(data)
	contentType = strings.TrimSpace(contentType)
	if contentType == "" {
		return body
	}
	return fmt.Sprintf("content-type=%q body=%s", contentType, body)
}

// ReasoningPool tries configured reasoning endpoints in priority order.
type ReasoningPool struct {
	clients []*ReasoningClient
}

// NewReasoningPool builds a pool from configured reasoning endpoints.
func NewReasoningPool(endpoints []ReasoningEndpoint) *ReasoningPool {
	clients := make([]*ReasoningClient, 0, len(endpoints))
	for _, ep := range endpoints {
		c := NewReasoningClient(ep.BaseURL, ep.APIKey, ep.Model)
		c.Name = ep.Name
		clients = append(clients, c)
	}
	return &ReasoningPool{clients: clients}
}

// Len reports the number of configured reasoning endpoints.
func (p *ReasoningPool) Len() int {
	if p == nil {
		return 0
	}
	return len(p.clients)
}

// Clients returns a copy of configured reasoning clients for diagnostics.
func (p *ReasoningPool) Clients() []*ReasoningClient {
	if p == nil {
		return nil
	}
	out := make([]*ReasoningClient, len(p.clients))
	copy(out, p.clients)
	return out
}

// PoolReasoningResult bundles a synthesis result with the endpoint that made it.
type PoolReasoningResult struct {
	*ReasoningResult
	EndpointName  string `json:"endpoint_name"`
	EndpointModel string `json:"model"`
}

// Complete tries endpoints in order, or one named endpoint when endpointName is set.
func (p *ReasoningPool) Complete(ctx context.Context, req ReasoningRequest, endpointName string) (*PoolReasoningResult, error) {
	if p == nil || len(p.clients) == 0 {
		return nil, fmt.Errorf("reasoning pool is empty: no reasoningEndpoints configured")
	}

	clients := p.clients
	endpointName = strings.TrimSpace(endpointName)
	if endpointName != "" {
		clients = nil
		for _, c := range p.clients {
			if c.Name == endpointName {
				clients = append(clients, c)
				break
			}
		}
		if len(clients) == 0 {
			return nil, fmt.Errorf("reasoning endpoint %q not found in reasoningEndpoints (available: %s)", endpointName, strings.Join(p.endpointNames(), ", "))
		}
	}

	errs := make([]string, 0, len(clients))
	for _, c := range clients {
		res, err := c.Complete(ctx, req)
		if err == nil && res != nil && res.Content != "" {
			model := strings.TrimSpace(req.Model)
			if model == "" {
				model = c.Model
			}
			return &PoolReasoningResult{
				ReasoningResult: res,
				EndpointName:    c.Name,
				EndpointModel:   model,
			}, nil
		}
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s (%s): %v", c.Name, c.Model, err))
		} else {
			errs = append(errs, fmt.Sprintf("%s (%s): empty result", c.Name, c.Model))
		}
	}
	return nil, fmt.Errorf("all %d reasoning endpoints failed: %s", len(clients), strings.Join(errs, "; "))
}

func (p *ReasoningPool) endpointNames() []string {
	names := make([]string, 0, len(p.clients))
	for _, c := range p.clients {
		names = append(names, c.Name)
	}
	if len(names) == 0 {
		return []string{"(none)"}
	}
	return names
}
