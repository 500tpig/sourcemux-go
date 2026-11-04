package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func jevTestQuestions() map[string]JEVQuestion {
	return map[string]JEVQuestion{"sufficient": {Type: "noul", Instructions: "Enough evidence?", Criteria: map[string]string{"true": "Enough", "false": "Missing facts"}}}
}

func TestJEVRequestAndKnownAnswers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/systemone" || r.Header.Get("Authorization") != "Bearer test-key" || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var body struct {
			Model     string                 `json:"model"`
			State     map[string]string      `json:"state"`
			Questions map[string]JEVQuestion `json:"questions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.Model != "jev-latest" || body.State["question"] != "test" || body.Questions["sufficient"].Criteria["true"] != "Enough" {
			t.Errorf("unexpected body: %+v", body)
		}
		fmt.Fprint(w, `{"answers":{"sufficient":{"type":"noul","noul":0.8},"unexpected":{"type":"command","noul":false}},"usage":{"input_tokens":12,"output_tokens":3}}`)
	}))
	defer server.Close()
	for _, suffix := range []string{"/v1/", "/v1/systemone"} {
		client := NewJEVClient(JEVConfig{APIURL: server.URL + suffix, APIKey: "test-key"})
		result, err := client.Evaluate(context.Background(), map[string]string{"question": "test"}, jevTestQuestions())
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Scores) != 1 || result.Scores["sufficient"] != 0.8 || result.InputTokens == nil || *result.InputTokens != 12 || result.OutputTokens == nil || *result.OutputTokens != 3 {
			t.Fatalf("unexpected result: %+v", result)
		}
	}
}

func TestJEVRejectsInvalidProbabilities(t *testing.T) {
	for _, answer := range []string{`null`, `{}`, `{"type":"other","noul":0.8}`, `{"type":"noul"}`, `{"type":"noul","noul":null}`, `{"type":"noul","noul":true}`, `{"type":"noul","noul":"0.8"}`, `{"type":"noul","noul":-0.1}`, `{"type":"noul","noul":1.1}`, `{"type":"noul","noul":1e999}`} {
		t.Run(answer, func(t *testing.T) {
			_, err := parseJEVJudgment([]byte(`{"answers":{"sufficient":`+answer+`}}`), jevTestQuestions())
			if err == nil {
				t.Fatal("accepted invalid answer")
			}
		})
	}
	for _, response := range []string{`{}`, `null`, `[]`, `{"answers":{}}`, `{"answers":null}`, `{"answers":{"other":{"type":"noul","noul":1}}}`} {
		if _, err := parseJEVJudgment([]byte(response), jevTestQuestions()); err == nil {
			t.Fatalf("accepted missing answer: %s", response)
		}
	}
	for _, value := range []string{"0", "1"} {
		if _, err := parseJEVJudgment([]byte(`{"answers":{"sufficient":{"type":"noul","noul":`+value+`}}}`), jevTestQuestions()); err != nil {
			t.Fatalf("rejected valid boundary: %v", err)
		}
	}
}

func TestJEVOptionalUsage(t *testing.T) {
	for _, usage := range []string{`null`, `[]`, `{"input_tokens":true,"output_tokens":-1}`, `{"input_tokens":1.5,"output_tokens":"5"}`} {
		result, err := parseJEVJudgment([]byte(`{"answers":{"sufficient":{"type":"noul","noul":1}},"usage":`+usage+`}`), jevTestQuestions())
		if err != nil || result.InputTokens != nil || result.OutputTokens != nil {
			t.Fatalf("usage = %s, result = %+v, err = %v", usage, result, err)
		}
	}
}

func TestJEVRetriesOnly429And529(t *testing.T) {
	for _, status := range []int{429, 529, 500, 401} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.WriteHeader(status)
				fmt.Fprint(w, "test-key private-error-body")
			}))
			defer server.Close()
			client := NewJEVClient(JEVConfig{APIURL: server.URL, APIKey: "test-key"})
			_, err := client.Evaluate(context.Background(), nil, jevTestQuestions())
			want := int32(1)
			if status == 429 || status == 529 {
				want = 3
			}
			if calls.Load() != want || err == nil || strings.Contains(err.Error(), "test-key") || strings.Contains(err.Error(), "private-error-body") {
				t.Fatalf("calls = %d, err = %v", calls.Load(), err)
			}
		})
	}
}

func TestJEVRetryThenSuccess(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(429)
			return
		}
		fmt.Fprint(w, `{"answers":{"sufficient":{"type":"noul","noul":1}}}`)
	}))
	defer server.Close()
	result, err := NewJEVClient(JEVConfig{APIURL: server.URL, APIKey: "test-key"}).Evaluate(context.Background(), nil, jevTestQuestions())
	if err != nil || calls.Load() != 2 || result.Scores["sufficient"] != 1 {
		t.Fatalf("calls = %d, result = %+v, err = %v", calls.Load(), result, err)
	}
}

func TestJEVDeadlineIncludesBackoff(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(429)
	}))
	defer server.Close()
	client := NewJEVClient(JEVConfig{APIURL: server.URL, APIKey: "test-key", TimeoutSec: 1})
	started := time.Now()
	_, err := client.Evaluate(context.Background(), nil, jevTestQuestions())
	if !errors.Is(err, context.DeadlineExceeded) || calls.Load() != 2 || time.Since(started) > 2*time.Second {
		t.Fatalf("calls = %d, elapsed = %v, err = %v", calls.Load(), time.Since(started), err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err = client.Evaluate(ctx, nil, jevTestQuestions())
	if !errors.Is(err, context.DeadlineExceeded) || calls.Load() != 3 {
		t.Fatalf("parent deadline ignored: calls = %d, err = %v", calls.Load(), err)
	}
}

func TestJEVCancelledBeforeRequestAndMissingKey(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := NewJEVClient(JEVConfig{APIURL: server.URL, APIKey: "test-key"})
	if _, err := client.Evaluate(ctx, nil, jevTestQuestions()); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation = %v", err)
	}
	client = NewJEVClient(JEVConfig{APIURL: server.URL})
	if client.Configured() {
		t.Fatal("missing key reported configured")
	}
	if _, err := client.Evaluate(context.Background(), nil, jevTestQuestions()); err == nil || calls.Load() != 0 {
		t.Fatalf("missing key: calls = %d, err = %v", calls.Load(), err)
	}
}

func TestJEVDeadlineCancelsRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		<-r.Context().Done()
	}))
	defer server.Close()
	client := NewJEVClient(JEVConfig{APIURL: server.URL, APIKey: "test-key"})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := client.Evaluate(ctx, nil, jevTestQuestions()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("request was not cancelled: %v", err)
	}
}

func TestJEVRejectsRedirectAndLargeResponse(t *testing.T) {
	var targetCalls atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { targetCalls.Add(1) }))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect/systemone" {
			http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
			return
		}
		fmt.Fprint(w, strings.Repeat("x", (1<<20)+1))
	}))
	defer server.Close()
	for _, suffix := range []string{"/redirect", ""} {
		client := NewJEVClient(JEVConfig{APIURL: server.URL + suffix, APIKey: "test-key"})
		if _, err := client.Evaluate(context.Background(), nil, jevTestQuestions()); err == nil {
			t.Fatal("expected failure")
		}
	}
	if targetCalls.Load() != 0 {
		t.Fatal("redirect sent judgment to another endpoint")
	}
}
