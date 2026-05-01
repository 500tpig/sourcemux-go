package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestExaSearch_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("x-api-key"); got != "exa-test" {
			t.Fatalf("x-api-key = %q", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["query"] != "hello world" {
			t.Errorf("query = %v", body["query"])
		}
		if body["type"] != "auto" {
			t.Errorf("type = %v", body["type"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"requestId": "req-1",
			"searchType": "auto",
			"results": [
				{"title": "Wikipedia", "url": "https://en.wikipedia.org/wiki/Hello_world", "highlights": ["classic example"]},
				{"title": "Example", "url": "https://example.com/hello", "summary": "summary"}
			]
		}`))
	}))
	defer ts.Close()

	c := NewExaClient(ts.URL+"/", "exa-test")
	res, err := c.Search(context.Background(), "hello world")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if res.RequestID != "req-1" {
		t.Errorf("RequestID = %q", res.RequestID)
	}
	urls := ExaSearchSourceURLs(res)
	if len(urls) != 2 || urls[0] != "https://en.wikipedia.org/wiki/Hello_world" {
		t.Fatalf("urls = %#v", urls)
	}
	formatted := FormatExaSearchContent(res)
	if !strings.Contains(formatted, "classic example") || !strings.Contains(formatted, "Wikipedia") {
		t.Fatalf("formatted content missing expected text: %s", formatted)
	}
}

func TestExaContents_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/contents" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		urls, ok := body["urls"].([]any)
		if !ok || len(urls) != 1 || urls[0] != "https://example.com/a" {
			t.Fatalf("urls = %#v", body["urls"])
		}
		if body["text"] != true {
			t.Errorf("text = %v", body["text"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"requestId": "req-2",
			"results": [
				{"url": "https://example.com/a", "text": "# Hello\ncontent"}
			],
			"statuses": [{"id":"https://example.com/a","status":"success"}]
		}`))
	}))
	defer ts.Close()

	c := NewExaClient(ts.URL, "exa-test")
	res, err := c.Extract(context.Background(), "https://example.com/a")
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}
	if res.URL != "https://example.com/a" || res.Content != "# Hello\ncontent" {
		t.Fatalf("result = %+v", res)
	}
}

func TestExaSearch_HTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("invalid api key"))
	}))
	defer ts.Close()

	c := NewExaClient(ts.URL, "bad")
	_, err := c.Search(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "401") || !strings.Contains(err.Error(), "invalid api key") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestExaSearch_RetriesOn429ThenSucceeds(t *testing.T) {
	var calls int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"title":"ok","url":"https://example.com"}]}`))
	}))
	defer ts.Close()

	c := NewExaClient(ts.URL, "exa-test")
	c.RetryConfig = RetryConfig{MaxAttempts: 3, BaseDelay: 0, MaxDelay: 0, Jitter: false}

	res, err := c.Search(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(res.Results) != 1 {
		t.Fatalf("len(Results) = %d, want 1", len(res.Results))
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("calls = %d, want 2 (initial 429 + retry)", got)
	}
}
