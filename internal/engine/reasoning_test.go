package engine

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestReasoningClientComplete(t *testing.T) {
	var gotPath, gotAuth string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		if !strings.Contains(r.Header.Get("Content-Type"), "application/json") {
			t.Errorf("content type = %q, want application/json", r.Header.Get("Content-Type"))
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"final answer"}}]}`))
	}))
	defer srv.Close()

	c := NewReasoningClient(srv.URL+"/v1", "sk-test", "deepseek-v4-flash")
	c.RetryConfig = noRetryConfig()
	res, err := c.Complete(context.Background(), ReasoningRequest{
		SystemPrompt: "system",
		UserPrompt:   "user",
	})
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}
	if res.Content != "final answer" {
		t.Fatalf("content = %q", res.Content)
	}
	if gotPath != "/v1/chat/completions" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotAuth != "Bearer sk-test" {
		t.Fatalf("auth = %q", gotAuth)
	}
	if gotBody["model"] != "deepseek-v4-flash" {
		t.Fatalf("model = %#v", gotBody["model"])
	}
	messages, ok := gotBody["messages"].([]any)
	if !ok || len(messages) != 2 {
		t.Fatalf("messages = %#v", gotBody["messages"])
	}
}

func TestReasoningClientComplete_EventStreamBodyWithoutContentType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		bw := bufio.NewWriter(w)
		_, _ = bw.WriteString(`data: {"choices":[{"delta":{"content":"final "}}]}` + "\n\n")
		_, _ = bw.WriteString(`data: {"choices":[{"delta":{"content":"answer"}}]}` + "\n\n")
		_, _ = bw.WriteString("data: [DONE]\n\n")
		_ = bw.Flush()
	}))
	defer srv.Close()

	c := NewReasoningClient(srv.URL+"/v1", "sk-test", "deepseek-v4-flash")
	c.RetryConfig = noRetryConfig()
	res, err := c.Complete(context.Background(), ReasoningRequest{UserPrompt: "user"})
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}
	if res.Content != "final answer" {
		t.Fatalf("content = %q", res.Content)
	}
}

func TestReasoningClientComplete_ResponsesEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"final answer"}]}]}`))
	}))
	defer srv.Close()

	c := NewReasoningClient(srv.URL+"/v1", "sk-test", "deepseek-v4-flash")
	c.RetryConfig = noRetryConfig()
	res, err := c.Complete(context.Background(), ReasoningRequest{UserPrompt: "user"})
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}
	if res.Content != "final answer" {
		t.Fatalf("content = %q", res.Content)
	}
}

func TestReasoningClientComplete_SlowEventStreamUsesCallerDeadline(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("response writer does not support flushing")
		}
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"final "}}]}` + "\n\n"))
		flusher.Flush()
		time.Sleep(150 * time.Millisecond)
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"answer"}}]}` + "\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		flusher.Flush()
	}))
	defer srv.Close()

	c := NewReasoningClient(srv.URL+"/v1", "sk-test", "deepseek-v4-flash")
	c.RetryConfig = noRetryConfig()
	c.RequestTimeout = 50 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	res, err := c.Complete(ctx, ReasoningRequest{UserPrompt: "user"})
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}
	if res.Content != "final answer" {
		t.Fatalf("content = %q", res.Content)
	}
}

func TestReasoningClientComplete_SlowEventStreamWithoutCallerDeadlineUsesFallbackTimeout(t *testing.T) {
	var started atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started.Store(true)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("response writer does not support flushing")
		}
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"final "}}]}` + "\n\n"))
		flusher.Flush()
		time.Sleep(150 * time.Millisecond)
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"answer"}}]}` + "\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		flusher.Flush()
	}))
	defer srv.Close()

	c := NewReasoningClient(srv.URL+"/v1", "sk-test", "deepseek-v4-flash")
	c.RetryConfig = noRetryConfig()
	c.RequestTimeout = 50 * time.Millisecond

	start := time.Now()
	_, err := c.Complete(context.Background(), ReasoningRequest{UserPrompt: "user"})
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !started.Load() {
		t.Fatal("expected server to receive request")
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("Complete took %v, expected fallback timeout well before 500ms", elapsed)
	}
	if !errors.Is(err, context.DeadlineExceeded) && !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("expected deadline error, got %v", err)
	}
}

func TestReasoningClientRedactsSecretInErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad key sk-secret-value", http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := NewReasoningClient(srv.URL+"/v1", "sk-secret-value", "deepseek-v4-flash")
	c.RetryConfig = noRetryConfig()
	_, err := c.Complete(context.Background(), ReasoningRequest{UserPrompt: "hello"})
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "sk-secret-value") || !strings.Contains(err.Error(), "<redacted>") {
		t.Fatalf("secret was not redacted: %v", err)
	}
}

func TestReasoningClientDecodeErrorIncludesBodySnippet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("denied sk-secret-value"))
	}))
	defer srv.Close()

	c := NewReasoningClient(srv.URL+"/v1", "sk-secret-value", "deepseek-v4-flash")
	c.RetryConfig = noRetryConfig()
	_, err := c.Complete(context.Background(), ReasoningRequest{UserPrompt: "hello"})
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	for _, want := range []string{"decode reasoning response", `content-type="text/plain"`, "denied", "<redacted>"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("missing %q in error: %v", want, err)
		}
	}
	if strings.Contains(msg, "sk-secret-value") {
		t.Fatalf("secret was not redacted: %v", err)
	}
}

func TestReasoningPoolNamedEndpoint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"from pro"}}]}`))
	}))
	defer srv.Close()

	pool := NewReasoningPool([]ReasoningEndpoint{
		{Name: "flash", BaseURL: "http://unused/v1", APIKey: "sk-a", Model: "deepseek-v4-flash"},
		{Name: "pro", BaseURL: srv.URL + "/v1", APIKey: "sk-b", Model: "deepseek-v4-pro"},
	})
	for _, c := range pool.Clients() {
		c.RetryConfig = noRetryConfig()
	}

	res, err := pool.Complete(context.Background(), ReasoningRequest{UserPrompt: "hello"}, "pro")
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}
	if res.EndpointName != "pro" || res.EndpointModel != "deepseek-v4-pro" || res.Content != "from pro" {
		t.Fatalf("result = %+v", res)
	}
}

func TestReasoningPoolFallsBackAfterInvalidBody(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("denied"))
	}))
	defer bad.Close()

	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"from fallback"}}]}`))
	}))
	defer good.Close()

	pool := NewReasoningPool([]ReasoningEndpoint{
		{Name: "bad", BaseURL: bad.URL + "/v1", APIKey: "sk-a", Model: "deepseek-v4-flash"},
		{Name: "good", BaseURL: good.URL + "/v1", APIKey: "sk-b", Model: "deepseek-v4-pro"},
	})
	for _, c := range pool.Clients() {
		c.RetryConfig = noRetryConfig()
	}

	res, err := pool.Complete(context.Background(), ReasoningRequest{UserPrompt: "hello"}, "")
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}
	if res.EndpointName != "good" || res.EndpointModel != "deepseek-v4-pro" || res.Content != "from fallback" {
		t.Fatalf("result = %+v", res)
	}
}

func TestReasoningPoolNamedEndpointNotFoundListsAvailable(t *testing.T) {
	pool := NewReasoningPool([]ReasoningEndpoint{
		{Name: "flash", BaseURL: "http://unused/v1", APIKey: "sk-a", Model: "deepseek-v4-flash"},
		{Name: "pro", BaseURL: "http://unused/v1", APIKey: "sk-b", Model: "deepseek-v4-pro"},
	})

	_, err := pool.Complete(context.Background(), ReasoningRequest{UserPrompt: "hello"}, "missing")
	if err == nil {
		t.Fatal("expected error")
	}
	for _, want := range []string{`reasoning endpoint "missing" not found`, "flash", "pro"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("missing %q in error: %v", want, err)
		}
	}
}

func noRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts: 1,
		BaseDelay:   0,
		MaxDelay:    0,
		Jitter:      false,
		now:         func() time.Time { return time.Unix(0, 0) },
		sleep:       func(ctx context.Context, d time.Duration) error { return nil },
	}
}
