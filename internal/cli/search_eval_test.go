package cli

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/500tpig/sourcemux-go/internal/config"
	"github.com/500tpig/sourcemux-go/internal/engine"
)

func TestRunSearchEvalCasesUsesFakeEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"choices":[{"message":{"content":"release v1.2.3"}}],
			"citations":["[release](https://github.com/acme/project/releases/tag/v1.2.3)"]
		}`)
	}))
	defer server.Close()

	minSources := 1
	report, err := runSearchEvalCases(context.Background(), &config.Config{
		GrokEndpoints: []engine.GrokEndpoint{{
			Name: "fake", BaseURL: server.URL, APIKey: "sk-test", Model: "grok-test", Profile: "heavy",
		}},
	}, searchEvalSuite{Cases: []searchEvalCase{{
		Name:         "release",
		Query:        "latest release",
		TruthCommand: "gh api repos/acme/project/releases/latest",
		Expect: searchEvalExpect{
			MinValidSources:      &minSources,
			MinStructuredSources: &minSources,
			ContentContains:      []string{"v1.2.3"},
			SourceDomainsInclude: []string{"github.com"},
		},
	}}}, searchEvalOptions{Repeats: 2, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if report.CaseCount != 1 || report.EndpointCount != 1 || len(report.Measurements) != 2 || len(report.Summaries) != 1 {
		t.Fatalf("report shape = %+v", report)
	}
	for _, measurement := range report.Measurements {
		if !measurement.OK || measurement.Status != "ok" || measurement.SourceOrigin != engine.SourceOriginCitations {
			t.Fatalf("measurement = %+v", measurement)
		}
		if measurement.StructuredSourcesCount != 1 || measurement.TruthCommand == "" {
			t.Fatalf("measurement metadata = %+v", measurement)
		}
	}
	if summary := report.Summaries[0]; summary.Passed != 2 || summary.ValidSourceBacked != 2 || summary.StructuredBacked != 2 || summary.RegexOnly != 0 {
		t.Fatalf("summary = %+v", summary)
	}
}

func TestSearchEvalRedactsEndpointSecret(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":"echo sk-private"}`)
	}))
	defer server.Close()

	measurement := runSearchEvalMeasurement(context.Background(), engine.GrokEndpoint{
		Name: "fake", BaseURL: server.URL, APIKey: "sk-private", Model: "grok-test",
	}, searchEvalCase{Name: "case", Query: "query"}, 1, time.Second)
	encoded, err := json.Marshal(measurement)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "sk-private") || !strings.Contains(measurement.Error, "<redacted>") {
		t.Fatalf("measurement leaked secret: %s", encoded)
	}
}

func TestRunSearchEvalRequiresLiveAcknowledgement(t *testing.T) {
	out := captureStdout(t, func() {
		if got := runSearchEval([]string{"--json"}); got != 1 {
			t.Fatalf("runSearchEval = %d, want 1", got)
		}
	})
	if !strings.Contains(out, "requires --live") {
		t.Fatalf("output = %s", out)
	}
}

func TestExpandSearchEvalQuery(t *testing.T) {
	now := time.Date(2026, time.August, 10, 18, 30, 0, 0, time.FixedZone("CST", 8*60*60))
	got := expandSearchEvalQuery(" [Current date: {{CURRENT_DATE}}] at {{CURRENT_TIME}} ", now)
	want := "[Current date: 2026-08-10] at 2026-08-10T18:30:00+08:00"
	if got != want {
		t.Fatalf("expandSearchEvalQuery = %q, want %q", got, want)
	}
}

func TestSelectSearchEvalEndpointsRequiresExplicitDisabledSelection(t *testing.T) {
	disabled := false
	endpoints := []engine.GrokEndpoint{
		{Name: "enabled"},
		{Name: "disabled", Enabled: &disabled},
	}

	selected, err := selectSearchEvalEndpoints(endpoints, nil, true)
	if err != nil || len(selected) != 1 || selected[0].Name != "enabled" {
		t.Fatalf("default selection = %+v, %v", selected, err)
	}
	if _, err := selectSearchEvalEndpoints(endpoints, map[string]bool{"disabled": true}, false); err == nil || !strings.Contains(err.Error(), "--include-disabled") {
		t.Fatalf("disabled selection error = %v", err)
	}
	selected, err = selectSearchEvalEndpoints(endpoints, map[string]bool{"disabled": true}, true)
	if err != nil || len(selected) != 1 || selected[0].Name != "disabled" {
		t.Fatalf("explicit disabled selection = %+v, %v", selected, err)
	}
}
