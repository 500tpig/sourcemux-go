package tools

import (
	"fmt"
	"sort"
	"strings"
)

const agentExcerptRunes = 700

type AgentOutput struct {
	Mode            string        `json:"mode"`
	Status          string        `json:"status"`
	AnswerReadiness string        `json:"answer_readiness"`
	SelectedSources []AgentSource `json:"selected_sources"`
	Warnings        []string      `json:"warnings"`
	NextActions     []string      `json:"next_actions"`
	Facts           []string      `json:"facts,omitempty"`
	Gaps            []string      `json:"gaps,omitempty"`
	Error           string        `json:"error,omitempty"`
}

type AgentSource struct {
	ID           string  `json:"id"`
	URL          string  `json:"url"`
	Provider     string  `json:"provider,omitempty"`
	Domain       string  `json:"domain,omitempty"`
	QualityScore int     `json:"quality_score"`
	Quality      string  `json:"quality"`
	Reason       string  `json:"reason"`
	Excerpt      string  `json:"excerpt,omitempty"`
	ContentLen   int     `json:"content_len,omitempty"`
	Score        float64 `json:"score,omitempty"`
}

func BuildFetchAgentOutput(result *WebFetchResult) AgentOutput {
	out := baseAgentOutput("fetch")
	if result == nil {
		out.Status = "error"
		out.AnswerReadiness = "blocked"
		out.Warnings = append(out.Warnings, "empty fetch result")
		out.NextActions = append(out.NextActions, `retry fetch with --profile quality or run search --agent to find alternate sources`)
		return out
	}
	content := strings.TrimSpace(result.Content)
	score, reasons := contentQuality(content)
	if result.Policy.Reason != "" {
		reasons = append(reasons, result.Policy.Reason)
	}
	source := AgentSource{
		ID:           "S1",
		URL:          result.URL,
		Provider:     result.Source,
		Domain:       domainFromURL(result.URL),
		QualityScore: score,
		Quality:      qualityLabel(score),
		Reason:       strings.Join(reasons, "; "),
		Excerpt:      clipRunes(content, agentExcerptRunes),
		ContentLen:   len([]rune(content)),
	}
	out.SelectedSources = []AgentSource{source}
	if score < 50 {
		out.AnswerReadiness = "needs_better_source"
		out.Warnings = append(out.Warnings, "selected source has low extraction quality")
		out.NextActions = append(out.NextActions, `retry fetch with --profile quality`, `run search --agent to find alternate URLs`)
	} else {
		out.AnswerReadiness = "ready"
		out.NextActions = append(out.NextActions, `cite S1 for claims grounded in this page`, `fetch another source if the answer needs corroboration`)
	}
	return out
}

func BuildSearchAgentOutput(query string, result *WebSearchResult) AgentOutput {
	out := baseAgentOutput("search")
	if result == nil {
		out.Status = "error"
		out.AnswerReadiness = "blocked"
		out.Warnings = append(out.Warnings, "empty search result")
		out.NextActions = append(out.NextActions, `retry search --agent with a more specific query`)
		return out
	}
	sources := rankAgentSearchSources(query, result.SourceURLs, 5)
	out.SelectedSources = sources
	if len(sources) == 0 {
		out.AnswerReadiness = "needs_sources"
		out.Warnings = append(out.Warnings, "search returned no source URLs")
		out.NextActions = append(out.NextActions, `retry search --agent with site: or docs-search`)
		return out
	}
	out.AnswerReadiness = "needs_fetch"
	out.Warnings = append(out.Warnings, searchWarnings(query, sources)...)
	out.NextActions = append(out.NextActions, `fetch S1 with fetch --agent before citing`)
	if queryLooksDocsLike(query) {
		out.NextActions = append(out.NextActions, `try docs-search for lower-noise official documentation results`)
	}
	if len(out.Warnings) > 0 {
		out.NextActions = append(out.NextActions, `refine with site:<domain> or add the exact product/project name`)
	}
	return out
}

func BuildResearchAgentOutput(pack ResearchPack) AgentOutput {
	out := baseAgentOutput("research")
	if pack.Error != "" {
		out.Status = "error"
		out.AnswerReadiness = "partial"
		out.Error = pack.Error
	}
	pageByURL := make(map[string]ResearchFetchedPage, len(pack.FetchedPagesSummary))
	for _, page := range pack.FetchedPagesSummary {
		if normalized, ok := normalizeResearchURL(page.URL); ok {
			pageByURL[normalized] = page
		}
	}
	limit := minInt(5, len(pack.HighSignalSources))
	out.SelectedSources = make([]AgentSource, 0, limit)
	for i, source := range pack.HighSignalSources[:limit] {
		page := pageByURL[source.URL]
		score := int(source.Score * 10)
		if page.Success {
			score += 20
		}
		if score > 100 {
			score = 100
		}
		if score < 0 {
			score = 0
		}
		reason := strings.Join(source.Reasons, "; ")
		if reason == "" {
			reason = "ranked by SourceMux research source signals"
		}
		out.SelectedSources = append(out.SelectedSources, AgentSource{
			ID:           fmt.Sprintf("S%d", i+1),
			URL:          source.URL,
			Provider:     page.Source,
			Domain:       source.Domain,
			QualityScore: score,
			Quality:      qualityLabel(score),
			Reason:       reason,
			Excerpt:      clipRunes(strings.TrimSpace(page.Excerpt), agentExcerptRunes),
			ContentLen:   page.ContentChars,
			Score:        source.Score,
		})
	}
	out.Facts = firstNonEmptyStrings(pack.ConfirmedFacts, 6)
	out.Gaps = firstNonEmptyStrings(pack.OpenQuestions, 6)
	switch {
	case len(out.Facts) > 0 && !strings.HasPrefix(out.Facts[0], "No source-backed facts"):
		out.AnswerReadiness = "ready_with_gaps"
	case len(out.SelectedSources) > 0:
		out.AnswerReadiness = "needs_manual_read"
		out.Warnings = append(out.Warnings, "research found sources but extracted no strong source-backed facts")
	default:
		out.AnswerReadiness = "needs_sources"
		out.Warnings = append(out.Warnings, "research found no usable sources")
	}
	out.NextActions = append(out.NextActions, `cite source ids from selected_sources`, `fetch any source id that needs full-page verification`)
	if len(out.Gaps) > 0 {
		out.NextActions = append(out.NextActions, `answer gaps with a narrower search or deeper research run`)
	}
	return out
}

func BuildAgentErrorOutput(mode, msg string) AgentOutput {
	out := baseAgentOutput(mode)
	out.Status = "error"
	out.AnswerReadiness = "blocked"
	out.Error = msg
	out.Warnings = append(out.Warnings, msg)
	out.NextActions = append(out.NextActions, `fix the reported error, then rerun with --agent`)
	return out
}

func baseAgentOutput(mode string) AgentOutput {
	return AgentOutput{
		Mode:            mode,
		Status:          "ok",
		AnswerReadiness: "unknown",
		SelectedSources: []AgentSource{},
		Warnings:        []string{},
		NextActions:     []string{},
	}
}

func rankAgentSearchSources(query string, urls []string, limit int) []AgentSource {
	inputs := make([]researchSourceInput, 0, len(urls))
	for _, raw := range urls {
		inputs = append(inputs, researchSourceInput{URL: raw, Query: query})
	}
	ranked, _ := rankResearchSources(deduplicateResearchSources(inputs), query, nil)
	if limit > len(ranked) {
		limit = len(ranked)
	}
	out := make([]AgentSource, 0, limit)
	for i, source := range ranked[:limit] {
		score := int(source.Score * 10)
		if score > 100 {
			score = 100
		}
		reason := strings.Join(source.Reasons, "; ")
		if reason == "" {
			reason = "ranked by URL and query relevance signals"
		}
		out = append(out, AgentSource{
			ID:           fmt.Sprintf("S%d", i+1),
			URL:          source.URL,
			Domain:       source.Domain,
			QualityScore: score,
			Quality:      qualityLabel(score),
			Reason:       reason,
			Score:        source.Score,
		})
	}
	return out
}

func searchWarnings(query string, sources []AgentSource) []string {
	warnings := make([]string, 0, 3)
	if topDomainsDispersed(sources) {
		warnings = append(warnings, "top domains are dispersed; search intent may be ambiguous")
	}
	queryDomains := domainsMentionedInQuery(query)
	for _, domain := range queryDomains {
		if !agentSourcesIncludeDomain(sources, domain) {
			warnings = append(warnings, fmt.Sprintf("query mentions %s but top results did not match that domain", domain))
		}
	}
	if possibleNamePollution(query, sources) {
		warnings = append(warnings, "top results have weak query-name match; possible same-name pollution")
	}
	return warnings
}

func topDomainsDispersed(sources []AgentSource) bool {
	if len(sources) < 3 {
		return false
	}
	counts := make(map[string]int)
	for _, source := range sources {
		if source.Domain != "" {
			counts[source.Domain]++
		}
	}
	if len(counts) < 3 {
		return false
	}
	maxCount := 0
	for _, count := range counts {
		if count > maxCount {
			maxCount = count
		}
	}
	return maxCount == 1
}

func domainsMentionedInQuery(query string) []string {
	fields := strings.Fields(strings.NewReplacer(",", " ", "(", " ", ")", " ", "\"", " ").Replace(query))
	out := make([]string, 0, 1)
	seen := make(map[string]struct{})
	for _, field := range fields {
		field = strings.Trim(field, " .:;!?")
		if !strings.Contains(field, ".") {
			continue
		}
		domain := normalizeAllowedDomain(field)
		if domain == "" || !strings.Contains(domain, ".") {
			continue
		}
		if _, ok := seen[domain]; ok {
			continue
		}
		seen[domain] = struct{}{}
		out = append(out, domain)
	}
	sort.Strings(out)
	return out
}

func agentSourcesIncludeDomain(sources []AgentSource, domain string) bool {
	for _, source := range sources {
		if domainAllowed(source.Domain, []string{domain}) {
			return true
		}
	}
	return false
}

func possibleNamePollution(query string, sources []AgentSource) bool {
	terms := tokenizeQuery(query)
	if len(terms) < 2 || len(sources) < 3 {
		return false
	}
	for _, source := range sources[:minInt(3, len(sources))] {
		if queryRelevanceScore(query, source.URL+" "+source.Domain) >= minInt(2, len(terms)) {
			return false
		}
	}
	return true
}

func contentQuality(content string) (int, []string) {
	if content == "" {
		return 0, []string{"empty content"}
	}
	score := 55
	reasons := []string{"content extracted"}
	length := len([]rune(content))
	switch {
	case length >= 2000:
		score += 25
		reasons = append(reasons, "substantial content")
	case length >= 500:
		score += 15
		reasons = append(reasons, "usable content length")
	case length < 120:
		score -= 20
		reasons = append(reasons, "very short content")
	}
	if looksBoilerplateExcerpt(content) {
		score -= 35
		reasons = append(reasons, "boilerplate signals")
	}
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	return score, reasons
}

func qualityLabel(score int) string {
	switch {
	case score >= 75:
		return "high"
	case score >= 45:
		return "medium"
	default:
		return "low"
	}
}

func queryLooksDocsLike(query string) bool {
	lower := strings.ToLower(query)
	for _, marker := range []string{"docs", "documentation", "api", "sdk", "reference", "guide"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func firstNonEmptyStrings(values []string, limit int) []string {
	out := make([]string, 0, minInt(limit, len(values)))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, value)
		if len(out) >= limit {
			return out
		}
	}
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
