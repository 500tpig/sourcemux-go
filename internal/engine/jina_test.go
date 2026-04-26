package engine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestJinaFetch_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("expected GET, got %s", r.Method)
		}
		// Path on the test server includes the appended target verbatim.
		if !strings.HasPrefix(r.URL.Path, "/https:/") && !strings.HasPrefix(r.URL.Path, "/http:/") {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/markdown")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("# Example\n\nHello world."))
	}))
	defer ts.Close()

	c := NewJinaClient(ts.URL, "")
	res, err := c.Fetch(context.Background(), "https://example.com")
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}
	if res.URL != "https://example.com" {
		t.Errorf("URL = %q, want https://example.com", res.URL)
	}
	if !strings.Contains(res.Content, "Hello world.") {
		t.Errorf("content missing body: %q", res.Content)
	}
}

func TestJinaFetch_AuthHeaderForwardedWhenKeySet(t *testing.T) {
	var gotAuth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte("ok"))
	}))
	defer ts.Close()

	c := NewJinaClient(ts.URL, "jina-secret")
	if _, err := c.Fetch(context.Background(), "https://example.com"); err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}
	if gotAuth != "Bearer jina-secret" {
		t.Errorf("Authorization = %q, want Bearer jina-secret", gotAuth)
	}
}

func TestJinaFetch_NoAuthHeaderWhenKeyEmpty(t *testing.T) {
	var gotAuth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte("ok"))
	}))
	defer ts.Close()

	c := NewJinaClient(ts.URL, "")
	if _, err := c.Fetch(context.Background(), "https://example.com"); err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}
	if gotAuth != "" {
		t.Errorf("Authorization unexpectedly set to %q", gotAuth)
	}
}

func TestJinaFetch_HTTPErrorIncludesStatusAndBody(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("bad token"))
	}))
	defer ts.Close()

	c := NewJinaClient(ts.URL, "x")
	_, err := c.Fetch(context.Background(), "https://example.com")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "401") || !strings.Contains(err.Error(), "bad token") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestJinaFetch_EmptyResponseIsError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	c := NewJinaClient(ts.URL, "")
	_, err := c.Fetch(context.Background(), "https://example.com")
	if err == nil {
		t.Fatal("expected empty-content error, got nil")
	}
	if !strings.Contains(err.Error(), "empty content") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestJinaFetch_EmptyTargetIsError(t *testing.T) {
	c := NewJinaClient("https://r.jina.ai", "")
	_, err := c.Fetch(context.Background(), "")
	if err == nil {
		t.Fatal("expected empty-target error, got nil")
	}
}

func TestNewJinaClient_DefaultBaseURL(t *testing.T) {
	c := NewJinaClient("", "")
	if c.BaseURL != "https://r.jina.ai" {
		t.Errorf("default BaseURL = %q, want https://r.jina.ai", c.BaseURL)
	}
}

func TestNewJinaClient_TrimsTrailingSlash(t *testing.T) {
	c := NewJinaClient("https://r.jina.ai/", "")
	if c.BaseURL != "https://r.jina.ai" {
		t.Errorf("BaseURL = %q, want trimmed", c.BaseURL)
	}
}

func TestJinaFetch_RetriesOn429ThenSucceeds(t *testing.T) {
	var calls int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "text/markdown")
		_, _ = w.Write([]byte("# Example\n\nHello after retry."))
	}))
	defer ts.Close()

	c := NewJinaClient(ts.URL, "")
	c.RetryConfig = RetryConfig{MaxAttempts: 3, BaseDelay: 0, MaxDelay: 0, Jitter: false}

	res, err := c.Fetch(context.Background(), "https://example.com")
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}
	if !strings.Contains(res.Content, "Hello after retry.") {
		t.Errorf("content missing body: %q", res.Content)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("calls = %d, want 2 (initial 429 + retry)", got)
	}
}
