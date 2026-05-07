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
		_, _ = w.Write([]byte(`{"results":["https://example.com/a","https://example.com/b"]}`))
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

func TestTavilyMap_LegacyURLsFieldStillWorks(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/map" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"urls":["https://example.com/legacy"]}`))
	}))
	defer ts.Close()

	c := NewTavilyClient(ts.URL, "tvly-test")
	res, err := c.Map(context.Background(), "https://example.com", 1, 5, 10)
	if err != nil {
		t.Fatalf("Map failed: %v", err)
	}
	if len(res.URLs) != 1 || res.URLs[0] != "https://example.com/legacy" {
		t.Fatalf("URLs = %#v, want legacy urls field to be accepted", res.URLs)
	}
}

func TestTavilyCrawl_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/crawl" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tvly-test" {
			t.Fatalf("Authorization = %q", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["url"] != "https://example.com/docs" {
			t.Errorf("url = %v", body["url"])
		}
		if body["instructions"] != "Find API pages" {
			t.Errorf("instructions = %v", body["instructions"])
		}
		if body["max_depth"] != float64(2) || body["max_breadth"] != float64(5) || body["limit"] != float64(3) {
			t.Errorf("crawl limits = %#v", body)
		}
		if body["extract_depth"] != "advanced" || body["format"] != "markdown" {
			t.Errorf("extract options = %#v", body)
		}
		if body["include_images"] != true {
			t.Errorf("include_images = %v", body["include_images"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"base_url": "example.com",
			"results": [
				{"url": "https://example.com/docs/a", "raw_content": "# A", "images": ["https://example.com/a.png"]},
				{"url": "https://example.com/docs/b", "raw_content": "# B"}
			],
			"response_time": 1.23
		}`))
	}))
	defer ts.Close()

	c := NewTavilyClient(ts.URL, "tvly-test")
	res, err := c.Crawl(context.Background(), TavilyCrawlRequest{
		URL:           "https://example.com/docs",
		Instructions:  "Find API pages",
		MaxDepth:      2,
		MaxBreadth:    5,
		Limit:         3,
		ExtractDepth:  "advanced",
		Format:        "markdown",
		IncludeImages: true,
	})
	if err != nil {
		t.Fatalf("Crawl failed: %v", err)
	}
	if res.BaseURL != "example.com" {
		t.Errorf("BaseURL = %q", res.BaseURL)
	}
	if len(res.Results) != 2 {
		t.Fatalf("len(Results) = %d, want 2", len(res.Results))
	}
	if res.Results[0].RawContent != "# A" || len(res.Results[0].Images) != 1 {
		t.Errorf("first result = %+v", res.Results[0])
	}
	if res.ResponseTime != 1.23 {
		t.Errorf("ResponseTime = %v", res.ResponseTime)
	}
}

func TestTavilyCrawl_EmptyURLError(t *testing.T) {
	c := NewTavilyClient("https://api.tavily.com", "x")
	_, err := c.Crawl(context.Background(), TavilyCrawlRequest{})
	if err == nil {
		t.Fatal("expected empty-url error, got nil")
	}
}

func TestTavilyCrawl_RetriesOn500ThenSucceeds(t *testing.T) {
	var calls int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/crawl" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"base_url":"example.com","results":[{"url":"https://example.com","raw_content":"after retry"}]}`))
	}))
	defer ts.Close()

	c := NewTavilyClient(ts.URL, "tvly-test")
	c.RetryConfig = RetryConfig{MaxAttempts: 3, BaseDelay: 0, MaxDelay: 0, Jitter: false}

	res, err := c.Crawl(context.Background(), TavilyCrawlRequest{URL: "https://example.com"})
	if err != nil {
		t.Fatalf("Crawl failed: %v", err)
	}
	if res.Results[0].RawContent != "after retry" {
		t.Errorf("RawContent = %q, want \"after retry\"", res.Results[0].RawContent)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("calls = %d, want 2 (initial 500 + retry)", got)
	}
}

func TestFormatTavilyCrawlContent(t *testing.T) {
	out := FormatTavilyCrawlContent(&TavilyCrawlResult{
		BaseURL:      "example.com",
		ResponseTime: 0.5,
		Results: []TavilyCrawlPage{{
			URL:        "https://example.com/a",
			RawContent: strings.Repeat("a", 20),
			Images:     []string{"https://example.com/a.png"},
		}},
	}, 5)
	for _, want := range []string{
		"base_url: example.com",
		"results_count: 1",
		"response_time: 0.50s",
		"content_chars: 20",
		"images_count: 1",
		"... [truncated]",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in %s", want, out)
		}
	}
}
