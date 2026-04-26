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

func TestTavilySearch_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tvly-test" {
			t.Fatalf("Authorization = %q", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["query"] != "hello world" {
			t.Errorf("query = %v", body["query"])
		}
		if body["include_answer"] != true {
			t.Errorf("include_answer = %v", body["include_answer"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"answer": "Hello world is a classic.",
			"query": "hello world",
			"results": [
				{"title": "Wikipedia", "url": "https://en.wikipedia.org/wiki/Hello_world", "content": "...", "score": 0.97},
				{"title": "K&R", "url": "https://example.com/k-and-r", "content": "...", "score": 0.85}
			]
		}`))
	}))
	defer ts.Close()

	c := NewTavilyClient(ts.URL, "tvly-test")
	res, err := c.Search(context.Background(), "hello world")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if res.Answer != "Hello world is a classic." {
		t.Errorf("Answer = %q", res.Answer)
	}
	if len(res.Results) != 2 {
		t.Fatalf("len(Results) = %d, want 2", len(res.Results))
	}
	if res.Results[0].URL != "https://en.wikipedia.org/wiki/Hello_world" {
		t.Errorf("first URL = %q", res.Results[0].URL)
	}
}

func TestTavilySearch_HTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("invalid api key"))
	}))
	defer ts.Close()

	c := NewTavilyClient(ts.URL, "bad")
	_, err := c.Search(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "401") || !strings.Contains(err.Error(), "invalid api key") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestTavilySearch_EmptyResult(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"answer": "", "results": []}`))
	}))
	defer ts.Close()

	c := NewTavilyClient(ts.URL, "x")
	_, err := c.Search(context.Background(), "obscure query")
	if err == nil {
		t.Fatal("expected empty-result error, got nil")
	}
	if !strings.Contains(err.Error(), "empty result") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestTavilySearch_AnswerOnlyIsValid(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"answer": "42", "results": []}`))
	}))
	defer ts.Close()

	c := NewTavilyClient(ts.URL, "x")
	res, err := c.Search(context.Background(), "meaning of life")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Answer != "42" {
		t.Errorf("Answer = %q", res.Answer)
	}
}

func TestTavilySearch_EmptyQueryIsError(t *testing.T) {
	c := NewTavilyClient("https://api.tavily.com", "x")
	_, err := c.Search(context.Background(), "")
	if err == nil {
		t.Fatal("expected empty-query error, got nil")
	}
}

func TestTavilySearch_RetriesOn429ThenSucceeds(t *testing.T) {
	var calls int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"answer": "after retry", "results": []}`))
	}))
	defer ts.Close()

	c := NewTavilyClient(ts.URL, "tvly-test")
	c.RetryConfig = RetryConfig{MaxAttempts: 3, BaseDelay: 0, MaxDelay: 0, Jitter: false}

	res, err := c.Search(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if res.Answer != "after retry" {
		t.Errorf("Answer = %q, want \"after retry\"", res.Answer)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("calls = %d, want 2 (initial 429 + retry)", got)
	}
}

func TestTavilyExtract_RetriesOn429ThenSucceeds(t *testing.T) {
	var calls int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/extract" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"url":"https://example.com","raw_content":"after retry"}]}`))
	}))
	defer ts.Close()

	c := NewTavilyClient(ts.URL, "tvly-test")
	c.RetryConfig = RetryConfig{MaxAttempts: 3, BaseDelay: 0, MaxDelay: 0, Jitter: false}

	res, err := c.Extract(context.Background(), "https://example.com")
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}
	if res.Content != "after retry" {
		t.Errorf("Content = %q, want \"after retry\"", res.Content)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("calls = %d, want 2 (initial 429 + retry)", got)
	}
}

func TestTavilyMap_RetriesOn500ThenSucceeds(t *testing.T) {
	var calls int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/map" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"urls":["https://example.com/a","https://example.com/b"]}`))
	}))
	defer ts.Close()

	c := NewTavilyClient(ts.URL, "tvly-test")
	c.RetryConfig = RetryConfig{MaxAttempts: 3, BaseDelay: 0, MaxDelay: 0, Jitter: false}

	res, err := c.Map(context.Background(), "https://example.com", 1, 5, 10)
	if err != nil {
		t.Fatalf("Map failed: %v", err)
	}
	if len(res.URLs) != 2 {
		t.Fatalf("len(URLs) = %d, want 2", len(res.URLs))
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("calls = %d, want 2 (initial 500 + retry)", got)
	}
}
