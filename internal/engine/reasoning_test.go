package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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
