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

func TestFirecrawlScrapeRequestAndResponse(t *testing.T) {
	var gotBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/scrape" {
			t.Fatalf("path = %s, want /scrape", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer fc-test" {
			t.Fatalf("Authorization = %q", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("Content-Type = %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"markdown":"# Hello","links":["https://example.com/a"],"metadata":{"title":"Hello","sourceURL":"https://example.com/final"}}}`))
	}))
	defer ts.Close()

	onlyMain := true
	mobile := true
	c := NewFirecrawlClient(ts.URL, "fc-test")
	res, err := c.Scrape(context.Background(), FirecrawlScrapeRequest{
		URL:             "https://example.com",
		Formats:         []string{"markdown", "links"},
		OnlyMainContent: &onlyMain,
		Mobile:          &mobile,
		WaitFor:         250,
		Proxy:           "auto",
	})
	if err != nil {
		t.Fatalf("Scrape failed: %v", err)
	}
	if res.Data.Markdown != "# Hello" || res.Data.Metadata.SourceURL != "https://example.com/final" {
		t.Fatalf("result = %+v", res)
	}
	if gotBody["url"] != "https://example.com" || gotBody["waitFor"] != float64(250) || gotBody["proxy"] != "auto" {
		t.Fatalf("body = %#v", gotBody)
	}
	formats, ok := gotBody["formats"].([]any)
	if !ok || len(formats) != 2 || formats[0] != "markdown" || formats[1] != "links" {
		t.Fatalf("formats = %#v", gotBody["formats"])
	}
	if gotBody["onlyMainContent"] != true || gotBody["mobile"] != true {
		t.Fatalf("bool fields = %#v", gotBody)
	}
}

func TestFirecrawlScrapeDefaultsMarkdownAndRejectsEmptyContent(t *testing.T) {
	var gotBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"markdown":"   "}}`))
	}))
	defer ts.Close()

	c := NewFirecrawlClient(ts.URL, "fc-test")
	_, err := c.Scrape(context.Background(), FirecrawlScrapeRequest{URL: "https://example.com"})
	if err == nil || !strings.Contains(err.Error(), "empty markdown") {
		t.Fatalf("err = %v, want empty markdown", err)
	}
	formats, ok := gotBody["formats"].([]any)
	if !ok || len(formats) != 1 || formats[0] != "markdown" {
		t.Fatalf("formats = %#v", gotBody["formats"])
	}
}

func TestFirecrawlMapRequestAndResponse(t *testing.T) {
	var gotBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/map" {
			t.Fatalf("path = %s, want /map", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer fc-test" {
			t.Fatalf("Authorization = %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"links":[{"url":"https://example.com/docs","title":"Docs"},{"url":"https://example.com/blog","description":"Blog"}]}`))
	}))
	defer ts.Close()

	includeSubs := false
	ignoreQuery := true
	c := NewFirecrawlClient(ts.URL, "fc-test")
	res, err := c.Map(context.Background(), FirecrawlMapRequest{
		URL:                   "https://example.com",
		Search:                "docs",
		Sitemap:               "only",
		IncludeSubdomains:     &includeSubs,
		IgnoreQueryParameters: &ignoreQuery,
		Limit:                 25,
		Timeout:               60000,
	})
	if err != nil {
		t.Fatalf("Map failed: %v", err)
	}
	if len(res.Links) != 2 || res.Links[0].URL != "https://example.com/docs" {
		t.Fatalf("links = %+v", res.Links)
	}
	if gotBody["search"] != "docs" || gotBody["limit"] != float64(25) || gotBody["sitemap"] != "only" || gotBody["timeout"] != float64(60000) {
		t.Fatalf("body = %#v", gotBody)
	}
	if gotBody["includeSubdomains"] != false || gotBody["ignoreQueryParameters"] != true {
		t.Fatalf("bool body = %#v", gotBody)
	}
}

func TestFirecrawlHTTPStatusErrors(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				_, _ = w.Write([]byte("upstream failure"))
			}))
			defer ts.Close()

			c := NewFirecrawlClient(ts.URL, "fc-test")
			_, err := c.Scrape(context.Background(), FirecrawlScrapeRequest{URL: "https://example.com"})
			if err == nil || !strings.Contains(err.Error(), "firecrawl API") || !strings.Contains(err.Error(), "upstream failure") {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestFirecrawlHTTPStatusErrorsRedactSecret(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`bad key fc-secret`))
	}))
	defer ts.Close()

	c := NewFirecrawlClient(ts.URL, "fc-secret")
	_, err := c.Scrape(context.Background(), FirecrawlScrapeRequest{URL: "https://example.com"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if strings.Contains(err.Error(), "fc-secret") || !strings.Contains(err.Error(), "<redacted>") {
		t.Fatalf("err = %v, want redacted secret", err)
	}
}

func TestFirecrawlPoolScrapeFallsBackAcrossKeys(t *testing.T) {
	var seen []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		seen = append(seen, key)
		if key == "fc-limited" {
			http.Error(w, `{"error":"rate limited","key":"fc-limited"}`, http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"markdown":"fallback content"}}`))
	}))
	defer ts.Close()

	pool := NewFirecrawlPool([]FirecrawlKey{
		{Name: "limited", APIKey: "fc-limited"},
		{Name: "ok", APIKey: "fc-ok"},
	}, ts.URL)
	pool.next = 0

	res, err := pool.Scrape(context.Background(), FirecrawlScrapeRequest{URL: "https://example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if res.KeyName != "ok" || res.Data.Markdown != "fallback content" {
		t.Fatalf("result = %+v", res)
	}
	if strings.Join(seen, ",") != "fc-limited,fc-ok" {
		t.Fatalf("seen keys = %v", seen)
	}
}

func TestFirecrawlPoolScrapeRotatesStartKey(t *testing.T) {
	var seen []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		seen = append(seen, key)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"markdown":"` + key + `"}}`))
	}))
	defer ts.Close()

	pool := NewFirecrawlPool([]FirecrawlKey{
		{Name: "a", APIKey: "fc-a"},
		{Name: "b", APIKey: "fc-b"},
	}, ts.URL)
	pool.next = 0

	first, err := pool.Scrape(context.Background(), FirecrawlScrapeRequest{URL: "https://example.com"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := pool.Scrape(context.Background(), FirecrawlScrapeRequest{URL: "https://example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if first.KeyName != "a" || second.KeyName != "b" {
		t.Fatalf("key names = %q, %q", first.KeyName, second.KeyName)
	}
	if strings.Join(seen, ",") != "fc-a,fc-b" {
		t.Fatalf("seen keys = %v", seen)
	}
}

func TestFirecrawlRetries500ThenSucceeds(t *testing.T) {
	var calls int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"links":[{"url":"https://example.com/a"}]}`))
	}))
	defer ts.Close()

	c := NewFirecrawlClient(ts.URL, "fc-test")
	c.RetryConfig = RetryConfig{MaxAttempts: 2, BaseDelay: 0, MaxDelay: 0, Jitter: false}
	res, err := c.Map(context.Background(), FirecrawlMapRequest{URL: "https://example.com"})
	if err != nil {
		t.Fatalf("Map failed: %v", err)
	}
	if len(res.Links) != 1 || atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("links=%+v calls=%d", res.Links, calls)
	}
}

func TestNewFirecrawlClientDefaultsBaseURL(t *testing.T) {
	c := NewFirecrawlClient("", "fc-test")
	if c.BaseURL != DefaultFirecrawlAPIURL {
		t.Fatalf("BaseURL = %q, want %q", c.BaseURL, DefaultFirecrawlAPIURL)
	}
}
