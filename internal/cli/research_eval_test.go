package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunResearchEvalJSONUsesOfflineFixtures(t *testing.T) {
	casesPath := writeResearchEvalCases(t, `{
	  "cases": [
	    {
	      "name": "primary-docs",
	      "query": "Compare SourceMux fetch profiles",
	      "depth": "standard",
	      "max_fetches": 2,
	      "search_results": [
	        {
	          "query_contains": "official documentation",
	          "urls": [
	            "https://docs.example.com/sourcemux/fetch-profiles",
	            "https://example.com/login"
	          ]
	        },
	        {
	          "query_contains": "latest updates",
	          "urls": ["https://github.com/500tpig/sourcemux-go/releases"]
	        }
	      ],
	      "fetch_pages": [
	        {
	          "url": "https://docs.example.com/sourcemux/fetch-profiles",
	          "content": "SourceMux fetch profiles compare auto, quality, cheap, and github modes for extraction."
	        },
	        {
	          "url": "https://github.com/500tpig/sourcemux-go/releases",
	          "content": "SourceMux release notes document fetch profile changes and current behavior."
	        },
	        {
	          "url": "https://example.com/login",
	          "content": "Sign in. Accept cookies."
	        }
	      ],
	      "expect": {
	        "selected_source_urls_include": [
	          "https://docs.example.com/sourcemux/fetch-profiles",
	          "https://github.com/500tpig/sourcemux-go/releases"
	        ],
	        "forbid_selected_source_urls": ["https://example.com/login"],
	        "min_fetched_pages": 2,
	        "min_confirmed_facts": 1
	      }
	    }
	  ]
	}`)

	out := captureStdout(t, func() {
		if got := Run([]string{"eval-research", "--cases", casesPath, "--json"}); got != 0 {
			t.Fatalf("Run(eval-research) = %d, want 0", got)
		}
	})
	var report researchEvalReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode report: %v\n%s", err, out)
	}
	if !report.OK || report.Passed != 1 || report.Failed != 0 || len(report.Cases) != 1 {
		t.Fatalf("report = %+v", report)
	}
	gotCase := report.Cases[0]
	if gotCase.Pack != nil {
		t.Fatalf("pack should be omitted unless --include-pack is set")
	}
	if len(gotCase.Failures) != 0 {
		t.Fatalf("failures = %#v", gotCase.Failures)
	}
	if !testContainsString(gotCase.PackSummary.SelectedSourceURLs, "https://docs.example.com/sourcemux/fetch-profiles") {
		t.Fatalf("selected urls = %#v", gotCase.PackSummary.SelectedSourceURLs)
	}
	if gotCase.PackSummary.FetchedPagesSuccess != 2 {
		t.Fatalf("fetched success = %d", gotCase.PackSummary.FetchedPagesSuccess)
	}
	if gotCase.PackSummary.ConfirmedFactsCount < 1 {
		t.Fatalf("confirmed facts = %d", gotCase.PackSummary.ConfirmedFactsCount)
	}
}

func TestRunResearchEvalReportsExpectationFailures(t *testing.T) {
	casesPath := writeResearchEvalCases(t, `{
	  "cases": [
	    {
	      "name": "missing-source",
	      "query": "SourceMux docs",
	      "search_results": [
	        {"urls": ["https://docs.example.com/sourcemux"]}
	      ],
	      "fetch_pages": [
	        {"url": "https://docs.example.com/sourcemux", "content": "SourceMux docs describe search and fetch."}
	      ],
	      "expect": {
	        "selected_source_urls_include": ["https://missing.example.com/source"]
	      }
	    }
	  ]
	}`)

	out := captureStdout(t, func() {
		if got := Run([]string{"eval-research", "--cases", casesPath, "--json"}); got != 1 {
			t.Fatalf("Run(eval-research failure) = %d, want 1", got)
		}
	})
	var report researchEvalReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode report: %v\n%s", err, out)
	}
	if report.OK || report.Failed != 1 || len(report.Cases) != 1 {
		t.Fatalf("report = %+v", report)
	}
	if len(report.Cases[0].Failures) == 0 || !strings.Contains(report.Cases[0].Failures[0], "missing.example.com") {
		t.Fatalf("failures = %#v", report.Cases[0].Failures)
	}
}

func writeResearchEvalCases(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "research-eval-cases.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write cases: %v", err)
	}
	return path
}
