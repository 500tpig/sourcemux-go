package engine

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
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

func TestSearch_AnnotationsField(t *testing.T) {
	body := `{
		"choices":[{"message":{
			"content":"answer",
			"annotations":[
				{"type":"url_citation","url_citation":{"url":"https://ann1.example.com","title":"A1"}},
				{"type":"url_citation","url_citation":{"url":"https://ann2.example.com","title":"A2"}}
			]
		}}]
	}`
	c := newMockGrok(t, 200, body)
	res, err := c.Search(context.Background(), "q")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"https://ann1.example.com", "https://ann2.example.com"}
	if !reflect.DeepEqual(res.SourceURLs, want) {
		t.Fatalf("urls = %v, want %v", res.SourceURLs, want)
	}
}

func TestSearch_SearchSourcesField(t *testing.T) {
	body := `{
		"choices":[{"message":{"content":"answer"}}],
		"search_sources":[
			{"url":"https://ss1.example.com","title":"S1","type":"web"},
			{"url":"https://ss2.example.com","title":"S2","type":"x_post"}
		]
	}`
	c := newMockGrok(t, 200, body)
	res, err := c.Search(context.Background(), "q")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"https://ss1.example.com", "https://ss2.example.com"}
	if !reflect.DeepEqual(res.SourceURLs, want) {
		t.Fatalf("urls = %v, want %v", res.SourceURLs, want)
	}
}

func TestSearch_AnnotationsBeatsAllOthers(t *testing.T) {
	body := `{
		"choices":[{"message":{
			"content":"x",
			"annotations":[{"type":"url_citation","url_citation":{"url":"https://primary.example.com"}}]
		}}],
		"search_sources":[{"url":"https://b.example.com"}],
		"citations":["https://c.example.com"],
		"search_results":[{"url":"https://d.example.com"}]
	}`
	c := newMockGrok(t, 200, body)
	res, err := c.Search(context.Background(), "q")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(res.SourceURLs, []string{"https://primary.example.com"}) {
		t.Fatalf("expected annotations to win, got %v", res.SourceURLs)
	}
}

func TestSearch_SearchSourcesBeatsCitations(t *testing.T) {
	body := `{
		"choices":[{"message":{"content":"x"}}],
		"search_sources":[{"url":"https://ss.example.com"}],
		"citations":["https://cit.example.com"]
	}`
	c := newMockGrok(t, 200, body)
	res, err := c.Search(context.Background(), "q")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(res.SourceURLs, []string{"https://ss.example.com"}) {
		t.Fatalf("expected search_sources to win, got %v", res.SourceURLs)
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

func TestSearch_SendSearchFlagControl(t *testing.T) {
	var receivedBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf, _ := io.ReadAll(r.Body)
		receivedBody = string(buf)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"x"}}]}`))
	}))
	t.Cleanup(srv.Close)

	c := NewGrokClient(srv.URL, "k", "m")
	c.SendSearchFlag = false
	if _, err := c.Search(context.Background(), "q"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(receivedBody, `"search":true`) {
		t.Fatalf("search flag should be omitted; body=%s", receivedBody)
	}

	c.SendSearchFlag = true
	if _, err := c.Search(context.Background(), "q"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(receivedBody, `"search":true`) {
		t.Fatalf("search flag should be present; body=%s", receivedBody)
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

func TestListModels_RetriesOn429ThenSucceeds(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"grok-3-mini"},{"id":"grok-4-fast"}]}`))
	}))
	t.Cleanup(srv.Close)

	c := NewGrokClient(srv.URL, "test-key", "grok-3-mini")
	c.RetryConfig = RetryConfig{MaxAttempts: 3, BaseDelay: 0, MaxDelay: 0, Jitter: false}

	models, err := c.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels failed: %v", err)
	}
	want := []string{"grok-3-mini", "grok-4-fast"}
	if !reflect.DeepEqual(models, want) {
		t.Fatalf("models = %v, want %v", models, want)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("calls = %d, want 2 (initial 429 + retry)", got)
	}
}
