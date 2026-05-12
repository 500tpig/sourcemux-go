package tools

import (
	"fmt"
	"strings"
)

func writeStringList(sb *strings.Builder, title string, items []string) {
	fmt.Fprintf(sb, "%s:\n", title)
	for _, item := range items {
		fmt.Fprintf(sb, "- %s\n", item)
	}
}

func writeLimitedStringList(sb *strings.Builder, title string, items []string, limit int) {
	fmt.Fprintf(sb, "%s:\n", title)
	writeLimitedBulletLines(sb, items, limit)
}

func writeLimitedBulletLines(sb *strings.Builder, items []string, limit int) {
	if limit <= 0 || limit > len(items) {
		limit = len(items)
	}
	for _, item := range items[:limit] {
		fmt.Fprintf(sb, "- %s\n", item)
	}
	if remaining := len(items) - limit; remaining > 0 {
		fmt.Fprintf(sb, "- ... (%d more)\n", remaining)
	}
}

func indentContinuation(text, prefix string) string {
	lines := strings.Split(text, "\n")
	for i := 1; i < len(lines); i++ {
		lines[i] = prefix + lines[i]
	}
	return strings.Join(lines, "\n")
}

func clipOneLine(text string, maxRunes int) string {
	text = strings.Join(strings.Fields(text), " ")
	return clipRunes(text, maxRunes)
}

func clipRunes(text string, maxRunes int) string {
	if maxRunes <= 0 {
		return text
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return strings.TrimSpace(string(runes[:maxRunes])) + "..."
}
