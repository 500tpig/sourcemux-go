package tools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/500tpig/sourcemux-go/internal/capability"
)

func TestGitHubFetchProviderReturnsRepoMetadataAndReadme(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/repo":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"description": "test repo",
				"stargazers_count": 12,
				"forks_count": 3,
				"language": "Go",
				"default_branch": "main",
				"open_issues_count": 4,
				"license": {"name": "MIT License", "spdx_id": "MIT"}
			}`))
		case "/repos/owner/repo/readme":
			w.Header().Set("Content-Type", "text/markdown")
			_, _ = w.Write([]byte("# Repo\nREADME body"))
		case "/repos/owner/repo/languages":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"Go":1000,"Shell":50}`))
		case "/repos/owner/repo/releases/latest":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"tag_name":"v1.2.3","name":"Release 1.2.3"}`))
		case "/owner/repo/main/README.md":
			w.Header().Set("Content-Type", "text/markdown")
			_, _ = w.Write([]byte("# Raw README"))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer api.Close()

	provider := NewGitHubFetchProvider(api.Client())
	provider.APIURL = api.URL
	provider.RawBaseURL = api.URL
	res, err := provider.Try(context.Background(), capability.Request{URL: "https://github.com/owner/repo/blob/main/README.md"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# owner/repo", "stars: 12", "forks: 3", "license: MIT", "languages: Go:1000, Shell:50", "latest_release: v1.2.3", "requested_kind: blob", "# Repo", "# Raw README"} {
		if !strings.Contains(res.Content, want) {
			t.Fatalf("content missing %q:\n%s", want, res.Content)
		}
	}
	if got := res.Metadata["engine"]; got != "GitHub Provider" {
		t.Fatalf("engine = %v", got)
	}
}
