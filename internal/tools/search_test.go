package tools

import (
	"strings"
	"testing"

	"github.com/500tpig/grok-search-go/internal/engine"
)

type testSourceCache struct {
	sessionID string
	urls      []string
}

func (c *testSourceCache) CacheSources(sessionID string, urls []string) {
	c.sessionID = sessionID
	c.urls = urls
}

func TestFormatTinyFishResponseCachesSources(t *testing.T) {
	cache := &testSourceCache{}
	res := webSearchTinyFishResult("query", &engine.TinyFishPoolSearchResult{
		KeyName: "acct-a",
		TinyFishSearchResponse: &engine.TinyFishSearchResponse{
			Results: []engine.TinyFishSearchResult{
				{Title: "A", URL: "https://example.com/a"},
				{Title: "B", URL: "https://example.com/b"},
			},
		},
	}, nil, cache)
	body := FormatWebSearchResult(res)

	if !strings.Contains(body, "engine: TinyFish Search (acct-a; no Grok endpoint configured)") {
		t.Fatalf("body = %s", body)
	}
	if cache.sessionID == "" {
		t.Fatal("session id was not cached")
	}
	if strings.Join(cache.urls, ",") != "https://example.com/a,https://example.com/b" {
		t.Fatalf("cached urls = %v", cache.urls)
	}
}

func TestFormatWebSearchResultIsCompactForMCP(t *testing.T) {
	content := strings.Repeat("A", mcpSearchExcerptRunes) + "TAIL"
	body := FormatWebSearchResult(&WebSearchResult{
		Engine:       "grok-prod",
		Model:        "grok-4.20-fast",
		SessionID:    "session-123",
		Content:      content,
		SourcesCount: 2,
	})

	for _, want := range []string{
		"engine: grok-prod (grok-4.20-fast)",
		"session_id: session-123",
		"sources: call get_sources(session_id) for URLs",
		"content_chars:",
		"summary:",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "TAIL") {
		t.Fatalf("expected clipped MCP output, got tail in:\n%s", body)
	}
}
