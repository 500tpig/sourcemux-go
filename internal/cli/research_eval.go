package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/500tpig/sourcemux-go/internal/tools"
)

const defaultResearchEvalCasesPath = "docs/research-eval-cases.sample.json"

type researchEvalSuite struct {
	Cases []researchEvalCase `json:"cases"`
}

type researchEvalCase struct {
	Name          string                     `json:"name"`
	Query         string                     `json:"query"`
	Depth         string                     `json:"depth,omitempty"`
	Profile       string                     `json:"profile,omitempty"`
	Platform      string                     `json:"platform,omitempty"`
	Domains       []string                   `json:"domains,omitempty"`
	MaxFetches    int                        `json:"max_fetches,omitempty"`
	SearchResults []researchEvalSearchResult `json:"search_results"`
	FetchPages    []researchEvalFetchPage    `json:"fetch_pages"`
	Expect        researchEvalExpect         `json:"expect"`
}

type researchEvalSearchResult struct {
	QueryContains string   `json:"query_contains,omitempty"`
	URLs          []string `json:"urls"`
}

type researchEvalFetchPage struct {
	URL     string `json:"url"`
	Source  string `json:"source,omitempty"`
	Content string `json:"content,omitempty"`
	Error   string `json:"error,omitempty"`
}

type researchEvalExpect struct {
	SelectedSourceURLsInclude   []string `json:"selected_source_urls_include,omitempty"`
	HighSignalSourceURLsInclude []string `json:"high_signal_source_urls_include,omitempty"`
	ForbidSelectedSourceURLs    []string `json:"forbid_selected_source_urls,omitempty"`
	MinFetchedPages             int      `json:"min_fetched_pages,omitempty"`
	MinConfirmedFacts           int      `json:"min_confirmed_facts,omitempty"`
}

type researchEvalReport struct {
	OK        bool                     `json:"ok"`
	CasesFile string                   `json:"cases_file"`
	CaseCount int                      `json:"case_count"`
	Passed    int                      `json:"passed"`
	Failed    int                      `json:"failed"`
	Cases     []researchEvalCaseResult `json:"cases"`
	Error     string                   `json:"error,omitempty"`
}

type researchEvalCaseResult struct {
	Name        string                  `json:"name"`
	Query       string                  `json:"query"`
	Depth       string                  `json:"depth"`
	OK          bool                    `json:"ok"`
	Failures    []string                `json:"failures"`
	PackSummary researchEvalPackSummary `json:"pack_summary"`
	Pack        *tools.ResearchPack     `json:"pack,omitempty"`
}

type researchEvalPackSummary struct {
	SelectedSourceURLs   []string `json:"selected_source_urls"`
	HighSignalSourceURLs []string `json:"high_signal_source_urls"`
	FetchedPagesSuccess  int      `json:"fetched_pages_success"`
	ConfirmedFactsCount  int      `json:"confirmed_facts_count"`
}

func runResearchEval(args []string) int {
	fs := flag.NewFlagSet("eval-research", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	casesPath := fs.String("cases", defaultResearchEvalCasesPath, "Research eval cases JSON file")
	caseName := fs.String("case", "", "Only run one named case")
	includePack := fs.Bool("include-pack", false, "Include full research packs in JSON output")
	jsonOut := fs.Bool("json", false, "Emit JSON")
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	report, err := runResearchEvalCases(context.Background(), *casesPath, *caseName, *includePack)
	if err != nil {
		report = researchEvalReport{
			OK:        false,
			CasesFile: *casesPath,
			Cases:     []researchEvalCaseResult{},
			Error:     err.Error(),
		}
	}
	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(report)
	} else {
		printResearchEvalReport(report)
	}
	if report.OK {
		return 0
	}
	return 1
}

func runResearchEvalCases(ctx context.Context, casesPath, onlyName string, includePack bool) (researchEvalReport, error) {
	suite, err := loadResearchEvalSuite(casesPath)
	if err != nil {
		return researchEvalReport{}, err
	}
	report := researchEvalReport{
		CasesFile: casesPath,
		Cases:     []researchEvalCaseResult{},
	}
	for _, tc := range suite.Cases {
		if strings.TrimSpace(onlyName) != "" && tc.Name != onlyName {
			continue
		}
		result := runResearchEvalCase(ctx, tc, includePack)
		report.Cases = append(report.Cases, result)
		if result.OK {
			report.Passed++
		} else {
			report.Failed++
		}
	}
	report.CaseCount = len(report.Cases)
	report.OK = report.Failed == 0 && report.CaseCount > 0
	if report.CaseCount == 0 {
		return report, fmt.Errorf("no research eval cases matched %q", onlyName)
	}
	return report, nil
}

func loadResearchEvalSuite(path string) (researchEvalSuite, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return researchEvalSuite{}, fmt.Errorf("read cases file %s: %w", path, err)
	}
	var suite researchEvalSuite
	if err := json.Unmarshal(data, &suite); err != nil {
		return researchEvalSuite{}, fmt.Errorf("parse cases file %s: %w", path, err)
	}
	if len(suite.Cases) == 0 {
		return researchEvalSuite{}, errors.New("no research eval cases configured")
	}
	for i, tc := range suite.Cases {
		if strings.TrimSpace(tc.Name) == "" {
			return researchEvalSuite{}, fmt.Errorf("case #%d has empty name", i+1)
		}
		if strings.TrimSpace(tc.Query) == "" {
			return researchEvalSuite{}, fmt.Errorf("case %q has empty query", tc.Name)
		}
		if len(tc.SearchResults) == 0 {
			return researchEvalSuite{}, fmt.Errorf("case %q has no search_results", tc.Name)
		}
	}
	return suite, nil
}

func runResearchEvalCase(ctx context.Context, tc researchEvalCase, includePack bool) researchEvalCaseResult {
	executor := &tools.ResearchExecutor{
		Searcher: researchEvalSearcher{results: tc.SearchResults},
		Fetcher:  researchEvalFetcher{pages: tc.FetchPages},
	}
	pack, err := executor.Run(ctx, tools.ResearchOptions{
		Query:      tc.Query,
		Depth:      tc.Depth,
		Profile:    tc.Profile,
		Platform:   tc.Platform,
		Domains:    tc.Domains,
		MaxFetches: tc.MaxFetches,
	})
	failures := evaluateResearchEvalExpect(tc.Expect, pack)
	if err != nil {
		failures = append(failures, "research run failed: "+err.Error())
	}
	result := researchEvalCaseResult{
		Name:     tc.Name,
		Query:    tc.Query,
		Depth:    pack.EffectiveDepth,
		OK:       len(failures) == 0,
		Failures: failures,
		PackSummary: researchEvalPackSummary{
			SelectedSourceURLs:   append([]string(nil), pack.SourceSummary.SelectedSourceURLs...),
			HighSignalSourceURLs: append([]string(nil), pack.SourceSummary.HighSignalSourceURLs...),
			FetchedPagesSuccess:  successfulFetchedPages(pack.FetchedPagesSummary),
			ConfirmedFactsCount:  confirmedFactsCount(pack.ConfirmedFacts),
		},
	}
	if result.Failures == nil {
		result.Failures = []string{}
	}
	if result.PackSummary.SelectedSourceURLs == nil {
		result.PackSummary.SelectedSourceURLs = []string{}
	}
	if result.PackSummary.HighSignalSourceURLs == nil {
		result.PackSummary.HighSignalSourceURLs = []string{}
	}
	if includePack {
		result.Pack = &pack
	}
	return result
}

func evaluateResearchEvalExpect(expect researchEvalExpect, pack tools.ResearchPack) []string {
	failures := make([]string, 0)
	for _, url := range expect.SelectedSourceURLsInclude {
		if !stringSliceContains(pack.SourceSummary.SelectedSourceURLs, url) {
			failures = append(failures, fmt.Sprintf("selected_source_urls missing %s", url))
		}
	}
	for _, url := range expect.HighSignalSourceURLsInclude {
		if !stringSliceContains(pack.SourceSummary.HighSignalSourceURLs, url) {
			failures = append(failures, fmt.Sprintf("high_signal_source_urls missing %s", url))
		}
	}
	for _, url := range expect.ForbidSelectedSourceURLs {
		if stringSliceContains(pack.SourceSummary.SelectedSourceURLs, url) {
			failures = append(failures, fmt.Sprintf("selected_source_urls unexpectedly included %s", url))
		}
	}
	if got := successfulFetchedPages(pack.FetchedPagesSummary); got < expect.MinFetchedPages {
		failures = append(failures, fmt.Sprintf("fetched_pages_success = %d, want >= %d", got, expect.MinFetchedPages))
	}
	if got := confirmedFactsCount(pack.ConfirmedFacts); got < expect.MinConfirmedFacts {
		failures = append(failures, fmt.Sprintf("confirmed_facts_count = %d, want >= %d", got, expect.MinConfirmedFacts))
	}
	return failures
}

func successfulFetchedPages(pages []tools.ResearchFetchedPage) int {
	count := 0
	for _, page := range pages {
		if page.Success {
			count++
		}
	}
	return count
}

func confirmedFactsCount(facts []string) int {
	count := 0
	for _, fact := range facts {
		if strings.HasPrefix(fact, "No source-backed facts") {
			continue
		}
		count++
	}
	return count
}

func stringSliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func printResearchEvalReport(report researchEvalReport) {
	fmt.Printf("research eval: ok=%v cases=%d passed=%d failed=%d\n", report.OK, report.CaseCount, report.Passed, report.Failed)
	if report.Error != "" {
		fmt.Printf("error: %s\n", report.Error)
	}
	for _, c := range report.Cases {
		fmt.Printf("- %s ok=%v fetched=%d confirmed_facts=%d\n", c.Name, c.OK, c.PackSummary.FetchedPagesSuccess, c.PackSummary.ConfirmedFactsCount)
		for _, failure := range c.Failures {
			fmt.Printf("  failure: %s\n", failure)
		}
	}
}

type researchEvalSearcher struct {
	results []researchEvalSearchResult
}

func (s researchEvalSearcher) Search(ctx context.Context, query, platform, profile string) (*tools.WebSearchResult, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	lowerQuery := strings.ToLower(query)
	urls := make([]string, 0)
	for _, result := range s.results {
		needle := strings.ToLower(strings.TrimSpace(result.QueryContains))
		if needle != "" && !strings.Contains(lowerQuery, needle) {
			continue
		}
		urls = append(urls, result.URLs...)
	}
	return &tools.WebSearchResult{
		Query:        query,
		Engine:       "Research Eval Fixture",
		SessionID:    "research-eval-" + strings.ReplaceAll(strings.ToLower(strings.TrimSpace(query)), " ", "-"),
		Content:      "fixture search result",
		SourceURLs:   urls,
		SourcesCount: len(urls),
	}, nil
}

type researchEvalFetcher struct {
	pages []researchEvalFetchPage
}

func (f researchEvalFetcher) Fetch(ctx context.Context, rawURL string) (*tools.WebFetchResult, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	for _, page := range f.pages {
		if page.URL != rawURL {
			continue
		}
		if page.Error != "" {
			return nil, errors.New(page.Error)
		}
		source := page.Source
		if source == "" {
			source = "Research Eval Fixture"
		}
		return &tools.WebFetchResult{
			URL:     page.URL,
			Source:  source,
			Content: page.Content,
		}, nil
	}
	return nil, fmt.Errorf("fixture fetch page not found for %s", rawURL)
}
