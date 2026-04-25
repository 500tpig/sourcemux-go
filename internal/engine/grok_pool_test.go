package engine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
)

func newPoolMock(t *testing.T, status int, body string) *GrokClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	c := NewGrokClient(srv.URL, "k", "m")
	c.SendSearchFlag = false
	// Default to a single attempt so existing pool fallback tests stay fast
	// and don't trigger the new in-client retry loop. Tests that specifically
	// exercise retry should opt in by setting RetryConfig themselves.
	c.RetryConfig = RetryConfig{MaxAttempts: 1, Jitter: false}
	return c
}

func TestPool_FallbackFromFirstToSecond(t *testing.T) {
	failing := newPoolMock(t, http.StatusInternalServerError, `{"error":"oops"}`)
	failing.Name = "primary"

	good := newPoolMock(t, http.StatusOK, `{
		"choices":[{"message":{"content":"ok"}}],
		"citations":["https://example.com/a"]
	}`)
	good.Name = "secondary"

	pool := &GrokPool{clients: []*GrokClient{failing, good}}

	res, err := pool.Search(context.Background(), "q")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.EndpointName != "secondary" {
		t.Fatalf("EndpointName = %q, want %q", res.EndpointName, "secondary")
	}
	if res.Content != "ok" {
		t.Fatalf("Content = %q", res.Content)
	}
	if !reflect.DeepEqual(res.SourceURLs, []string{"https://example.com/a"}) {
		t.Fatalf("SourceURLs = %v", res.SourceURLs)
	}
}

func TestPool_AllFailReturnsAggregatedError(t *testing.T) {
	a := newPoolMock(t, http.StatusUnauthorized, `{"error":"bad key"}`)
	a.Name = "a"
	b := newPoolMock(t, http.StatusForbidden, `{"error":"forbidden"}`)
	b.Name = "b"
	pool := &GrokPool{clients: []*GrokClient{a, b}}

	_, err := pool.Search(context.Background(), "q")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "all 2 grok endpoints failed") {
		t.Fatalf("error message: %v", err)
	}
	if !strings.Contains(err.Error(), "a: ") || !strings.Contains(err.Error(), "b: ") {
		t.Fatalf("error must mention each endpoint: %v", err)
	}
}

func TestPool_EmptyContentTreatedAsFailure(t *testing.T) {
	empty := newPoolMock(t, http.StatusOK, `{"choices":[{"message":{"content":""}}]}`)
	empty.Name = "empty"
	good := newPoolMock(t, http.StatusOK, `{"choices":[{"message":{"content":"hello"}}]}`)
	good.Name = "good"
	pool := &GrokPool{clients: []*GrokClient{empty, good}}

	res, err := pool.Search(context.Background(), "q")
	if err != nil {
		t.Fatal(err)
	}
	if res.EndpointName != "good" {
		t.Fatalf("EndpointName = %q", res.EndpointName)
	}
}

func TestPool_EmptyPool(t *testing.T) {
	pool := &GrokPool{}
	_, err := pool.Search(context.Background(), "q")
	if err == nil {
		t.Fatal("expected error for empty pool")
	}
}

func TestNewGrokPool_PreservesNameAndFlag(t *testing.T) {
	eps := []GrokEndpoint{
		{Name: "wykon", BaseURL: "http://x", APIKey: "k", Model: "m", SendSearchFlag: false},
		{Name: "yyds", BaseURL: "http://y", APIKey: "k2", Model: "m2", SendSearchFlag: true},
	}
	pool := NewGrokPool(eps)
	if pool.Len() != 2 {
		t.Fatalf("Len = %d", pool.Len())
	}
	cs := pool.Clients()
	if cs[0].Name != "wykon" || cs[0].SendSearchFlag != false {
		t.Fatalf("client 0 = %+v", cs[0])
	}
	if cs[1].Name != "yyds" || cs[1].SendSearchFlag != true {
		t.Fatalf("client 1 = %+v", cs[1])
	}
}

// TestPool_PerEndpointRetriesBeforeFallback verifies the layered retry policy:
// when a primary endpoint serves a retryable status (here 503), the per-client
// retry loop attempts MaxAttempts=3 times against it before the pool moves on
// to the next endpoint.
func TestPool_PerEndpointRetriesBeforeFallback(t *testing.T) {
	var primaryCalls int32
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&primaryCalls, 1)
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(primary.Close)

	var secondaryCalls int32
	secondary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&secondaryCalls, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}],"citations":["https://example.com/x"]}`))
	}))
	t.Cleanup(secondary.Close)

	primaryClient := NewGrokClient(primary.URL, "k", "m")
	primaryClient.Name = "primary"
	primaryClient.SendSearchFlag = false
	primaryClient.RetryConfig = RetryConfig{MaxAttempts: 3, BaseDelay: 0, MaxDelay: 0, Jitter: false}

	secondaryClient := NewGrokClient(secondary.URL, "k", "m")
	secondaryClient.Name = "secondary"
	secondaryClient.SendSearchFlag = false
	secondaryClient.RetryConfig = RetryConfig{MaxAttempts: 1, Jitter: false}

	pool := &GrokPool{clients: []*GrokClient{primaryClient, secondaryClient}}
	res, err := pool.Search(context.Background(), "q")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.EndpointName != "secondary" {
		t.Fatalf("EndpointName = %q, want secondary", res.EndpointName)
	}
	if got := atomic.LoadInt32(&primaryCalls); got != 3 {
		t.Fatalf("expected primary to be retried 3 times, got %d", got)
	}
	if got := atomic.LoadInt32(&secondaryCalls); got != 1 {
		t.Fatalf("expected secondary to be hit once, got %d", got)
	}
}
