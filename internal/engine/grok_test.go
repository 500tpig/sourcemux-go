package engine

import (
	"bufio"
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

func TestSearch_EventStreamResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		bw := bufio.NewWriter(w)
		_, _ = bw.WriteString(": heartbeat stream connected\n\n")
		_, _ = bw.WriteString(`data: {"choices":[{"delta":{"content":"Hello "}}]}` + "\n\n")
		_, _ = bw.WriteString(`data: {"choices":[{"delta":{"content":"world."}}]}` + "\n\n")
		_, _ = bw.WriteString(`data: {"choices":[{"message":{"annotations":[{"type":"url_citation","url_citation":{"url":"https://example.com/source"}}]}}]}` + "\n\n")
		_, _ = bw.WriteString("data: [DONE]\n\n")
		_ = bw.Flush()
	}))
	t.Cleanup(srv.Close)

	c := NewGrokClient(srv.URL, "k", "m")
	c.SendSearchFlag = false
	res, err := c.Search(context.Background(), "q")
	if err != nil {
		t.Fatal(err)
	}
	if res.Content != "Hello world." {
		t.Fatalf("Content = %q, want Hello world.", res.Content)
	}
	if !reflect.DeepEqual(res.SourceURLs, []string{"https://example.com/source"}) {
		t.Fatalf("SourceURLs = %v", res.SourceURLs)
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

// --- Responses API tests ---

// newMockResponsesGrok creates a GrokClient with apiType="responses" pointed at a mock server.
func newMockResponsesGrok(t *testing.T, status int, body string) *GrokClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	c := NewGrokClient(srv.URL, "test-key", "grok-4.20-reasoning")
	c.APIType = "responses"
	return c
}

func TestSearch_ResponsesAPI_Basic(t *testing.T) {
	body := `{
		"output": [
			{
				"type": "message",
				"role": "assistant",
				"content": [
					{
						"type": "output_text",
						"text": "Grok 4.20 answer.",
						"annotations": [
							{"type":"url_citation","url":"https://a.example.com/1","title":"A1"},
							{"type":"url_citation","url":"https://b.example.com/2","title":"B2"},
							{"type":"url_citation","url":"https://a.example.com/1","title":"A1 dup"}
						]
					}
				]
			}
		]
	}`
	c := newMockResponsesGrok(t, 200, body)
	res, err := c.Search(context.Background(), "q")
	if err != nil {
		t.Fatal(err)
	}
	if res.Content != "Grok 4.20 answer." {
		t.Fatalf("Content = %q", res.Content)
	}
	want := []string{"https://a.example.com/1", "https://b.example.com/2"}
	if !reflect.DeepEqual(res.SourceURLs, want) {
		t.Fatalf("SourceURLs = %v, want %v", res.SourceURLs, want)
	}
	if res.SourcesCount != 2 {
		t.Fatalf("SourcesCount = %d, want 2", res.SourcesCount)
	}
}

func TestSearch_ResponsesAPI_MultipleOutputBlocks(t *testing.T) {
	body := `{
		"output": [
			{
				"type": "message",
				"role": "assistant",
				"content": [
					{"type": "output_text", "text": "Part one.", "annotations": [{"type":"url_citation","url":"https://src1.example.com"}]},
					{"type": "output_text", "text": "Part two.", "annotations": [{"type":"url_citation","url":"https://src2.example.com"}]}
				]
			}
		]
	}`
	c := newMockResponsesGrok(t, 200, body)
	res, err := c.Search(context.Background(), "q")
	if err != nil {
		t.Fatal(err)
	}
	if res.Content != "Part one.\nPart two." {
		t.Fatalf("Content = %q", res.Content)
	}
	want := []string{"https://src1.example.com", "https://src2.example.com"}
	if !reflect.DeepEqual(res.SourceURLs, want) {
		t.Fatalf("SourceURLs = %v, want %v", res.SourceURLs, want)
	}
}

func TestSearch_ResponsesAPI_TopLevelCitationsFallback(t *testing.T) {
	body := `{
		"output": [
			{
				"type": "message",
				"role": "assistant",
				"content": [{"type": "output_text", "text": "Answer with no annotations."}]
			}
		],
		"citations": ["https://cite1.example.com", "https://cite2.example.com"]
	}`
	c := newMockResponsesGrok(t, 200, body)
	res, err := c.Search(context.Background(), "q")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"https://cite1.example.com", "https://cite2.example.com"}
	if !reflect.DeepEqual(res.SourceURLs, want) {
		t.Fatalf("SourceURLs = %v, want %v", res.SourceURLs, want)
	}
}

func TestSearch_ResponsesAPI_RegexFallback(t *testing.T) {
	body := `{
		"output": [
			{
				"type": "message",
				"role": "assistant",
				"content": [{"type": "output_text", "text": "See https://fallback.example.com/page for details."}]
			}
		]
	}`
	c := newMockResponsesGrok(t, 200, body)
	res, err := c.Search(context.Background(), "q")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(res.SourceURLs, []string{"https://fallback.example.com/page"}) {
		t.Fatalf("SourceURLs = %v", res.SourceURLs)
	}
}

func TestSearch_ResponsesAPI_SkipsNonMessageOutput(t *testing.T) {
	body := `{
		"output": [
			{"type": "web_search_call", "content": []},
			{
				"type": "message",
				"role": "assistant",
				"content": [
					{"type": "output_text", "text": "Final answer.", "annotations": [{"type":"url_citation","url":"https://real.example.com"}]}
				]
			}
		]
	}`
	c := newMockResponsesGrok(t, 200, body)
	res, err := c.Search(context.Background(), "q")
	if err != nil {
		t.Fatal(err)
	}
	if res.Content != "Final answer." {
		t.Fatalf("Content = %q", res.Content)
	}
	if !reflect.DeepEqual(res.SourceURLs, []string{"https://real.example.com"}) {
		t.Fatalf("SourceURLs = %v", res.SourceURLs)
	}
}

func TestSearch_ResponsesAPI_SendsSearchTool(t *testing.T) {
	var receivedBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf, _ := io.ReadAll(r.Body)
		receivedBody = string(buf)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"output":[{"type":"message","content":[{"type":"output_text","text":"x"}]}]}`))
	}))
	t.Cleanup(srv.Close)

	c := NewGrokClient(srv.URL, "k", "m")
	c.APIType = "responses"
	c.SendSearchFlag = true
	if _, err := c.Search(context.Background(), "q"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(receivedBody, `"web_search"`) {
		t.Fatalf("expected web_search tool in body; got %s", receivedBody)
	}
	if strings.Contains(receivedBody, `"search":true`) {
		t.Fatalf("chat-completions search flag should not appear in responses API body; got %s", receivedBody)
	}
	if !strings.Contains(receivedBody, `"store":false`) {
		t.Fatalf("store:false should be set; got %s", receivedBody)
	}
}

func TestSearch_ResponsesAPI_RoutesToCorrectEndpoint(t *testing.T) {
	var calledPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calledPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"output":[{"type":"message","content":[{"type":"output_text","text":"x"}]}]}`))
	}))
	t.Cleanup(srv.Close)

	c := NewGrokClient(srv.URL, "k", "m")
	c.APIType = "responses"
	c.SendSearchFlag = false
	if _, err := c.Search(context.Background(), "q"); err != nil {
		t.Fatal(err)
	}
	if calledPath != "/responses" {
		t.Fatalf("expected path /responses, got %s", calledPath)
	}
}

func TestSearch_ResponsesAPI_HTTPError(t *testing.T) {
	c := newMockResponsesGrok(t, http.StatusUnauthorized, `{"error":"bad key"}`)
	_, err := c.Search(context.Background(), "q")
	if err == nil {
		t.Fatal("expected error for 401")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Fatalf("error should contain status 401, got %v", err)
	}
}

func TestSearch_ResponsesAPI_SSEReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		_, _ = w.Write([]byte("data: {}\n\n"))
	}))
	t.Cleanup(srv.Close)

	c := NewGrokClient(srv.URL, "k", "m")
	c.APIType = "responses"
	c.SendSearchFlag = false
	_, err := c.Search(context.Background(), "q")
	if err == nil {
		t.Fatal("expected error for SSE response in responses API mode")
	}
	if !strings.Contains(err.Error(), "streaming") {
		t.Fatalf("error should mention streaming, got %v", err)
	}
}

