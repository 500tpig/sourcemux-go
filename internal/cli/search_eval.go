package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/500tpig/sourcemux-go/internal/config"
	"github.com/500tpig/sourcemux-go/internal/engine"
)

const (
	defaultSearchEvalCasesPath = "docs/search-eval-cases.sample.json"
	searchEvalContentLimit     = 4000
)

type searchEvalSuite struct {
	Cases []searchEvalCase `json:"cases"`
}

type searchEvalCase struct {
	Name         string           `json:"name"`
	Query        string           `json:"query"`
	Platform     string           `json:"platform,omitempty"`
	TruthCommand string           `json:"truth_command,omitempty"`
	Expect       searchEvalExpect `json:"expect,omitempty"`
}

type searchEvalExpect struct {
	MinValidSources      *int     `json:"min_valid_sources,omitempty"`
	MinStructuredSources *int     `json:"min_structured_sources,omitempty"`
	ContentContains      []string `json:"content_contains,omitempty"`
	SourceDomainsInclude []string `json:"source_domains_include,omitempty"`
}

type searchEvalOptions struct {
	EndpointNames   map[string]bool
	OnlyCase        string
	Repeats         int
	Timeout         time.Duration
	IncludeDisabled bool
	Progress        func(searchEvalMeasurement)
}

type searchEvalReport struct {
	GeneratedAt   string                      `json:"generated_at"`
	CasesFile     string                      `json:"cases_file"`
	Live          bool                        `json:"live"`
	CaseCount     int                         `json:"case_count"`
	EndpointCount int                         `json:"endpoint_count"`
	Repeats       int                         `json:"repeats"`
	DurationMS    int64                       `json:"duration_ms"`
	Measurements  []searchEvalMeasurement     `json:"measurements"`
	Summaries     []searchEvalEndpointSummary `json:"summaries"`
	Error         string                      `json:"error,omitempty"`
}

type searchEvalMeasurement struct {
	EndpointName           string   `json:"endpoint_name"`
	Model                  string   `json:"model"`
	Profile                string   `json:"profile"`
	Case                   string   `json:"case"`
	TruthCommand           string   `json:"truth_command,omitempty"`
	Repeat                 int      `json:"repeat"`
	OK                     bool     `json:"ok"`
	Status                 string   `json:"status"`
	LatencyMS              int64    `json:"latency_ms"`
	SourceOrigin           string   `json:"source_origin"`
	SourcesCount           int      `json:"sources_count"`
	ValidSourcesCount      int      `json:"valid_sources_count"`
	StructuredSourcesCount int      `json:"structured_sources_count"`
	SourceURLs             []string `json:"source_urls"`
	ContentPreview         string   `json:"content_preview,omitempty"`
	ExpectationFailures    []string `json:"expectation_failures"`
	Error                  string   `json:"error,omitempty"`
}

type searchEvalEndpointSummary struct {
	EndpointName      string `json:"endpoint_name"`
	Model             string `json:"model"`
	Profile           string `json:"profile"`
	Attempts          int    `json:"attempts"`
	Passed            int    `json:"passed"`
	RequestErrors     int    `json:"request_errors"`
	ValidSourceBacked int    `json:"valid_source_backed"`
	StructuredBacked  int    `json:"structured_backed"`
	RegexOnly         int    `json:"regex_only"`
	AverageLatencyMS  int64  `json:"average_latency_ms"`
}

func runSearchEval(args []string) int {
	fs := flag.NewFlagSet("eval-search", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	casesPath := fs.String("cases", defaultSearchEvalCasesPath, "Live search eval cases JSON file")
	endpointNames := fs.String("endpoints", "", "Comma-separated endpoint names; default is every enabled Grok endpoint")
	onlyCase := fs.String("case", "", "Only run one named case")
	repeats := fs.Int("repeats", 3, "Number of runs per endpoint and case")
	timeout := fs.Duration("timeout", 180*time.Second, "Per-run timeout")
	includeDisabled := fs.Bool("include-disabled", false, "Allow explicitly selected disabled endpoints")
	progress := fs.Bool("progress", false, "Print one progress line per completed run to stderr")
	live := fs.Bool("live", false, "Required acknowledgement that this command calls paid external APIs")
	jsonOut := fs.Bool("json", false, "Emit JSON")
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if !*live {
		return reportSearchEvalError(*jsonOut, "eval-search requires --live because it calls external APIs")
	}
	if *repeats <= 0 {
		return reportSearchEvalError(*jsonOut, "repeats must be positive")
	}
	if *timeout <= 0 {
		return reportSearchEvalError(*jsonOut, "timeout must be positive")
	}

	cfg, err := loadConfig()
	if err != nil {
		return reportSearchEvalError(*jsonOut, fmt.Sprintf("config: %v", err))
	}
	suite, err := loadSearchEvalSuite(*casesPath)
	if err != nil {
		return reportSearchEvalError(*jsonOut, err.Error())
	}

	start := time.Now()
	options := searchEvalOptions{
		EndpointNames:   splitSearchEvalNames(*endpointNames),
		OnlyCase:        *onlyCase,
		Repeats:         *repeats,
		Timeout:         *timeout,
		IncludeDisabled: *includeDisabled,
	}
	if *progress {
		options.Progress = func(measurement searchEvalMeasurement) {
			fmt.Fprintf(os.Stderr, "eval-search endpoint=%s case=%s repeat=%d status=%s latency_ms=%d origin=%s valid=%d structured=%d\n",
				measurement.EndpointName, measurement.Case, measurement.Repeat, measurement.Status,
				measurement.LatencyMS, measurement.SourceOrigin, measurement.ValidSourcesCount,
				measurement.StructuredSourcesCount)
		}
	}
	report, err := runSearchEvalCases(context.Background(), cfg, suite, options)
	report.GeneratedAt = start.UTC().Format(time.RFC3339)
	report.CasesFile = *casesPath
	report.Live = true
	report.DurationMS = time.Since(start).Milliseconds()
	if err != nil {
		report.Error = err.Error()
	}
	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(report)
	} else {
		printSearchEvalReport(report)
	}
	if report.Error != "" {
		return 1
	}
	return 0
}

func loadSearchEvalSuite(path string) (searchEvalSuite, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return searchEvalSuite{}, fmt.Errorf("read cases file %s: %w", path, err)
	}
	var suite searchEvalSuite
	if err := json.Unmarshal(data, &suite); err != nil {
		return searchEvalSuite{}, fmt.Errorf("parse cases file %s: %w", path, err)
	}
	if len(suite.Cases) == 0 {
		return searchEvalSuite{}, errors.New("no search eval cases configured")
	}
	seen := make(map[string]struct{}, len(suite.Cases))
	for i, tc := range suite.Cases {
		if strings.TrimSpace(tc.Name) == "" {
			return searchEvalSuite{}, fmt.Errorf("case #%d has empty name", i+1)
		}
		if strings.TrimSpace(tc.Query) == "" {
			return searchEvalSuite{}, fmt.Errorf("case %q has empty query", tc.Name)
		}
		if _, exists := seen[tc.Name]; exists {
			return searchEvalSuite{}, fmt.Errorf("duplicate case name %q", tc.Name)
		}
		seen[tc.Name] = struct{}{}
		if tc.Expect.MinValidSources != nil && *tc.Expect.MinValidSources < 0 {
			return searchEvalSuite{}, fmt.Errorf("case %q min_valid_sources must be non-negative", tc.Name)
		}
		if tc.Expect.MinStructuredSources != nil && *tc.Expect.MinStructuredSources < 0 {
			return searchEvalSuite{}, fmt.Errorf("case %q min_structured_sources must be non-negative", tc.Name)
		}
	}
	return suite, nil
}

func runSearchEvalCases(ctx context.Context, cfg *config.Config, suite searchEvalSuite, opts searchEvalOptions) (searchEvalReport, error) {
	report := searchEvalReport{
		Repeats:      opts.Repeats,
		Measurements: []searchEvalMeasurement{},
		Summaries:    []searchEvalEndpointSummary{},
	}
	endpoints, err := selectSearchEvalEndpoints(cfg.GrokEndpoints, opts.EndpointNames, opts.IncludeDisabled)
	if err != nil {
		return report, err
	}
	cases := selectSearchEvalCases(suite.Cases, opts.OnlyCase)
	if len(cases) == 0 {
		return report, fmt.Errorf("no search eval cases matched %q", opts.OnlyCase)
	}
	report.CaseCount = len(cases)
	report.EndpointCount = len(endpoints)

	for _, endpoint := range endpoints {
		for _, tc := range cases {
			for repeat := 1; repeat <= opts.Repeats; repeat++ {
				if err := ctx.Err(); err != nil {
					return report, err
				}
				measurement := runSearchEvalMeasurement(ctx, endpoint, tc, repeat, opts.Timeout)
				report.Measurements = append(report.Measurements, measurement)
				if opts.Progress != nil {
					opts.Progress(measurement)
				}
			}
		}
	}
	report.Summaries = summarizeSearchEval(report.Measurements)
	return report, nil
}

func runSearchEvalMeasurement(parent context.Context, endpoint engine.GrokEndpoint, tc searchEvalCase, repeat int, timeout time.Duration) searchEvalMeasurement {
	query := expandSearchEvalQuery(tc.Query, time.Now())
	if platform := strings.TrimSpace(tc.Platform); platform != "" {
		query = fmt.Sprintf("[Focus: %s] %s", platform, query)
	}
	measurement := searchEvalMeasurement{
		EndpointName:        endpoint.Name,
		Model:               endpoint.Model,
		Profile:             endpoint.EffectiveProfile(),
		Case:                tc.Name,
		TruthCommand:        tc.TruthCommand,
		Repeat:              repeat,
		Status:              "request_error",
		SourceOrigin:        engine.SourceOriginNone,
		SourceURLs:          []string{},
		ExpectationFailures: []string{},
	}

	enabled := true
	endpoint.Enabled = &enabled
	pool := engine.NewGrokPool([]engine.GrokEndpoint{endpoint})
	clients := pool.Clients()
	if len(clients) != 1 {
		measurement.Error = "endpoint client could not be constructed"
		return measurement
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	start := time.Now()
	result, err := clients[0].Search(ctx, query)
	measurement.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		measurement.Error = redactSearchEvalError(err.Error(), endpoint.APIKey)
		return measurement
	}
	measurement.SourceOrigin = result.SourceOrigin
	measurement.SourceURLs = append([]string(nil), result.SourceURLs...)
	measurement.SourcesCount = result.SourcesCount
	measurement.ValidSourcesCount = result.ValidSourcesCount
	measurement.StructuredSourcesCount = result.StructuredSourcesCount
	measurement.ContentPreview = clipSearchEvalContent(result.Content)
	measurement.ExpectationFailures = evaluateSearchEvalExpect(tc.Expect, result)
	switch {
	case result.ValidSourcesCount == 0:
		measurement.Status = "no_valid_sources"
	case len(measurement.ExpectationFailures) > 0:
		measurement.Status = "expectation_failed"
	default:
		measurement.Status = "ok"
		measurement.OK = true
	}
	return measurement
}

func expandSearchEvalQuery(query string, now time.Time) string {
	query = strings.ReplaceAll(query, "{{CURRENT_DATE}}", now.Format("2006-01-02"))
	query = strings.ReplaceAll(query, "{{CURRENT_TIME}}", now.Format(time.RFC3339))
	return strings.TrimSpace(query)
}

func evaluateSearchEvalExpect(expect searchEvalExpect, result *engine.SearchResult) []string {
	failures := make([]string, 0)
	minValid := 1
	if expect.MinValidSources != nil {
		minValid = *expect.MinValidSources
	}
	if result.ValidSourcesCount < minValid {
		failures = append(failures, fmt.Sprintf("valid_sources_count=%d, want >= %d", result.ValidSourcesCount, minValid))
	}
	if expect.MinStructuredSources != nil && result.StructuredSourcesCount < *expect.MinStructuredSources {
		failures = append(failures, fmt.Sprintf("structured_sources_count=%d, want >= %d", result.StructuredSourcesCount, *expect.MinStructuredSources))
	}
	lowerContent := strings.ToLower(result.Content)
	for _, want := range expect.ContentContains {
		if !strings.Contains(lowerContent, strings.ToLower(strings.TrimSpace(want))) {
			failures = append(failures, fmt.Sprintf("content missing %q", want))
		}
	}
	for _, domain := range expect.SourceDomainsInclude {
		if !searchEvalURLsContainDomain(result.SourceURLs, domain) {
			failures = append(failures, fmt.Sprintf("source domains missing %q", domain))
		}
	}
	return failures
}

func selectSearchEvalEndpoints(endpoints []engine.GrokEndpoint, names map[string]bool, includeDisabled bool) ([]engine.GrokEndpoint, error) {
	out := make([]engine.GrokEndpoint, 0, len(endpoints))
	matched := make(map[string]bool, len(names))
	for _, endpoint := range endpoints {
		if len(names) > 0 && !names[endpoint.Name] {
			continue
		}
		matched[endpoint.Name] = true
		if !endpoint.IsEnabled() {
			if len(names) == 0 || !includeDisabled {
				if len(names) > 0 {
					return nil, fmt.Errorf("selected endpoint %q is disabled; pass --include-disabled to evaluate it", endpoint.Name)
				}
				continue
			}
		}
		out = append(out, endpoint)
	}
	for name := range names {
		if !matched[name] {
			return nil, fmt.Errorf("search endpoint %q not found", name)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("no enabled search endpoints selected")
	}
	return out, nil
}

func selectSearchEvalCases(cases []searchEvalCase, onlyName string) []searchEvalCase {
	if strings.TrimSpace(onlyName) == "" {
		return append([]searchEvalCase(nil), cases...)
	}
	for _, tc := range cases {
		if tc.Name == onlyName {
			return []searchEvalCase{tc}
		}
	}
	return nil
}

func summarizeSearchEval(measurements []searchEvalMeasurement) []searchEvalEndpointSummary {
	byEndpoint := make(map[string]*searchEvalEndpointSummary)
	order := make([]string, 0)
	for _, measurement := range measurements {
		summary := byEndpoint[measurement.EndpointName]
		if summary == nil {
			summary = &searchEvalEndpointSummary{
				EndpointName: measurement.EndpointName,
				Model:        measurement.Model,
				Profile:      measurement.Profile,
			}
			byEndpoint[measurement.EndpointName] = summary
			order = append(order, measurement.EndpointName)
		}
		summary.Attempts++
		summary.AverageLatencyMS += measurement.LatencyMS
		if measurement.OK {
			summary.Passed++
		}
		if measurement.Status == "request_error" {
			summary.RequestErrors++
		}
		if measurement.ValidSourcesCount > 0 {
			summary.ValidSourceBacked++
		}
		if measurement.StructuredSourcesCount > 0 {
			summary.StructuredBacked++
		} else if measurement.SourceOrigin == engine.SourceOriginRegex {
			summary.RegexOnly++
		}
	}
	out := make([]searchEvalEndpointSummary, 0, len(order))
	for _, name := range order {
		summary := byEndpoint[name]
		if summary.Attempts > 0 {
			summary.AverageLatencyMS /= int64(summary.Attempts)
		}
		out = append(out, *summary)
	}
	return out
}

func splitSearchEvalNames(raw string) map[string]bool {
	parts := strings.Split(raw, ",")
	out := make(map[string]bool)
	for _, part := range parts {
		if name := strings.TrimSpace(part); name != "" {
			out[name] = true
		}
	}
	return out
}

func searchEvalURLsContainDomain(urls []string, want string) bool {
	want = strings.ToLower(strings.TrimSpace(want))
	want = strings.TrimPrefix(want, "www.")
	for _, rawURL := range urls {
		parsed, err := url.Parse(rawURL)
		if err != nil {
			continue
		}
		host := strings.TrimPrefix(strings.ToLower(parsed.Hostname()), "www.")
		if host == want || strings.HasSuffix(host, "."+want) {
			return true
		}
	}
	return false
}

func clipSearchEvalContent(content string) string {
	runes := []rune(strings.TrimSpace(content))
	if len(runes) <= searchEvalContentLimit {
		return string(runes)
	}
	return string(runes[:searchEvalContentLimit]) + "..."
}

func redactSearchEvalError(message, secret string) string {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return message
	}
	return strings.ReplaceAll(message, secret, "<redacted>")
}

func printSearchEvalReport(report searchEvalReport) {
	fmt.Printf("search eval: endpoints=%d cases=%d repeats=%d duration=%dms\n",
		report.EndpointCount, report.CaseCount, report.Repeats, report.DurationMS)
	if report.Error != "" {
		fmt.Printf("error: %s\n", report.Error)
	}
	for _, summary := range report.Summaries {
		fmt.Printf("- %s model=%s attempts=%d passed=%d valid_source_backed=%d structured=%d regex_only=%d request_errors=%d avg_latency=%dms\n",
			summary.EndpointName, summary.Model, summary.Attempts, summary.Passed, summary.ValidSourceBacked,
			summary.StructuredBacked, summary.RegexOnly, summary.RequestErrors, summary.AverageLatencyMS)
	}
}

func reportSearchEvalError(asJSON bool, message string) int {
	if asJSON {
		_ = json.NewEncoder(os.Stdout).Encode(searchEvalReport{
			Live:         false,
			Measurements: []searchEvalMeasurement{},
			Summaries:    []searchEvalEndpointSummary{},
			Error:        message,
		})
	} else {
		fmt.Fprintln(os.Stderr, message)
	}
	return 1
}
