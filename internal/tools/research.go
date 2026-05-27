package tools

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/500tpig/sourcemux-go/internal/engine"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

const (
	researchFetchExcerptRunes  = 1200
	researchHumanExcerptRunes  = 700
	researchMaxFetches         = 12
	researchPerCallTimeout     = 25 * time.Second
	researchHeavySearchTimeout = 180 * time.Second
	researchHeavyFallbackAfter = 60 * time.Second
	mcpResearchPlanLimit       = 4
	mcpResearchSearchLimit     = 4
	mcpResearchSourceLimit     = 6
	mcpResearchPageLimit       = 4
	mcpResearchListLimit       = 4
	mcpResearchSnippetRunes    = 180
	mcpResearchExcerptRunes    = 260
)

func researchConcurrency(depth string) int {
	switch depth {
	case "quick":
		return 2
	case "deep":
		return 4
	default:
		return 3
	}
}

func researchSearchTimeout(profile string) time.Duration {
	if normalizeSearchProfile(profile) == engine.HeavyGrokEndpointProfile {
		return researchHeavySearchTimeout
	}
	return researchPerCallTimeout
}

var dateInURLPattern = regexp.MustCompile(`20(2[4-9]|3[0-9])`)

// ResearchOptions controls one in-memory research run.
type ResearchOptions struct {
	Query      string   `json:"query"`
	Depth      string   `json:"depth,omitempty"`
	Profile    string   `json:"profile,omitempty"`
	Platform   string   `json:"platform,omitempty"`
	Domains    []string `json:"domains,omitempty"`
	MaxFetches int      `json:"max_fetches,omitempty"`
}

// ResearchPack is the stable JSON output for research_run / cli research.
type ResearchPack struct {
	Query               string                  `json:"query"`
	EffectiveDepth      string                  `json:"effective_depth"`
	RequestedProfile    string                  `json:"requested_profile"`
	EffectiveProfile    string                  `json:"effective_profile"`
	ProfileReason       string                  `json:"profile_reason,omitempty"`
	Platform            string                  `json:"platform,omitempty"`
	Domains             []string                `json:"domains,omitempty"`
	MaxFetches          int                     `json:"max_fetches"`
	PlanQueries         []string                `json:"plan_queries"`
	ExecutedSearches    []ResearchSearchSummary `json:"executed_searches"`
	SourceSummary       ResearchSourceSummary   `json:"source_summary"`
	FetchedPagesSummary []ResearchFetchedPage   `json:"fetched_pages_summary"`
	HighSignalSources   []ResearchSource        `json:"high_signal_sources"`
	ConfirmedFacts      []string                `json:"confirmed_facts"`
	LikelyInferences    []string                `json:"likely_inferences"`
	OpenQuestions       []string                `json:"open_questions"`
	SiteContextSummary  []ResearchSiteSummary   `json:"site_context_summary,omitempty"`
	Error               string                  `json:"error,omitempty"`
}

// ResearchSearchSummary records one executed web_search round.
type ResearchSearchSummary struct {
	Query        string `json:"query"`
	Engine       string `json:"engine,omitempty"`
	SessionID    string `json:"session_id,omitempty"`
	SourcesCount int    `json:"sources_count"`
	Snippet      string `json:"snippet,omitempty"`
	Error        string `json:"error,omitempty"`
}

// ResearchSourceSummary records URL collection and selection statistics.
type ResearchSourceSummary struct {
	TotalURLs            int                     `json:"total_urls"`
	UniqueURLs           int                     `json:"unique_urls"`
	FilteredOutByDomain  int                     `json:"filtered_out_by_domain"`
	SelectedForFetch     int                     `json:"selected_for_fetch"`
	Domains              []ResearchDomainSummary `json:"domains"`
	SelectedSourceURLs   []string                `json:"selected_source_urls"`
	HighSignalSourceURLs []string                `json:"high_signal_source_urls"`
}

// ResearchDomainSummary is stable map-like output for source domains.
type ResearchDomainSummary struct {
	Domain string `json:"domain"`
	Count  int    `json:"count"`
}

// ResearchSource is one normalized, ranked candidate source URL.
type ResearchSource struct {
	URL           string   `json:"url"`
	Domain        string   `json:"domain"`
	Score         float64  `json:"score"`
	Occurrences   int      `json:"occurrences"`
	SearchQueries []string `json:"search_queries,omitempty"`
	Reasons       []string `json:"reasons,omitempty"`
}

// ResearchFetchedPage summarizes a fetched or crawled page with clipped text.
type ResearchFetchedPage struct {
	URL          string `json:"url"`
	Source       string `json:"source,omitempty"`
	Success      bool   `json:"success"`
	Error        string `json:"error,omitempty"`
	ContentChars int    `json:"content_chars,omitempty"`
	Excerpt      string `json:"excerpt,omitempty"`
}

// ResearchSiteSummary records optional map/crawl expansion for target domains.
type ResearchSiteSummary struct {
	Tool  string   `json:"tool"`
	URL   string   `json:"url"`
	Count int      `json:"count"`
	URLs  []string `json:"urls,omitempty"`
	Error string   `json:"error,omitempty"`
}

// ResearchSearchProvider abstracts web_search execution for fakes in tests.
type ResearchSearchProvider interface {
	Search(ctx context.Context, query, platform, profile string) (*WebSearchResult, error)
}

// ResearchSearchProfileResolver lets production searchers resolve profile=auto
// before the executor fans out concurrent searches.
type ResearchSearchProfileResolver interface {
	ResolveSearchProfile(requested string, opts ResearchOptions) (SearchProfileResolution, error)
}

// ResearchFetchProvider abstracts web_fetch execution for fakes in tests.
type ResearchFetchProvider interface {
	Fetch(ctx context.Context, rawURL string) (*WebFetchResult, error)
}

// ResearchMapper abstracts web_map for optional target-domain expansion.
type ResearchMapper interface {
	Map(ctx context.Context, rawURL string, maxDepth, maxBreadth, limit int) (*engine.MapResult, error)
}

// ResearchCrawler abstracts web_crawl for optional target-domain expansion.
type ResearchCrawler interface {
	Crawl(ctx context.Context, req engine.TavilyCrawlRequest) (*engine.TavilyCrawlResult, error)
}

// ResearchExecutor composes planning, search, get_sources, fetch, map, and
// crawl into one bounded in-memory research run.
type ResearchExecutor struct {
	Searcher ResearchSearchProvider
	Sources  SourceGetter
	Fetcher  ResearchFetchProvider
	Mapper   ResearchMapper
	Crawler  ResearchCrawler
}

// ResearchExecutorDeps wires production clients into the executor.
type ResearchExecutorDeps struct {
	Search  WebSearchClients
	Fetch   WebFetchClients
	Sources SourceGetter
	Mapper  ResearchMapper
	Crawler ResearchCrawler
}

// NewResearchExecutor creates a production executor using existing routing
// helpers. Tests can instantiate ResearchExecutor directly with fakes.
func NewResearchExecutor(deps ResearchExecutorDeps) *ResearchExecutor {
	return &ResearchExecutor{
		Searcher: webSearchResearchProvider{clients: deps.Search},
		Sources:  deps.Sources,
		Fetcher:  webFetchResearchProvider{clients: deps.Fetch},
		Mapper:   deps.Mapper,
		Crawler:  deps.Crawler,
	}
}

type webSearchResearchProvider struct {
	clients WebSearchClients
}

func (p webSearchResearchProvider) ResolveSearchProfile(requested string, opts ResearchOptions) (SearchProfileResolution, error) {
	return ResolveSearchProfile(p.clients.Pool, requested, SearchProfileContext{
		Flow:     searchProfileFlowResearch,
		Depth:    normalizeDepth(opts.Depth),
		Query:    opts.Query,
		Platform: opts.Platform,
	})
}

func (p webSearchResearchProvider) Search(ctx context.Context, query, platform, profile string) (*WebSearchResult, error) {
	clients := p.clients
	profileCtx := SearchProfileContext{
		Flow:     searchProfileFlowResearch,
		Query:    query,
		Platform: platform,
	}
	resolution, err := ResolveSearchProfile(p.clients.Pool, profile, profileCtx)
	if err != nil {
		return nil, err
	}
	if resolution.EffectiveProfile == engine.HeavyGrokEndpointProfile {
		clients = withResearchGrokPoolTimeout(clients, researchHeavyFallbackAfter)
	}
	return RunWebSearchWithOptions(ctx, clients, WebSearchOptions{
		Query:          query,
		Platform:       platform,
		Profile:        profile,
		ProfileContext: profileCtx,
	})
}

func withResearchGrokPoolTimeout(clients WebSearchClients, timeout time.Duration) WebSearchClients {
	if clients.Pool == nil || timeout <= 0 {
		return clients
	}
	return clients.WithGrokPoolTimeout(timeout)
}

type webFetchResearchProvider struct {
	clients WebFetchClients
}

func (p webFetchResearchProvider) Fetch(ctx context.Context, rawURL string) (*WebFetchResult, error) {
	return RunWebFetch(ctx, p.clients, rawURL)
}

// MemorySourceCache is a small in-memory source cache for CLI research runs.
type MemorySourceCache struct {
	mu      sync.RWMutex
	sources map[string][]string
}

func NewMemorySourceCache() *MemorySourceCache {
	return &MemorySourceCache{sources: make(map[string][]string)}
}

func (c *MemorySourceCache) CacheSources(sessionID string, urls []string) {
	if c == nil || sessionID == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sources[sessionID] = append([]string(nil), urls...)
}

func (c *MemorySourceCache) GetSources(sessionID string) ([]string, bool) {
	if c == nil || sessionID == "" {
		return nil, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	urls, ok := c.sources[sessionID]
	if !ok {
		return nil, false
	}
	return append([]string(nil), urls...), true
}

// RegisterResearchRun registers the executing research workflow MCP tool.
func RegisterResearchRun(s *mcpserver.MCPServer, executor *ResearchExecutor) {
	tool := mcp.NewTool("research_run",
		mcp.WithDescription("Run a bounded in-memory research workflow: plan queries, execute web_search rounds, optionally select a Grok search profile, read sources, rank/dedup URLs, fetch top pages, optionally map/crawl target domains, and return a compact research pack."),
		mcp.WithString("query", mcp.Required(), mcp.Description("Research question or topic")),
		mcp.WithString("depth", mcp.Description("Research depth: quick, standard, or deep (default standard)"), mcp.Enum("quick", "standard", "deep")),
		mcp.WithString("profile", mcp.Description("Grok endpoint profile for web_search: auto (default), default, heavy, or another configured profile")),
		mcp.WithString("platform", mcp.Description("Optional platform focus, e.g. 'GitHub, Reddit'")),
		mcp.WithArray("domains",
			mcp.Description("Optional allow-list of domains or site roots to prioritize/filter; may also drive web_map/web_crawl expansion"),
			mcp.Items(map[string]any{"type": "string"}),
		),
		mcp.WithNumber("max_fetches", mcp.Description("Maximum number of ranked URLs to fetch; defaults by depth")),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if executor == nil {
			return mcp.NewToolResultError("research executor is not configured"), nil
		}
		query, _ := req.Params.Arguments["query"].(string)
		if strings.TrimSpace(query) == "" {
			return mcp.NewToolResultError("query is required"), nil
		}
		depth, _ := req.Params.Arguments["depth"].(string)
		profile, _ := req.Params.Arguments["profile"].(string)
		profile = defaultResearchProfile(profile)
		platform, _ := req.Params.Arguments["platform"].(string)
		pack, err := executor.Run(ctx, ResearchOptions{
			Query:      query,
			Depth:      depth,
			Profile:    profile,
			Platform:   platform,
			Domains:    stringSliceArg(req.Params.Arguments, "domains"),
			MaxFetches: intArgOr(req.Params.Arguments, "max_fetches", 0),
		})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("research_run failed: %v", err)), nil
		}
		return mcp.NewToolResultText(FormatResearchPackMCP(pack)), nil
	})
}

// Run executes the complete research workflow.
func (e *ResearchExecutor) Run(ctx context.Context, opts ResearchOptions) (ResearchPack, error) {
	query := strings.TrimSpace(opts.Query)
	depth := normalizeDepth(opts.Depth)
	domains := normalizeAllowedDomains(opts.Domains)
	maxFetches := effectiveMaxFetches(depth, opts.MaxFetches)
	platform := strings.TrimSpace(opts.Platform)
	requestedProfile := defaultResearchProfile(opts.Profile)
	profileResolution := SearchProfileResolution{
		RequestedProfile: requestedProfile,
		EffectiveProfile: requestedProfile,
		ProfileReason:    "profile passed through by research searcher",
	}

	pack := ResearchPack{
		Query:               query,
		EffectiveDepth:      depth,
		RequestedProfile:    profileResolution.RequestedProfile,
		EffectiveProfile:    profileResolution.EffectiveProfile,
		ProfileReason:       profileResolution.ProfileReason,
		Platform:            platform,
		Domains:             domains,
		MaxFetches:          maxFetches,
		PlanQueries:         []string{},
		ExecutedSearches:    []ResearchSearchSummary{},
		SourceSummary:       emptyResearchSourceSummary(),
		FetchedPagesSummary: []ResearchFetchedPage{},
		HighSignalSources:   []ResearchSource{},
		ConfirmedFacts:      []string{},
		LikelyInferences:    []string{},
		OpenQuestions:       []string{},
	}
	if query == "" {
		pack.Error = "query is required"
		return pack, fmt.Errorf("query is required")
	}
	if e == nil || e.Searcher == nil {
		pack.Error = "research searcher is not configured"
		return pack, fmt.Errorf("research searcher is not configured")
	}
	if resolver, ok := e.Searcher.(ResearchSearchProfileResolver); ok {
		resolved, err := resolver.ResolveSearchProfile(requestedProfile, ResearchOptions{
			Query:      query,
			Depth:      depth,
			Profile:    requestedProfile,
			Platform:   platform,
			Domains:    domains,
			MaxFetches: maxFetches,
		})
		if err != nil {
			pack.RequestedProfile = resolved.RequestedProfile
			pack.EffectiveProfile = resolved.EffectiveProfile
			pack.ProfileReason = resolved.ProfileReason
			pack.Error = err.Error()
			return pack, err
		}
		profileResolution = resolved
		pack.RequestedProfile = resolved.RequestedProfile
		pack.EffectiveProfile = resolved.EffectiveProfile
		pack.ProfileReason = resolved.ProfileReason
	}
	searchProfile := profileResolution.EffectiveProfile
	searchTimeout := researchSearchTimeout(searchProfile)

	plan := BuildSearchPlan(query, depth, platform)
	pack.PlanQueries = uniqueStrings(ExtractPlanQueries(plan))
	if len(pack.PlanQueries) == 0 {
		pack.PlanQueries = []string{query}
	}

	sourceInputs := make([]researchSourceInput, 0)
	concurrency := researchConcurrency(depth)
	searchSummaries := make([]ResearchSearchSummary, len(pack.PlanQueries))
	perQueryURLs := make([][]researchSourceInput, len(pack.PlanQueries))
	var searchWG sync.WaitGroup
	searchSem := make(chan struct{}, concurrency)
	for i, plannedQuery := range pack.PlanQueries {
		searchWG.Add(1)
		go func(idx int, plannedQuery string) {
			defer searchWG.Done()
			searchSem <- struct{}{}
			defer func() { <-searchSem }()

			summary := ResearchSearchSummary{Query: plannedQuery}
			subCtx, cancel := context.WithTimeout(ctx, searchTimeout)
			defer cancel()
			res, err := e.Searcher.Search(subCtx, plannedQuery, platform, searchProfile)
			if err != nil {
				summary.Error = err.Error()
				searchSummaries[idx] = summary
				return
			}
			if res == nil {
				summary.Error = "empty search result"
				searchSummaries[idx] = summary
				return
			}
			summary.Engine = res.Engine
			summary.SessionID = res.SessionID
			summary.Snippet = clipOneLine(res.Content, 320)
			urls := append([]string(nil), res.SourceURLs...)
			if e.Sources != nil && res.SessionID != "" {
				if cached, ok := e.Sources.GetSources(res.SessionID); ok {
					urls = cached
				}
			}
			summary.SourcesCount = len(urls)
			searchSummaries[idx] = summary

			inputs := make([]researchSourceInput, 0, len(urls))
			for _, u := range urls {
				inputs = append(inputs, researchSourceInput{URL: u, Query: plannedQuery})
			}
			perQueryURLs[idx] = inputs
		}(i, plannedQuery)
	}
	searchWG.Wait()
	pack.ExecutedSearches = append(pack.ExecutedSearches, searchSummaries...)
	for _, inputs := range perQueryURLs {
		sourceInputs = append(sourceInputs, inputs...)
	}

	sourceInputs = append(sourceInputs, e.mapTargetDomains(ctx, query, depth, domains, &pack)...)

	allSources := deduplicateResearchSources(sourceInputs)
	rankedSources, filteredOut := rankResearchSources(allSources, query, domains)
	selectedFetchURLs := firstSourceURLs(rankedSources, maxFetches)
	highSignalLimit := maxInt(5, maxFetches)
	if highSignalLimit > len(rankedSources) {
		highSignalLimit = len(rankedSources)
	}
	pack.HighSignalSources = append([]ResearchSource{}, rankedSources[:highSignalLimit]...)
	pack.SourceSummary = buildResearchSourceSummary(allSources, rankedSources, selectedFetchURLs, filteredOut)

	pack.FetchedPagesSummary = e.fetchSelectedSources(ctx, selectedFetchURLs, concurrency)
	pack.FetchedPagesSummary = append(pack.FetchedPagesSummary, e.crawlTargetDomains(ctx, query, depth, domains, &pack)...)
	rankedSources = applyFetchSignals(rankedSources, pack.FetchedPagesSummary)
	if highSignalLimit > len(rankedSources) {
		highSignalLimit = len(rankedSources)
	}
	pack.HighSignalSources = append([]ResearchSource{}, rankedSources[:highSignalLimit]...)
	pack.SourceSummary.HighSignalSourceURLs = firstSourceURLs(rankedSources, highSignalLimit)

	pack.ConfirmedFacts = extractConfirmedFacts(pack.FetchedPagesSummary, query, 6)
	pack.LikelyInferences = buildLikelyInferences(rankedSources, pack.FetchedPagesSummary)
	pack.OpenQuestions = buildOpenQuestions(pack, maxFetches)

	return pack, nil
}

func (e *ResearchExecutor) mapTargetDomains(ctx context.Context, query, depth string, domains []string, pack *ResearchPack) []researchSourceInput {
	if e == nil || e.Mapper == nil || len(domains) == 0 {
		return nil
	}
	limit := map[string]int{"quick": 8, "standard": 12, "deep": 20}[depth]
	if limit == 0 {
		limit = 12
	}
	inputs := make([]researchSourceInput, 0)
	for _, domain := range domains {
		root := domainRootURL(domain)
		summary := ResearchSiteSummary{Tool: "web_map", URL: root}
		result, err := e.Mapper.Map(ctx, root, 1, 20, limit)
		if err != nil {
			summary.Error = err.Error()
			pack.SiteContextSummary = append(pack.SiteContextSummary, summary)
			continue
		}
		summary.URLs = append([]string(nil), result.URLs...)
		summary.Count = len(result.URLs)
		pack.SiteContextSummary = append(pack.SiteContextSummary, summary)
		for _, u := range result.URLs {
			inputs = append(inputs, researchSourceInput{URL: u, Query: query + " site map"})
		}
	}
	return inputs
}

func (e *ResearchExecutor) fetchSelectedSources(ctx context.Context, urls []string, concurrency int) []ResearchFetchedPage {
	pages := make([]ResearchFetchedPage, len(urls))
	if len(urls) == 0 {
		return pages
	}
	if concurrency < 1 {
		concurrency = 1
	}
	if concurrency > len(urls) {
		concurrency = len(urls)
	}
	var wg sync.WaitGroup
	sem := make(chan struct{}, concurrency)
	for i, u := range urls {
		wg.Add(1)
		go func(idx int, u string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			page := ResearchFetchedPage{URL: u}
			if e == nil || e.Fetcher == nil {
				page.Error = "research fetcher is not configured"
				pages[idx] = page
				return
			}
			subCtx, cancel := context.WithTimeout(ctx, researchPerCallTimeout)
			defer cancel()
			result, err := e.Fetcher.Fetch(subCtx, u)
			if err != nil {
				page.Error = err.Error()
				pages[idx] = page
				return
			}
			if result == nil || strings.TrimSpace(result.Content) == "" {
				page.Error = "empty fetch result"
				pages[idx] = page
				return
			}
			page.Success = true
			page.Source = result.Source
			if result.URL != "" {
				page.URL = result.URL
			}
			page.ContentChars = len([]rune(result.Content))
			page.Excerpt = clipRunes(strings.TrimSpace(result.Content), researchFetchExcerptRunes)
			pages[idx] = page
		}(i, u)
	}
	wg.Wait()
	return pages
}

func (e *ResearchExecutor) crawlTargetDomains(ctx context.Context, query, depth string, domains []string, pack *ResearchPack) []ResearchFetchedPage {
	if e == nil || e.Crawler == nil || len(domains) == 0 || depth != "deep" {
		return nil
	}
	root := domainRootURL(domains[0])
	req := engine.TavilyCrawlRequest{
		URL:           root,
		Instructions:  "Find pages directly relevant to: " + query,
		MaxDepth:      1,
		MaxBreadth:    20,
		Limit:         5,
		ExtractDepth:  "basic",
		Format:        "markdown",
		IncludeImages: false,
	}
	summary := ResearchSiteSummary{Tool: "web_crawl", URL: root}
	result, err := e.Crawler.Crawl(ctx, req)
	if err != nil {
		summary.Error = err.Error()
		pack.SiteContextSummary = append(pack.SiteContextSummary, summary)
		return nil
	}
	summary.Count = len(result.Results)
	pages := make([]ResearchFetchedPage, 0, len(result.Results))
	for _, page := range result.Results {
		summary.URLs = append(summary.URLs, page.URL)
		if strings.TrimSpace(page.RawContent) == "" {
			continue
		}
		pages = append(pages, ResearchFetchedPage{
			URL:          page.URL,
			Source:       "Tavily Crawl",
			Success:      true,
			ContentChars: len([]rune(page.RawContent)),
			Excerpt:      clipRunes(strings.TrimSpace(page.RawContent), researchFetchExcerptRunes),
		})
	}
	pack.SiteContextSummary = append(pack.SiteContextSummary, summary)
	return pages
}

// ExtractPlanQueries parses web_search query=... lines from BuildSearchPlan
// output or compatible hand-written plans.
func ExtractPlanQueries(plan string) []string {
	lines := strings.Split(plan, "\n")
	queries := make([]string, 0)
	for _, line := range lines {
		q, ok := extractPlanQueryLine(line)
		if ok && q != "" {
			queries = append(queries, q)
		}
	}
	return queries
}

func extractPlanQueryLine(line string) (string, bool) {
	if !strings.Contains(line, "web_search") {
		return "", false
	}
	idx := strings.Index(line, "query=")
	if idx < 0 {
		return "", false
	}
	rest := strings.TrimSpace(line[idx+len("query="):])
	if rest == "" {
		return "", false
	}
	if rest[0] == '"' {
		token, ok := readDoubleQuotedToken(rest)
		if !ok {
			return "", false
		}
		q, err := strconv.Unquote(token)
		if err != nil {
			return strings.Trim(token, `"`), true
		}
		return strings.TrimSpace(q), true
	}
	if rest[0] == '\'' {
		end := strings.Index(rest[1:], "'")
		if end < 0 {
			return strings.TrimSpace(rest[1:]), true
		}
		return strings.TrimSpace(rest[1 : end+1]), true
	}
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return "", false
	}
	return strings.TrimSpace(fields[0]), true
}

func readDoubleQuotedToken(s string) (string, bool) {
	escaped := false
	for i := 1; i < len(s); i++ {
		switch {
		case escaped:
			escaped = false
		case s[i] == '\\':
			escaped = true
		case s[i] == '"':
			return s[:i+1], true
		}
	}
	return "", false
}

type researchSourceInput struct {
	URL   string
	Query string
}

func deduplicateResearchSources(inputs []researchSourceInput) []ResearchSource {
	type acc struct {
		source  ResearchSource
		queries map[string]struct{}
	}
	seen := make(map[string]*acc)
	order := make([]string, 0, len(inputs))
	for _, input := range inputs {
		normalized, ok := normalizeResearchURL(input.URL)
		if !ok {
			continue
		}
		item, exists := seen[normalized]
		if !exists {
			item = &acc{
				source: ResearchSource{
					URL:         normalized,
					Domain:      domainFromURL(normalized),
					Occurrences: 0,
				},
				queries: make(map[string]struct{}),
			}
			seen[normalized] = item
			order = append(order, normalized)
		}
		item.source.Occurrences++
		q := strings.TrimSpace(input.Query)
		if q != "" {
			item.queries[q] = struct{}{}
		}
	}
	out := make([]ResearchSource, 0, len(order))
	for _, key := range order {
		item := seen[key]
		for q := range item.queries {
			item.source.SearchQueries = append(item.source.SearchQueries, q)
		}
		sort.Strings(item.source.SearchQueries)
		out = append(out, item.source)
	}
	return out
}

func rankResearchSources(sources []ResearchSource, query string, allowedDomains []string) ([]ResearchSource, int) {
	domainCounts := make(map[string]int)
	for _, s := range sources {
		domainCounts[s.Domain] += s.Occurrences
	}
	filtered := make([]ResearchSource, 0, len(sources))
	filteredOut := 0
	for _, s := range sources {
		if len(allowedDomains) > 0 && !domainAllowed(s.Domain, allowedDomains) {
			filteredOut++
			continue
		}
		score, reasons := scoreResearchSource(s, query, domainCounts[s.Domain])
		s.Score = score
		s.Reasons = reasons
		filtered = append(filtered, s)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].Score != filtered[j].Score {
			return filtered[i].Score > filtered[j].Score
		}
		if filtered[i].Occurrences != filtered[j].Occurrences {
			return filtered[i].Occurrences > filtered[j].Occurrences
		}
		return filtered[i].URL < filtered[j].URL
	})
	return filtered, filteredOut
}

func scoreResearchSource(source ResearchSource, query string, domainOccurrences int) (float64, []string) {
	score := 1.0
	reasons := make([]string, 0, 5)
	if source.Occurrences > 1 {
		score += float64(source.Occurrences) * 2
		reasons = append(reasons, fmt.Sprintf("repeated_url:%d", source.Occurrences))
	}
	if domainOccurrences > source.Occurrences {
		score += float64(domainOccurrences-source.Occurrences) * 0.75
		reasons = append(reasons, fmt.Sprintf("repeated_domain:%d", domainOccurrences))
	}
	if looksOfficial(source.URL, source.Domain) {
		score += 3
		reasons = append(reasons, "official_or_primary")
	}
	if dateInURLPattern.MatchString(source.URL) {
		score += 1.5
		reasons = append(reasons, "recent_date_in_url")
	}
	relevance := queryRelevanceScore(query, source.URL+" "+source.Domain)
	if relevance > 0 {
		score += float64(relevance)
		reasons = append(reasons, fmt.Sprintf("query_relevance:%d", relevance))
	}
	if strings.Contains(strings.ToLower(source.URL), "login") || strings.Contains(strings.ToLower(source.URL), "signup") {
		score -= 3
		reasons = append(reasons, "boilerplate_downrank")
	}
	return score, reasons
}

func applyFetchSignals(sources []ResearchSource, pages []ResearchFetchedPage) []ResearchSource {
	if len(sources) == 0 || len(pages) == 0 {
		return sources
	}
	pageByURL := make(map[string]ResearchFetchedPage, len(pages))
	for _, page := range pages {
		normalized, ok := normalizeResearchURL(page.URL)
		if ok {
			pageByURL[normalized] = page
		}
	}
	out := append([]ResearchSource(nil), sources...)
	for i := range out {
		page, ok := pageByURL[out[i].URL]
		if !ok {
			continue
		}
		switch {
		case !page.Success:
			out[i].Score -= 5
			out[i].Reasons = append(out[i].Reasons, "fetch_failed_downrank")
		case looksBoilerplateExcerpt(page.Excerpt):
			out[i].Score -= 2
			out[i].Reasons = append(out[i].Reasons, "boilerplate_content_downrank")
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		if out[i].Occurrences != out[j].Occurrences {
			return out[i].Occurrences > out[j].Occurrences
		}
		return out[i].URL < out[j].URL
	})
	return out
}

func looksBoilerplateExcerpt(excerpt string) bool {
	lower := strings.ToLower(excerpt)
	boilerplateMarkers := []string{
		"enable javascript",
		"sign in",
		"sign up",
		"cookie policy",
		"accept cookies",
		"captcha",
	}
	for _, marker := range boilerplateMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func looksOfficial(rawURL, domain string) bool {
	lowerURL := strings.ToLower(rawURL)
	switch {
	case strings.HasPrefix(domain, "docs."):
		return true
	case strings.HasPrefix(domain, "developer.") || strings.HasPrefix(domain, "developers."):
		return true
	case strings.HasSuffix(domain, ".gov") || strings.HasSuffix(domain, ".edu"):
		return true
	case domain == "github.com" || strings.HasSuffix(domain, ".github.io"):
		return true
	case strings.Contains(lowerURL, "/docs") || strings.Contains(lowerURL, "/documentation"):
		return true
	case strings.Contains(lowerURL, "/changelog") || strings.Contains(lowerURL, "/release"):
		return true
	default:
		return false
	}
}

func queryRelevanceScore(query, haystack string) int {
	terms := tokenizeQuery(query)
	if len(terms) == 0 {
		return 0
	}
	haystack = strings.ToLower(haystack)
	score := 0
	for _, term := range terms {
		if strings.Contains(haystack, term) {
			score++
		}
	}
	return score
}

func tokenizeQuery(query string) []string {
	cleaned := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
		return ' '
	}, strings.ToLower(query))
	fields := strings.Fields(cleaned)
	out := make([]string, 0, len(fields))
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		if len(field) < 3 {
			continue
		}
		if _, ok := seen[field]; ok {
			continue
		}
		seen[field] = struct{}{}
		out = append(out, field)
	}
	return out
}

func normalizeResearchURL(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", false
	}
	u.Scheme = strings.ToLower(u.Scheme)
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", false
	}
	u.Host = strings.ToLower(u.Host)
	u.Fragment = ""
	q := u.Query()
	for key := range q {
		lower := strings.ToLower(key)
		if strings.HasPrefix(lower, "utm_") || lower == "fbclid" || lower == "gclid" {
			q.Del(key)
		}
	}
	u.RawQuery = q.Encode()
	if u.Path == "/" {
		u.Path = ""
	}
	if strings.HasSuffix(u.Path, "/") && len(u.Path) > 1 {
		u.Path = strings.TrimRight(u.Path, "/")
	}
	return u.String(), true
}

func domainFromURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

func normalizeAllowedDomains(rawDomains []string) []string {
	seen := make(map[string]struct{}, len(rawDomains))
	out := make([]string, 0, len(rawDomains))
	for _, raw := range rawDomains {
		domain := normalizeAllowedDomain(raw)
		if domain == "" {
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

func normalizeAllowedDomain(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return ""
	}
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err == nil && u.Hostname() != "" {
			return strings.TrimPrefix(u.Hostname(), "www.")
		}
	}
	raw = strings.TrimPrefix(raw, "www.")
	raw = strings.TrimPrefix(raw, "http://")
	raw = strings.TrimPrefix(raw, "https://")
	raw = strings.Split(raw, "/")[0]
	raw = strings.TrimSpace(raw)
	return raw
}

func domainAllowed(host string, allowedDomains []string) bool {
	host = strings.TrimPrefix(strings.ToLower(host), "www.")
	for _, allowed := range allowedDomains {
		if host == allowed || strings.HasSuffix(host, "."+allowed) {
			return true
		}
	}
	return false
}

func domainRootURL(domain string) string {
	domain = normalizeAllowedDomain(domain)
	if domain == "" {
		return ""
	}
	return "https://" + domain
}

func buildResearchSourceSummary(allSources, rankedSources []ResearchSource, selectedURLs []string, filteredOut int) ResearchSourceSummary {
	domainCounts := make(map[string]int)
	for _, source := range allSources {
		domainCounts[source.Domain] += source.Occurrences
	}
	domains := make([]ResearchDomainSummary, 0, len(domainCounts))
	for domain, count := range domainCounts {
		domains = append(domains, ResearchDomainSummary{Domain: domain, Count: count})
	}
	sort.Slice(domains, func(i, j int) bool {
		if domains[i].Count != domains[j].Count {
			return domains[i].Count > domains[j].Count
		}
		return domains[i].Domain < domains[j].Domain
	})

	highSignalURLs := firstSourceURLs(rankedSources, maxInt(5, len(selectedURLs)))
	return ResearchSourceSummary{
		TotalURLs:            totalOccurrences(allSources),
		UniqueURLs:           len(allSources),
		FilteredOutByDomain:  filteredOut,
		SelectedForFetch:     len(selectedURLs),
		Domains:              domains,
		SelectedSourceURLs:   selectedURLs,
		HighSignalSourceURLs: highSignalURLs,
	}
}

func totalOccurrences(sources []ResearchSource) int {
	total := 0
	for _, source := range sources {
		total += source.Occurrences
	}
	return total
}

func firstSourceURLs(sources []ResearchSource, limit int) []string {
	if limit > len(sources) {
		limit = len(sources)
	}
	if limit < 0 {
		limit = 0
	}
	out := make([]string, 0, limit)
	for _, source := range sources[:limit] {
		out = append(out, source.URL)
	}
	return out
}

func extractConfirmedFacts(pages []ResearchFetchedPage, query string, limit int) []string {
	terms := tokenizeQuery(query)
	facts := make([]string, 0, limit)
	seen := make(map[string]struct{})
	for _, page := range pages {
		if !page.Success || page.Excerpt == "" {
			continue
		}
		if len(facts) >= limit {
			return facts
		}
		line := bestConfirmedFactLine(splitCandidateFactLines(page.Excerpt), terms)
		if line == "" {
			continue
		}
		fact := fmt.Sprintf("%s (source: %s)", clipOneLine(line, 220), page.URL)
		if _, ok := seen[fact]; ok {
			continue
		}
		seen[fact] = struct{}{}
		facts = append(facts, fact)
	}
	if len(facts) == 0 {
		return []string{"No source-backed facts were extracted by the v1 heuristic; inspect fetched excerpts directly."}
	}
	return facts
}

func splitCandidateFactLines(text string) []string {
	rawLines := strings.Split(text, "\n")
	out := make([]string, 0, len(rawLines))
	for _, line := range rawLines {
		line = strings.TrimSpace(strings.Trim(line, "#-*`> "))
		if len([]rune(line)) < 16 {
			continue
		}
		lower := strings.ToLower(line)
		if isBoilerplateFactLine(line, lower) {
			continue
		}
		out = append(out, line)
	}
	return out
}

func bestConfirmedFactLine(lines, terms []string) string {
	bestLine := ""
	bestScore := 0
	for _, line := range lines {
		if !lineRelevant(line, terms) {
			continue
		}
		score := confirmedFactScore(line, terms)
		if score > bestScore || (score == bestScore && len(bestLine) > 0 && len([]rune(line)) < len([]rune(bestLine))) {
			bestLine = line
			bestScore = score
		}
	}
	if bestScore < 5 {
		return ""
	}
	return bestLine
}

func lineRelevant(line string, terms []string) bool {
	if len(terms) == 0 {
		return true
	}
	line = strings.ToLower(line)
	for _, term := range terms {
		if strings.Contains(line, term) {
			return true
		}
	}
	return false
}

func confirmedFactScore(line string, terms []string) int {
	lower := strings.ToLower(line)
	score := maxInt(1, lineTermMatchCount(lower, terms)*4)

	for _, keyword := range []string{
		" vs ",
		"comparison",
		"compare",
		"feature",
		"use case",
		"best for",
		"parameter",
		"content extraction",
		"full content",
		"urls only",
		"site structure",
		"sitemap",
		"deep content analysis",
		"discover",
		"extract",
	} {
		if strings.Contains(lower, keyword) {
			score += 2
		}
	}
	if strings.HasPrefix(lower, "title:") {
		score++
	}
	if strings.Contains(line, "|") {
		score++
	}
	if strings.Contains(lower, "crawl") && strings.Contains(lower, "map") {
		score += 3
	}
	return score
}

func lineTermMatchCount(line string, terms []string) int {
	count := 0
	for _, term := range terms {
		if strings.Contains(line, term) {
			count++
		}
	}
	return count
}

func isBoilerplateFactLine(line, lower string) bool {
	if strings.HasPrefix(line, "![") || strings.Contains(line, "](") && strings.Contains(lower, "logo") {
		return true
	}
	for _, fragment := range []string{
		"cookie",
		"privacy policy",
		"skip to main content",
		"get an api key",
		"ctrl k",
		"search...",
		"navigation",
		"powered by",
		"support",
		"linkedin",
		"youtube",
		"company logo",
		"do not sell or share",
		"accept cookies",
		"reject all",
		"manage consent",
		"cookie notice",
		"url source:",
		"markdown content:",
		"light logo",
		"dark logo",
		"response =",
		"print(",
		"api_key=",
		"pip install",
		"npm i ",
		"abouttavily",
		"tavily status",
	} {
		if strings.Contains(lower, fragment) {
			return true
		}
	}
	if strings.Contains(line, "`") {
		return true
	}
	return false
}

func buildLikelyInferences(sources []ResearchSource, pages []ResearchFetchedPage) []string {
	inferences := make([]string, 0, 3)
	if len(sources) > 0 {
		top := sources[0]
		reason := strings.Join(top.Reasons, ", ")
		if reason == "" {
			reason = "it ranked highest by basic URL scoring"
		}
		inferences = append(inferences, fmt.Sprintf("Highest-ranked source is %s because %s.", top.URL, reason))
	}
	successes := 0
	for _, page := range pages {
		if page.Success {
			successes++
		}
	}
	if successes > 0 {
		inferences = append(inferences, fmt.Sprintf("%d fetched/crawled pages provide source text for synthesis.", successes))
	}
	if len(inferences) == 0 {
		return []string{"Insufficient fetched evidence for reliable inference."}
	}
	return inferences
}

func buildOpenQuestions(pack ResearchPack, maxFetches int) []string {
	questions := make([]string, 0, 4)
	for _, search := range pack.ExecutedSearches {
		if search.Error != "" {
			questions = append(questions, "Search failed for query: "+search.Query)
			break
		}
	}
	fetchErrors := 0
	for _, page := range pack.FetchedPagesSummary {
		if !page.Success {
			fetchErrors++
		}
	}
	if fetchErrors > 0 {
		questions = append(questions, fmt.Sprintf("%d selected source fetches failed and may need retry or alternate extraction.", fetchErrors))
	}
	if pack.SourceSummary.SelectedForFetch < maxFetches {
		questions = append(questions, "Fewer unique high-signal URLs were available than max_fetches.")
	}
	if len(pack.ConfirmedFacts) == 0 || strings.HasPrefix(pack.ConfirmedFacts[0], "No source-backed facts") {
		questions = append(questions, "Which claims can be confirmed after inspecting the fetched excerpts manually?")
	}
	if len(questions) == 0 {
		questions = append(questions, "No unresolved gaps were detected by heuristic v1; still check source disagreements before final use.")
	}
	return questions
}

// FormatResearchPack renders a compact LLM-readable text pack.
func FormatResearchPack(pack ResearchPack) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "research_pack\nquery: %s\ndepth: %s\n", pack.Query, pack.EffectiveDepth)
	if pack.RequestedProfile != "" || pack.EffectiveProfile != "" {
		fmt.Fprintf(&sb, "profile: requested=%s effective=%s\n", pack.RequestedProfile, pack.EffectiveProfile)
	}
	if pack.Platform != "" {
		fmt.Fprintf(&sb, "platform: %s\n", pack.Platform)
	}
	if len(pack.Domains) > 0 {
		fmt.Fprintf(&sb, "domains: %s\n", strings.Join(pack.Domains, ", "))
	}
	fmt.Fprintf(&sb, "max_fetches: %d\n", pack.MaxFetches)

	sb.WriteString("\nplan_queries:\n")
	for _, q := range pack.PlanQueries {
		fmt.Fprintf(&sb, "- %s\n", q)
	}

	sb.WriteString("\nexecuted_searches:\n")
	for _, search := range pack.ExecutedSearches {
		fmt.Fprintf(&sb, "- query: %s\n", search.Query)
		if search.Engine != "" {
			fmt.Fprintf(&sb, "  engine: %s\n", search.Engine)
		}
		if search.SessionID != "" {
			fmt.Fprintf(&sb, "  session_id: %s\n", search.SessionID)
		}
		fmt.Fprintf(&sb, "  sources_count: %d\n", search.SourcesCount)
		if search.Error != "" {
			fmt.Fprintf(&sb, "  error: %s\n", search.Error)
		}
		if search.Snippet != "" {
			fmt.Fprintf(&sb, "  snippet: %s\n", search.Snippet)
		}
	}

	fmt.Fprintf(&sb, "\nsource_summary:\n- total_urls: %d\n- unique_urls: %d\n- selected_for_fetch: %d\n",
		pack.SourceSummary.TotalURLs, pack.SourceSummary.UniqueURLs, pack.SourceSummary.SelectedForFetch)
	if pack.SourceSummary.FilteredOutByDomain > 0 {
		fmt.Fprintf(&sb, "- filtered_out_by_domain: %d\n", pack.SourceSummary.FilteredOutByDomain)
	}
	if len(pack.SourceSummary.Domains) > 0 {
		sb.WriteString("- domains:\n")
		for _, domain := range pack.SourceSummary.Domains {
			fmt.Fprintf(&sb, "  - %s (%d)\n", domain.Domain, domain.Count)
		}
	}

	sb.WriteString("\nhigh_signal_sources:\n")
	for _, source := range pack.HighSignalSources {
		fmt.Fprintf(&sb, "- %s score=%.2f occurrences=%d reasons=%s\n",
			source.URL, source.Score, source.Occurrences, strings.Join(source.Reasons, ","))
	}

	sb.WriteString("\nfetched_pages_summary:\n")
	for _, page := range pack.FetchedPagesSummary {
		fmt.Fprintf(&sb, "- %s\n", page.URL)
		if page.Source != "" {
			fmt.Fprintf(&sb, "  source: %s\n", page.Source)
		}
		fmt.Fprintf(&sb, "  success: %t\n", page.Success)
		if page.Error != "" {
			fmt.Fprintf(&sb, "  error: %s\n", page.Error)
		}
		if page.ContentChars > 0 {
			fmt.Fprintf(&sb, "  content_chars: %d\n", page.ContentChars)
		}
		if page.Excerpt != "" {
			fmt.Fprintf(&sb, "  excerpt: %s\n", indentContinuation(clipRunes(page.Excerpt, researchHumanExcerptRunes), "    "))
		}
	}

	writeStringList(&sb, "\nconfirmed_facts", pack.ConfirmedFacts)
	writeStringList(&sb, "\nlikely_inferences", pack.LikelyInferences)
	writeStringList(&sb, "\nopen_questions", pack.OpenQuestions)
	return strings.TrimSpace(sb.String())
}

// FormatResearchPackMCP renders a thin MCP summary while keeping CLI/full
// formatting available via FormatResearchPack and JSON output.
func FormatResearchPackMCP(pack ResearchPack) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "research_pack\nquery: %s\ndepth: %s\n", pack.Query, pack.EffectiveDepth)
	if pack.RequestedProfile != "" || pack.EffectiveProfile != "" {
		fmt.Fprintf(&sb, "profile: requested=%s effective=%s\n", pack.RequestedProfile, pack.EffectiveProfile)
	}
	if pack.Platform != "" {
		fmt.Fprintf(&sb, "platform: %s\n", pack.Platform)
	}
	if len(pack.Domains) > 0 {
		fmt.Fprintf(&sb, "domains: %s\n", strings.Join(pack.Domains, ", "))
	}
	fmt.Fprintf(&sb, "max_fetches: %d\n", pack.MaxFetches)
	fmt.Fprintf(&sb, "search_rounds: %d\n", len(pack.ExecutedSearches))
	fmt.Fprintf(&sb, "fetched_pages: %d\n", len(pack.FetchedPagesSummary))
	fmt.Fprintf(&sb, "unique_sources: %d\n", pack.SourceSummary.UniqueURLs)

	sb.WriteString("\nplan_queries:\n")
	writeLimitedBulletLines(&sb, pack.PlanQueries, mcpResearchPlanLimit)

	sb.WriteString("\nexecuted_searches:\n")
	if len(pack.ExecutedSearches) > 0 {
		limit := mcpResearchSearchLimit
		if limit > len(pack.ExecutedSearches) {
			limit = len(pack.ExecutedSearches)
		}
		for _, search := range pack.ExecutedSearches[:limit] {
			fmt.Fprintf(&sb, "- query: %s\n", search.Query)
			if search.Engine != "" {
				fmt.Fprintf(&sb, "  engine: %s\n", search.Engine)
			}
			if search.SessionID != "" {
				fmt.Fprintf(&sb, "  session_id: %s\n", search.SessionID)
				if search.SourcesCount > 0 {
					sb.WriteString("  sources: call get_sources(session_id) for URLs\n")
				}
			}
			fmt.Fprintf(&sb, "  sources_count: %d\n", search.SourcesCount)
			if search.Error != "" {
				fmt.Fprintf(&sb, "  error: %s\n", search.Error)
			}
			if search.Snippet != "" {
				fmt.Fprintf(&sb, "  snippet: %s\n", clipOneLine(search.Snippet, mcpResearchSnippetRunes))
			}
		}
		if remaining := len(pack.ExecutedSearches) - limit; remaining > 0 {
			fmt.Fprintf(&sb, "- ... (%d more search rounds)\n", remaining)
		}
	}

	fmt.Fprintf(&sb, "\nsource_summary:\n- total_urls: %d\n- unique_urls: %d\n- selected_for_fetch: %d\n",
		pack.SourceSummary.TotalURLs, pack.SourceSummary.UniqueURLs, pack.SourceSummary.SelectedForFetch)
	if pack.SourceSummary.FilteredOutByDomain > 0 {
		fmt.Fprintf(&sb, "- filtered_out_by_domain: %d\n", pack.SourceSummary.FilteredOutByDomain)
	}
	sb.WriteString("\nhigh_signal_sources:\n")
	if len(pack.HighSignalSources) > 0 {
		limit := mcpResearchSourceLimit
		if limit > len(pack.HighSignalSources) {
			limit = len(pack.HighSignalSources)
		}
		for _, source := range pack.HighSignalSources[:limit] {
			fmt.Fprintf(&sb, "- %s (score=%.2f, occurrences=%d)\n", source.URL, source.Score, source.Occurrences)
		}
		if remaining := len(pack.HighSignalSources) - limit; remaining > 0 {
			fmt.Fprintf(&sb, "- ... (%d more sources)\n", remaining)
		}
	}

	sb.WriteString("\nfetched_pages_summary:\n")
	if len(pack.FetchedPagesSummary) > 0 {
		limit := mcpResearchPageLimit
		if limit > len(pack.FetchedPagesSummary) {
			limit = len(pack.FetchedPagesSummary)
		}
		for _, page := range pack.FetchedPagesSummary[:limit] {
			fmt.Fprintf(&sb, "- %s\n", page.URL)
			if page.Source != "" {
				fmt.Fprintf(&sb, "  source: %s\n", page.Source)
			}
			fmt.Fprintf(&sb, "  success: %t\n", page.Success)
			if page.Error != "" {
				fmt.Fprintf(&sb, "  error: %s\n", page.Error)
			}
			if page.ContentChars > 0 {
				fmt.Fprintf(&sb, "  content_chars: %d\n", page.ContentChars)
			}
			if page.Excerpt != "" {
				fmt.Fprintf(&sb, "  excerpt: %s\n", indentContinuation(clipRunes(page.Excerpt, mcpResearchExcerptRunes), "    "))
			}
		}
		if remaining := len(pack.FetchedPagesSummary) - limit; remaining > 0 {
			fmt.Fprintf(&sb, "- ... (%d more fetched pages)\n", remaining)
		}
	}

	writeLimitedStringList(&sb, "\nconfirmed_facts", pack.ConfirmedFacts, mcpResearchListLimit)
	writeLimitedStringList(&sb, "\nlikely_inferences", pack.LikelyInferences, mcpResearchListLimit)
	writeLimitedStringList(&sb, "\nopen_questions", pack.OpenQuestions, mcpResearchListLimit)
	if pack.Error != "" {
		fmt.Fprintf(&sb, "\nerror: %s\n", pack.Error)
	}
	return strings.TrimSpace(sb.String())
}

func stringSliceArg(args map[string]any, key string) []string {
	raw, ok := args[key]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
		parts := strings.Split(v, ",")
		out := make([]string, 0, len(parts))
		for _, part := range parts {
			out = append(out, strings.TrimSpace(part))
		}
		return out
	default:
		return []string{fmt.Sprint(v)}
	}
}

func effectiveMaxFetches(depth string, requested int) int {
	if requested > 0 {
		if requested > researchMaxFetches {
			return researchMaxFetches
		}
		return requested
	}
	switch depth {
	case "quick":
		return 2
	case "deep":
		return 8
	default:
		return 4
	}
}

func emptyResearchSourceSummary() ResearchSourceSummary {
	return ResearchSourceSummary{
		Domains:              []ResearchDomainSummary{},
		SelectedSourceURLs:   []string{},
		HighSignalSourceURLs: []string{},
	}
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
