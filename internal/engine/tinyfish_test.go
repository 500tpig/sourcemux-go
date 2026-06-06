package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTinyFishSearchRequestAndResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if got := r.Header.Get("X-API-Key"); got != "tf-secret" {
			t.Fatalf("X-API-Key = %q", got)
		}
		q := r.URL.Query()
		if q.Get("query") != "web automation" || q.Get("location") != "US" || q.Get("language") != "en" || q.Get("page") != "2" {
			t.Fatalf("query params = %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"query":"web automation",
			"results":[{"position":1,"site_name":"example.com","title":"Example","snippet":"Snippet","url":"https://example.com"}],
			"total_results":1,
			"page":2
		}`))
	}))
	defer srv.Close()

	page := 2
	client := NewTinyFishClient("tf-secret")
	client.SearchURL = srv.URL
	res, err := client.Search(context.Background(), TinyFishSearchRequest{
		Query:    "web automation",
		Location: "US",
		Language: "en",
		Page:     &page,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.HTTPStatus != 200 || res.TotalResults != 1 || len(res.Results) != 1 {
		t.Fatalf("response = %+v", res)
	}
	if res.Results[0].URL != "https://example.com" {
		t.Fatalf("result URL = %q", res.Results[0].URL)
	}
}

func TestTinyFishFetchRequestAndResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
			t.Fatalf("Content-Type = %q", ct)
		}
		var req TinyFishFetchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if len(req.URLs) != 1 || req.URLs[0] != "https://example.com" || req.Format != "markdown" || !req.Links {
			t.Fatalf("request = %+v", req)
		}
		latency := FlexibleNumber(42)
		resp := TinyFishFetchResponse{
			Results: []TinyFishFetchResult{{
				URL:       "https://example.com",
				FinalURL:  "https://www.example.com",
				Title:     "Example",
				Text:      json.RawMessage(`"hello world"`),
				LatencyMS: &latency,
				Format:    "markdown",
			}},
			Errors: []TinyFishFetchFailure{{URL: "https://bad.example", Code: "timeout", Error: "timed out"}},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewTinyFishClient("tf-secret")
	client.FetchURL = srv.URL
	res, err := client.Fetch(context.Background(), TinyFishFetchRequest{
		URLs:   []string{"https://example.com"},
		Format: "markdown",
		Links:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.HTTPStatus != 200 || len(res.Results) != 1 || len(res.Errors) != 1 {
		t.Fatalf("response = %+v", res)
	}
	if res.Errors[0].Code != "timeout" {
		t.Fatalf("error code = %q", res.Errors[0].Code)
	}
	if got := TinyFishTextLength(res.Results[0].Text); got != len("hello world") {
		t.Fatalf("TinyFishTextLength = %d", got)
	}
}

func TestTinyFishFetchAcceptsFloatLatencyMS(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"results": [{
				"url": "https://example.com",
				"text": "hello",
				"latency_ms": 42.5,
				"format": "markdown"
			}]
		}`))
	}))
	defer srv.Close()

	client := NewTinyFishClient("tf-secret")
	client.FetchURL = srv.URL
	res, err := client.Fetch(context.Background(), TinyFishFetchRequest{URLs: []string{"https://example.com"}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Results[0].LatencyMS == nil || float64(*res.Results[0].LatencyMS) != 42.5 {
		t.Fatalf("latency_ms = %v", res.Results[0].LatencyMS)
	}
}

func TestTinyFishAgentRequestAndResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/automation/run" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if r.Header.Get("X-API-Key") != "tf-secret" {
			t.Fatalf("missing API key")
		}
		var req TinyFishAgentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.URL != "https://example.com" || req.Goal != "find pricing" || req.BrowserProfile != "lite" {
			t.Fatalf("request = %+v", req)
		}
		if req.AgentConfig["max_steps"].(float64) != 5 {
			t.Fatalf("agent_config = %+v", req.AgentConfig)
		}
		steps := 3
		_ = json.NewEncoder(w).Encode(TinyFishAgentResponse{
			RunID:      "run-1",
			Status:     "COMPLETED",
			NumOfSteps: &steps,
			Steps:      json.RawMessage(`[{"action":"open","status":"ok"}]`),
			Result:     json.RawMessage(`{"answer":"ok"}`),
		})
	}))
	defer srv.Close()

	client := NewTinyFishClient("tf-secret")
	client.AgentURL = srv.URL + "/v1/automation/run"
	res, err := client.RunAgent(context.Background(), TinyFishAgentRequest{
		URL:            "https://example.com",
		Goal:           "find pricing",
		BrowserProfile: "lite",
		AgentConfig:    map[string]any{"max_steps": 5},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.HTTPStatus != 200 || res.RunID != "run-1" || res.NumOfSteps == nil || *res.NumOfSteps != 3 {
		t.Fatalf("response = %+v", res)
	}
	if !strings.Contains(string(res.Steps), `"action":"open"`) {
		t.Fatalf("steps = %s", res.Steps)
	}
}

func TestTinyFishHTTPErrorAndJSONTextLength(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"rate limited","key":"tf-secret"}`, http.StatusTooManyRequests)
	}))
	defer srv.Close()

	client := NewTinyFishClient("tf-secret")
	client.SearchURL = srv.URL
	res, err := client.Search(context.Background(), TinyFishSearchRequest{Query: "q"})
	if err == nil {
		t.Fatal("expected error")
	}
	if res == nil || res.HTTPStatus != http.StatusTooManyRequests {
		t.Fatalf("response = %+v", res)
	}
	if !strings.Contains(err.Error(), "429") || !strings.Contains(err.Error(), "rate limited") {
		t.Fatalf("error = %v", err)
	}
	if strings.Contains(err.Error(), "tf-secret") {
		t.Fatalf("full key leaked in error: %v", err)
	}
	if got := TinyFishTextLength(json.RawMessage(`{"nodes":[1,2]}`)); got != len(`{"nodes":[1,2]}`) {
		t.Fatalf("JSON text length = %d", got)
	}
}

func TestTinyFishPoolFetchFallsBackAcrossKeys(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Header.Get("X-API-Key"))
		if r.Header.Get("X-API-Key") == "tf-limited" {
			http.Error(w, `{"error":"rate limited","key":"tf-limited"}`, http.StatusTooManyRequests)
			return
		}
		_ = json.NewEncoder(w).Encode(TinyFishFetchResponse{
			Results: []TinyFishFetchResult{{
				URL:    "https://example.com",
				Text:   json.RawMessage(`"fallback content"`),
				Format: "markdown",
			}},
		})
	}))
	defer srv.Close()

	pool := NewTinyFishPool([]TinyFishKey{
		{Name: "limited", APIKey: "tf-limited"},
		{Name: "ok", APIKey: "tf-ok"},
	}, srv.URL, srv.URL)

	res, err := pool.Fetch(context.Background(), TinyFishFetchRequest{
		URLs: []string{"https://example.com"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.KeyName != "ok" || TinyFishFetchContent(res.TinyFishFetchResponse) != "fallback content" {
		t.Fatalf("result = %+v content=%q", res, TinyFishFetchContent(res.TinyFishFetchResponse))
	}
	if strings.Join(seen, ",") != "tf-limited,tf-ok" {
		t.Fatalf("seen keys = %v", seen)
	}
}

func TestTinyFishPoolSearchRotatesStartKey(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("X-API-Key")
		seen = append(seen, key)
		_ = json.NewEncoder(w).Encode(TinyFishSearchResponse{
			Query: r.URL.Query().Get("query"),
			Results: []TinyFishSearchResult{{
				Position: 1,
				Title:    key,
				URL:      "https://" + key + ".example",
			}},
			TotalResults: 1,
		})
	}))
	defer srv.Close()

	pool := NewTinyFishPool([]TinyFishKey{
		{Name: "a", APIKey: "tf-a"},
		{Name: "b", APIKey: "tf-b"},
	}, srv.URL, srv.URL)

	first, err := pool.Search(context.Background(), TinyFishSearchRequest{Query: "q"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := pool.Search(context.Background(), TinyFishSearchRequest{Query: "q"})
	if err != nil {
		t.Fatal(err)
	}
	if first.KeyName != "a" || second.KeyName != "b" {
		t.Fatalf("key names = %q, %q", first.KeyName, second.KeyName)
	}
	if strings.Join(seen, ",") != "tf-a,tf-b" {
		t.Fatalf("seen keys = %v", seen)
	}
}

func TestTinyFishSearchFormattingAndSourceURLs(t *testing.T) {
	res := &TinyFishSearchResponse{
		Results: []TinyFishSearchResult{
			{Title: "A", SiteName: "example.com", Snippet: "Alpha", URL: "https://example.com/a"},
			{Title: "A dup", URL: "https://example.com/a"},
			{Title: "B", URL: "https://example.com/b"},
		},
	}
	urls := TinyFishSearchSourceURLs(res)
	if strings.Join(urls, ",") != "https://example.com/a,https://example.com/b" {
		t.Fatalf("urls = %v", urls)
	}
	body := FormatTinyFishSearchContent(res)
	if !strings.Contains(body, "TinyFish returned source-first search results") || !strings.Contains(body, "Snippet: Alpha") {
		t.Fatalf("body = %s", body)
	}
}
