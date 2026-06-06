package tools

import (
	"strings"
	"testing"
)

func TestResolveFetchPolicyProviderOrders(t *testing.T) {
	tests := []struct {
		name    string
		profile string
		rawURL  string
		want    []string
	}{
		{
			name:    "auto ordinary quality first",
			profile: "auto",
			rawURL:  "https://example.com/article",
			want:    []string{"firecrawl", "jina", "exa", "tavily", "tinyfish"},
		},
		{
			name:    "quality ordinary",
			profile: "quality",
			rawURL:  "https://example.com/article",
			want:    []string{"firecrawl", "jina", "exa", "tavily", "tinyfish"},
		},
		{
			name:    "cheap keeps jina first",
			profile: "cheap",
			rawURL:  "https://example.com/article",
			want:    []string{"jina", "firecrawl", "exa", "tavily"},
		},
		{
			name:    "github intent",
			profile: "auto",
			rawURL:  "https://github.com/500tpig/sourcemux-go/issues/1",
			want:    []string{"github", "exa", "jina", "firecrawl", "tavily"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveFetchPolicy(tt.profile, tt.rawURL, nil, false)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Join(got.ProviderOrder, ",") != strings.Join(tt.want, ",") {
				t.Fatalf("order = %v, want %v", got.ProviderOrder, tt.want)
			}
		})
	}
}

func TestResolveFetchPolicyAutoHonorsExplicitV2OrderForOrdinaryURLs(t *testing.T) {
	got, err := ResolveFetchPolicy("auto", "https://example.com", []string{"jina", "firecrawl"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got.ProviderOrder, ",") != "jina,firecrawl" {
		t.Fatalf("order = %v", got.ProviderOrder)
	}
}

func TestClassifyFetchIntentGitHubRepoURLs(t *testing.T) {
	urls := []string{
		"https://github.com/owner/repo",
		"https://github.com/owner/repo/blob/main/README.md",
		"https://github.com/owner/repo/tree/main/docs",
		"https://github.com/owner/repo/issues/12",
		"https://github.com/owner/repo/releases/tag/v1.0.0",
	}
	for _, rawURL := range urls {
		if got := ClassifyFetchIntent(rawURL); got != FetchIntentGitHub {
			t.Fatalf("ClassifyFetchIntent(%q) = %q, want github", rawURL, got)
		}
	}
}
