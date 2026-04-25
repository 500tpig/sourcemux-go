package engine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func newMockGrok(t *testing.T, status int, body string) *GrokClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return NewGrokClient(srv.URL, "test-key", "grok-3-mini")
}

func TestSearch_CitationsField(t *testing.T) {
	body := `{
		"choices":[{"message":{"content":"Grok answer."}}],
		"citations":["https://a.example.com/x","https://b.example.com/y","https://a.example.com/x"]
	}`
	c := newMockGrok(t, 200, body)
	res, err := c.Search(context.Background(), "q")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"https://a.example.com/x", "https://b.example.com/y"}
	if !reflect.DeepEqual(res.SourceURLs, want) {
		t.Fatalf("urls = %v, want %v", res.SourceURLs, want)
	}
	if res.SourcesCount != 2 {
		t.Fatalf("count = %d, want 2", res.SourcesCount)
	}
	if res.Content != "Grok answer." {
		t.Fatalf("content = %q", res.Content)
	}
}

func TestSearch_SearchResultsField(t *testing.T) {
	body := `{
		"choices":[{"message":{"content":"Answer"}}],
		"search_results":[{"url":"https://x.example.com"},{"url":"https://y.example.com"},{"url":""}]
	}`
	c := newMockGrok(t, 200, body)
	res, err := c.Search(context.Background(), "q")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"https://x.example.com", "https://y.example.com"}
	if !reflect.DeepEqual(res.SourceURLs, want) {
		t.Fatalf("urls = %v, want %v", res.SourceURLs, want)
	}
	if res.SourcesCount != 2 {
		t.Fatalf("count = %d, want 2", res.SourcesCount)
	}
}

func TestSearch_FallbackContentURLs(t *testing.T) {
	// Two unique URLs; the second appears again inside parens to test dedup +
	// the trailing-punctuation stripping for the period that ends the sentence.
	body := `{"choices":[{"message":{"content":"See https://docs.example.com/a and https://docs.example.com/b for refs (also https://docs.example.com/a)."}}]}`
	c := newMockGrok(t, 200, body)
	res, err := c.Search(context.Background(), "q")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"https://docs.example.com/a", "https://docs.example.com/b"}
	if !reflect.DeepEqual(res.SourceURLs, want) {
		t.Fatalf("urls = %v, want %v", res.SourceURLs, want)
	}
	if res.SourcesCount != 2 {
		t.Fatalf("count = %d, want 2", res.SourcesCount)
	}
}

func TestSearch_NoSources(t *testing.T) {
	body := `{"choices":[{"message":{"content":"plain text answer with no urls"}}]}`
	c := newMockGrok(t, 200, body)
	res, err := c.Search(context.Background(), "q")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.SourceURLs) != 0 {
		t.Fatalf("expected 0 urls, got %v", res.SourceURLs)
	}
	if res.SourcesCount != 0 {
		t.Fatalf("expected 0 count, got %d", res.SourcesCount)
	}
}

func TestSearch_HTTPErrorIncludesStatus(t *testing.T) {
	c := newMockGrok(t, http.StatusUnauthorized, `{"error":"bad key"}`)
	_, err := c.Search(context.Background(), "q")
	if err == nil {
		t.Fatal("expected error for 401")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Fatalf("error should include status 401, got %v", err)
	}
}

func TestSearch_PrefersCitationsOverSearchResults(t *testing.T) {
	body := `{
		"choices":[{"message":{"content":"x"}}],
		"citations":["https://primary.example.com"],
		"search_results":[{"url":"https://secondary.example.com"}]
	}`
	c := newMockGrok(t, 200, body)
	res, err := c.Search(context.Background(), "q")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(res.SourceURLs, []string{"https://primary.example.com"}) {
		t.Fatalf("expected primary citation only, got %v", res.SourceURLs)
	}
}

func TestExtractSourceURLs_Dedup(t *testing.T) {
	in := []string{"https://a", "https://b", "https://a", ""}
	got := dedupURLs(in)
	want := []string{"https://a", "https://b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("dedupURLs = %v, want %v", got, want)
	}
}
