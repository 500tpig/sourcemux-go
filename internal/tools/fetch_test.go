package tools

import (
	"strings"
	"testing"
)

func TestFormatWebFetchResultIsCompactForMCP(t *testing.T) {
	content := strings.Repeat("B", mcpFetchExcerptRunes) + "TAIL"
	body := FormatWebFetchResult(&WebFetchResult{
		Source:  "Jina Reader",
		URL:     "https://example.com/page",
		Content: content,
	})

	for _, want := range []string{
		"Source: Jina Reader",
		"URL: https://example.com/page",
		"content_chars:",
		"excerpt:",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "TAIL") {
		t.Fatalf("expected clipped MCP output, got tail in:\n%s", body)
	}
}
