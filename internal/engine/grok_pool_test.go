package engine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
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
