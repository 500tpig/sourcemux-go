package engine

import (
	"net/url"
	"regexp"
	"strings"
)

// Source origin values are part of the search JSON contract. Keep them stable
// so live evaluation output can distinguish upstream evidence from URLs that
// only appeared in generated answer text.
const (
	SourceOriginNone          = "none"
	SourceOriginAnnotations   = "annotations"
	SourceOriginSearchSources = "search_sources"
	SourceOriginCitations     = "citations"
	SourceOriginSearchResults = "search_results"
	SourceOriginRegex         = "regex"
)

// urlRegex captures http(s) URLs without swallowing surrounding Markdown or
// common Chinese/English sentence punctuation.
var urlRegex = regexp.MustCompile(`https?://[^\s<>"'，。；：！？、）】》」』*\[\]]+`)

const urlTrailingPunct = ".,;:!?，。；：！？、*`_"

// NormalizeSourceURL extracts and validates one HTTP(S) source URL. It accepts
// plain URLs and common Markdown wrappers, then removes unmatched trailing
// brackets and sentence punctuation.
func NormalizeSourceURL(raw string) (string, bool) {
	candidate := cleanSourceURLCandidate(raw)
	if !strings.ContainsAny(candidate, " \t\r\n") {
		if normalized, ok := parseHTTPSourceURL(candidate); ok {
			return normalized, true
		}
	}
	match := urlRegex.FindString(raw)
	if match == "" {
		return "", false
	}
	return parseHTTPSourceURL(cleanSourceURLCandidate(match))
}

func cleanSourceURLCandidate(raw string) string {
	raw = strings.TrimSpace(strings.Trim(raw, "<>\"'"))
	if index := strings.IndexAny(raw, "，。；：！？、）】》」』"); index >= 0 {
		raw = raw[:index]
	}
	raw = strings.TrimRight(raw, urlTrailingPunct)
	raw = trimUnmatchedURLCloser(raw, '(', ')')
	raw = trimUnmatchedURLCloser(raw, '[', ']')
	raw = trimUnmatchedURLCloser(raw, '{', '}')
	raw = trimUnmatchedURLCloser(raw, '（', '）')
	raw = trimUnmatchedURLCloser(raw, '【', '】')
	return raw
}

func parseHTTPSourceURL(raw string) (string, bool) {
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", false
	}
	return parsed.String(), true
}

// NormalizeSourceURLs preserves first-seen order while dropping invalid,
// empty, and duplicate URLs.
func NormalizeSourceURLs(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, raw := range in {
		normalized, ok := NormalizeSourceURL(raw)
		if !ok {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out
}

func trimUnmatchedURLCloser(raw string, opener, closer rune) string {
	for strings.HasSuffix(raw, string(closer)) && strings.Count(raw, string(closer)) > strings.Count(raw, string(opener)) {
		raw = strings.TrimSuffix(raw, string(closer))
	}
	return raw
}

// dedupURLs is retained for provider helpers that already use the internal
// name. It now also enforces the shared URL normalization contract.
func dedupURLs(in []string) []string {
	return NormalizeSourceURLs(in)
}
