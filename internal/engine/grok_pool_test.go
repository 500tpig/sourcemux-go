package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
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

func TestPool_UnavailableContentTreatedAsFailure(t *testing.T) {
	overloaded := newPoolMock(t, http.StatusOK, `{
		"choices":[{"message":{"content":"This model is overloaded right now. Please try again shortly or pick a different model."}}]
	}`)
	overloaded.Name = "overloaded"
	good := newPoolMock(t, http.StatusOK, `{"choices":[{"message":{"content":"hello"}}]}`)
	good.Name = "good"
	pool := &GrokPool{clients: []*GrokClient{overloaded, good}}

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
		{Name: "yyds", BaseURL: "http://y", APIKey: "k2", Model: "m2", Profile: "heavy", SendSearchFlag: true, APIType: "responses", ResponseTools: []string{ResponseToolWebSearch, ResponseToolXSearch}},
	}
	pool := NewGrokPool(eps)
	if pool.Len() != 2 {
		t.Fatalf("Len = %d", pool.Len())
	}
	cs := pool.Clients()
	if cs[0].Name != "wykon" || cs[0].SendSearchFlag != false {
		t.Fatalf("client 0 = %+v", cs[0])
	}
	if cs[1].Name != "yyds" || cs[1].SendSearchFlag != true || cs[1].APIType != "responses" || cs[1].EffectiveProfile() != "heavy" {
		t.Fatalf("client 1 = %+v", cs[1])
	}
	if !reflect.DeepEqual(cs[1].ResponseTools, []string{ResponseToolWebSearch, ResponseToolXSearch}) {
		t.Fatalf("client 1 response tools = %v", cs[1].ResponseTools)
	}
}

func TestNewGrokPool_SkipsDisabledEndpoints(t *testing.T) {
	disabled := false
	pool := NewGrokPool([]GrokEndpoint{
		{Name: "disabled", BaseURL: "http://disabled", APIKey: "k", Model: "m", Enabled: &disabled},
		{Name: "enabled", BaseURL: "http://enabled", APIKey: "k", Model: "m"},
	})

	if pool.Len() != 1 {
		t.Fatalf("Len = %d, want 1", pool.Len())
	}
	if got := pool.Clients()[0].Name; got != "enabled" {
		t.Fatalf("client name = %q, want enabled", got)
	}
}

func TestGrokPool_ProfileIntrospection(t *testing.T) {
	disabled := false
	pool := NewGrokPool([]GrokEndpoint{
		{Name: "default-a", BaseURL: "http://default-a", APIKey: "k", Model: "m"},
		{Name: "default-disabled", BaseURL: "http://default-disabled", APIKey: "k", Model: "m", Enabled: &disabled},
		{Name: "heavy-a", BaseURL: "http://heavy-a", APIKey: "k", Model: "m", Profile: "heavy"},
		{Name: "heavy-b", BaseURL: "http://heavy-b", APIKey: "k", Model: "m", Profile: " HEAVY "},
	})

	if got := pool.ProfileLen(""); got != 1 {
		t.Fatalf("default ProfileLen = %d, want 1", got)
	}
	if got := pool.ProfileLen("heavy"); got != 2 {
		t.Fatalf("heavy ProfileLen = %d, want 2", got)
	}
	if !pool.HasProfile("HEAVY") {
		t.Fatalf("HasProfile(HEAVY) = false, want true")
	}
	if pool.HasProfile("missing") {
		t.Fatalf("HasProfile(missing) = true, want false")
	}
}

func TestGrokPool_ProfileSelectsOnlyMatchingEndpoints(t *testing.T) {
	var defaultCalls, heavyCalls int32
	defaultSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&defaultCalls, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"default ok"}}]}`))
	}))
	t.Cleanup(defaultSrv.Close)
	heavySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&heavyCalls, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"heavy ok"}}]}`))
	}))
	t.Cleanup(heavySrv.Close)

	pool := NewGrokPool([]GrokEndpoint{
		{Name: "default", BaseURL: defaultSrv.URL, APIKey: "k", Model: "m"},
		{Name: "heavy", BaseURL: heavySrv.URL, APIKey: "k", Model: "m", Profile: "heavy"},
	})

	res, err := pool.Search(context.Background(), "q")
	if err != nil {
		t.Fatalf("default Search failed: %v", err)
	}
	if res.EndpointName != "default" {
		t.Fatalf("default EndpointName = %q", res.EndpointName)
	}
	res, err = pool.SearchWithModelAndProfile(context.Background(), "q", "", "heavy")
	if err != nil {
		t.Fatalf("heavy Search failed: %v", err)
	}
	if res.EndpointName != "heavy" {
		t.Fatalf("heavy EndpointName = %q", res.EndpointName)
	}
	if atomic.LoadInt32(&defaultCalls) != 1 || atomic.LoadInt32(&heavyCalls) != 1 {
		t.Fatalf("calls default=%d heavy=%d", defaultCalls, heavyCalls)
	}
}

func TestGrokPool_SearchWithModelOverridesPerRequest(t *testing.T) {
	var gotModel string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		gotModel = body.Model
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"override ok"}}]}`))
	}))
	defer ts.Close()

	pool := NewGrokPool([]GrokEndpoint{{
		Name:    "primary",
		BaseURL: ts.URL,
		APIKey:  "k",
		Model:   "configured-model",
	}})

	res, err := pool.SearchWithModel(context.Background(), "q", "override-model")
	if err != nil {
		t.Fatalf("SearchWithModel failed: %v", err)
	}
	if res.EndpointModel != "override-model" {
		t.Fatalf("EndpointModel = %q, want override-model", res.EndpointModel)
	}
	if gotModel != "override-model" {
		t.Fatalf("request model = %q, want override-model", gotModel)
	}
	if pool.Clients()[0].Model != "configured-model" {
		t.Fatalf("pool client was mutated: %q", pool.Clients()[0].Model)
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

// TestPool_OverallTimeoutStopsEarly verifies that when the GrokPool's
// OverallTimeout fires while the primary endpoint is still in flight, the pool
// surfaces a deadline error and never falls through to the next endpoint.
func TestPool_OverallTimeoutStopsEarly(t *testing.T) {
	var primaryHits, secondaryHits int32
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&primaryHits, 1)
		// Keep the endpoint slow enough for the pool deadline to fire, but do
		// not block forever: httptest.Server.Close waits for active handlers.
		time.Sleep(200 * time.Millisecond)
	}))
	t.Cleanup(primary.Close)

	secondary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&secondaryHits, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	t.Cleanup(secondary.Close)

	primaryClient := NewGrokClient(primary.URL, "k", "m")
	primaryClient.Name = "primary"
	primaryClient.SendSearchFlag = false
	primaryClient.RetryConfig = RetryConfig{MaxAttempts: 1, Jitter: false}

	secondaryClient := NewGrokClient(secondary.URL, "k", "m")
	secondaryClient.Name = "secondary"
	secondaryClient.SendSearchFlag = false
	secondaryClient.RetryConfig = RetryConfig{MaxAttempts: 1, Jitter: false}

	pool := &GrokPool{
		clients:        []*GrokClient{primaryClient, secondaryClient},
		OverallTimeout: 50 * time.Millisecond,
	}

	start := time.Now()
	_, err := pool.Search(context.Background(), "q")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error after deadline, got nil")
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("Search took %v, expected to abort well before 500ms", elapsed)
	}
	if got := atomic.LoadInt32(&primaryHits); got != 1 {
		t.Errorf("primary hits = %d, want 1", got)
	}
	if got := atomic.LoadInt32(&secondaryHits); got != 0 {
		t.Errorf("secondary should not be called after pool deadline, got hits=%d", got)
	}
}

// TestPool_OverallTimeoutZeroDisabled verifies that the legacy behavior
// (no deadline) is preserved when OverallTimeout is left at its zero value.
func TestPool_OverallTimeoutZeroDisabled(t *testing.T) {
	good := newPoolMock(t, http.StatusOK, `{"choices":[{"message":{"content":"ok"}}]}`)
	good.Name = "good"
	pool := &GrokPool{clients: []*GrokClient{good}} // OverallTimeout: 0

	res, err := pool.Search(context.Background(), "q")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Content != "ok" {
		t.Errorf("Content = %q", res.Content)
	}
}
